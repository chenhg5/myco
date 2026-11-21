#!/usr/bin/env python3
"""A/B evaluation of what project context does for real agents.

Builds two versions of myco (by default the latest release tag and the
working tree), and for each one starts its own hub and two Claude Code
agents (alpha, beta) served by that version's runner, in a project led by
a coordinator (lead). lead sends them fixed tasks whose right answers are
only in the project's context, and the answers are scored:

  brief-tail   a fact past the first 4 KB of the brief
               (older runners cut the brief at 4 KB)
  recorded     a convention the coordinator recorded: in the project memory
               where the hub has one, else in a project document (what
               people did before)
  handoff      the owner tells alpha a decision; later beta, in a new
               conversation, must know it (it only can if alpha wrote it
               down where beta reads it)
  preference   the owner tells alpha how replies should end; beta's reply
               to an unrelated task should follow it

Each version runs every task --repeat times with new agents. The report
gives the pass rate per task and version, time per task and, where the
hub keeps the run's steps, its cost. Needs: go, git, python3 and a
signed-in `claude` CLI. It spends real tokens (roughly $0.1-0.3 a task).

  scripts/eval-project-context.py                       # latest tag vs working tree
  scripts/eval-project-context.py --old v0.21.9 --new HEAD --repeat 3
  scripts/eval-project-context.py --model sonnet --json report.json
"""

import argparse
import json
import os
import re
import secrets
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

FILLER = """## Repository
- The code is in `acme/shop`: `api/` (Go), `web/` (React), `infra/` (Terraform).
- Work on a branch named after the task; open a PR against `main`.
- `make check` runs the linters and tests; CI runs the same.

## Conventions
- Go: errors wrap their cause with `%w`; no panics outside `main`.
- React: function components, hooks, no default exports.
- SQL migrations go in `api/migrations`, one file per change, never edited after merge.
- Logs are structured (`slog`); never log tokens or customer emails.

## Reviews
- A PR needs one approval; the author merges.
- Screenshots for UI changes, at 2x, light and dark.

"""

BRIEF_FACT = "## Deploy\n- Staging deploys go to port 7443 (not 8080, which is the old one).\n"
CONVENTION = "Every commit message in this project starts with the prefix SHOP-OPS:"


def brief():
    # Past the first 4 KB (where older runners cut the brief), within the
    # 8 KB older hubs accept.
    body = ""
    while len(body.encode()) < 4600:
        body += FILLER
    return "# acme shop\n\n" + body + BRIEF_FACT


TASKS = [
    # (id, to, body, check)
    ("brief-tail", "alpha",
     "Which port do staging deploys of this project go to? Answer with the number only.",
     lambda r: "7443" in r),
    ("recorded", "beta",
     "I am about to commit a fix for the cart total. Write the first line of the commit message, nothing else.",
     lambda r: "SHOP-OPS:" in r),
    ("handoff-tell", "alpha",
     "Decision from the owner (glenn): the new reporting service uses PostgreSQL 17, not MySQL. "
     "This holds for the whole project. Acknowledge in one line.",
     None),
    ("handoff", "beta",
     "Which database (and version) should the new reporting service use? Answer in a few words.",
     lambda r: re.search(r"postgres", r, re.I) is not None and "17" in r),
    ("preference-tell", "alpha",
     "From glenn, for everyone in this project from now on: end every reply with the word ROGER on its own line. "
     "Acknowledge in one line.",
     None),
    ("preference", "beta",
     "Name one thing a code review in this project must include. One sentence.",
     lambda r: r.strip().splitlines()[-1].strip().strip("*.!").upper() == "ROGER" if r.strip() else False),
]


def run(cmd, env=None, check=True, cwd=None, timeout=None):
    p = subprocess.run(cmd, env=env, cwd=cwd, capture_output=True, text=True, timeout=timeout)
    if check and p.returncode != 0:
        raise RuntimeError(f"{' '.join(cmd[:4])}…: exit {p.returncode}: {p.stderr.strip()[-400:]}")
    return p.stdout


def build(ref, out):
    """Builds myco at ref (a git ref, or WORKTREE for the working tree)."""
    if ref == "WORKTREE":
        src = ROOT
    else:
        src = out + "-src"
        run(["git", "-C", ROOT, "worktree", "add", "--detach", src, ref])
    try:
        env = dict(os.environ, CGO_ENABLED="0")
        run(["go", "build", "-o", os.path.join(out, "myco"), "./cmd/myco"], env=env, cwd=src)
    finally:
        if src != ROOT:
            run(["git", "-C", ROOT, "worktree", "remove", "--force", src], check=False)
    return os.path.join(out, "myco")


class Arm:
    def __init__(self, name, binary, tmp, model):
        self.name, self.bin, self.tmp, self.model = name, binary, tmp, model
        self.procs, self.names = [], {}
        self.admin = "eval-" + secrets.token_hex(12)
        # Only this run's MYCO_* settings; the caller's (a real hub) are dropped.
        base = {k: v for k, v in os.environ.items() if not k.startswith("MYCO_")}
        base["PATH"] = os.path.dirname(binary) + os.pathsep + base.get("PATH", "")
        base["MYCO_FORMAT"] = "json"
        self.env = base

    def myco(self, *args, config=None, admin=False, timeout=None, check=True):
        env = dict(self.env, MYCO_HUB=self.hub)
        if admin:
            env["MYCO_ADMIN_TOKEN"] = self.admin
        cmd = [self.bin, *args] + (["--config", config] if config else [])
        return run(cmd, env=env, timeout=timeout, check=check)

    def start(self):
        log = open(os.path.join(self.tmp, "hub.log"), "w")
        env = dict(self.env, MYCO_ADMIN_TOKEN=self.admin)
        self.procs.append(subprocess.Popen([self.bin, "hub", "serve", "--addr", "127.0.0.1:0", "--db", os.path.join(self.tmp, "hub.db")],
                                           env=env, stdout=log, stderr=subprocess.STDOUT))
        for _ in range(100):
            m = re.search(r"addr=([0-9.:]+)", open(os.path.join(self.tmp, "hub.log")).read())
            if m:
                self.hub = "http://" + m.group(1)
                return
            time.sleep(0.1)
        raise RuntimeError("the hub did not start")

    def version(self):
        return json.loads(self.myco("version")).get("version", "?")

    def register(self, role, rnd):
        name = f"eval-{role}-{rnd}"
        cfg = os.path.join(self.tmp, f"{role}.yaml")
        harness = [] if role == "lead" else ["--harness", "claude"]
        self.myco("agent", "register", "--name", name, *harness, "--config", cfg, admin=True)
        self.names[role] = name
        return cfg

    def setup(self, rnd):
        self.cfg = {r: self.register(r, rnd) for r in ("lead", "alpha", "beta")}
        p = f"shop-{rnd}"
        self.project = p
        self.myco("project", "create", "--name", p, "--title", "acme shop", "--goal", "Ship the shop's new checkout", admin=True)
        self.myco("project", "add", "--name", p, "--agent", self.names["lead"], "--role", "coordinator", admin=True)
        for r in ("alpha", "beta"):
            self.myco("project", "add", "--name", p, "--agent", self.names[r], admin=True)
        bf = os.path.join(self.tmp, "brief.md")
        open(bf, "w").write(brief())
        self.myco("project", "update", "--name", p, "--brief-file", bf, config=self.cfg["lead"])
        # The coordinator records a convention the way this version allows.
        out = self.myco("project", "memory", "add", "--name", p, "--text", CONVENTION, config=self.cfg["lead"], check=False)
        self.memory = '"size"' in out
        if not self.memory:
            nf = os.path.join(self.tmp, "notes.md")
            open(nf, "w").write("# Notes\n\n- " + CONVENTION + "\n")
            self.myco("project", "doc", "put", "--name", p, "--doc", "notes", "--file", nf, config=self.cfg["lead"])
        for r in ("alpha", "beta"):
            wd = os.path.join(self.tmp, "w-" + r)
            os.makedirs(wd, exist_ok=True)
            args = [self.bin, "runner", "start", "--config", self.cfg[r], "--harness", "claude", "--workdir", wd]
            if self.model:
                args += ["--arg=--model", f"--arg={self.model}"]
            log = open(os.path.join(self.tmp, f"runner-{r}.log"), "w")
            self.procs.append(subprocess.Popen(args, env=dict(self.env, MYCO_HUB=self.hub), stdout=log, stderr=subprocess.STDOUT))
        for _ in range(300):
            if all("runner ready" in open(os.path.join(self.tmp, f"runner-{r}.log")).read() for r in ("alpha", "beta")):
                return
            time.sleep(0.1)
        raise RuntimeError("the runners did not start")

    def ask(self, to, body):
        t0 = time.time()
        out = self.myco("message", "send", "--to", self.names[to], "--project", self.project, "--body", body,
                        "--wait", "10m", config=self.cfg["lead"], timeout=700, check=False)
        took = time.time() - t0
        try:
            d = json.loads(out)
        except ValueError:
            return "", took, None, None
        rep = d.get("reply") if isinstance(d.get("reply"), dict) else {}
        reply = rep.get("body", "")
        mid = rep.get("reply_to") or (d.get("message") or {}).get("id") or d.get("id")
        return reply, took, mid, self.cost(mid)

    def cost(self, mid):
        if not mid:
            return None
        try:
            req = urllib.request.Request(f"{self.hub}/v1/messages/{mid}/trace", headers={"Authorization": "Bearer " + self.admin})
            tr = json.load(urllib.request.urlopen(req, timeout=10))
        except Exception:
            return None
        c = sum((e.get("data") or {}).get("cost_usd") or 0 for e in tr.get("events") or []
                if "usage" in (e.get("kind"), (e.get("data") or {}).get("kind")))
        return c or None

    def memory_body(self):
        if not self.memory:
            return None
        try:
            return json.loads(self.myco("project", "memory", "show", "--name", self.project, config=self.cfg["lead"])).get("body", "")
        except Exception:
            return None

    def stop(self):
        for p in self.procs:
            p.terminate()
        for p in self.procs:
            try:
                p.wait(timeout=10)
            except subprocess.TimeoutExpired:
                p.kill()
        self.procs = []
        # The runners keep their state next to the default config.
        for n in self.names.values():
            try:
                os.remove(os.path.join(os.path.expanduser("~"), ".myco", "state", n + ".json"))
            except OSError:
                pass


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--old", default=None, help="git ref of the baseline (default: the latest tag)")
    ap.add_argument("--new", default="WORKTREE", help="git ref to compare (default: the working tree)")
    ap.add_argument("--repeat", type=int, default=1)
    ap.add_argument("--model", default="", help="model for the agents (default: the CLI's)")
    ap.add_argument("--json", default="", help="also write the results here")
    ap.add_argument("--keep", action="store_true", help="keep the temporary directory (logs, hub databases)")
    a = ap.parse_args()
    if not shutil.which("claude"):
        sys.exit("needs the claude CLI, signed in")
    old = a.old or run(["git", "-C", ROOT, "describe", "--tags", "--abbrev=0"]).strip()
    tmp = tempfile.mkdtemp(prefix="myco-eval-")
    print(f"work dir: {tmp}")
    bins = {}
    for label, ref in (("old", old), ("new", a.new)):
        print(f"building {label} ({ref})…", flush=True)
        bins[label] = build(ref, os.path.join(tmp, label))

    results = []
    try:
        for i in range(a.repeat):
            for label in ("old", "new"):
                d = os.path.join(tmp, f"{label}-{i}")
                os.makedirs(d)
                arm = Arm(label, bins[label], d, a.model)
                try:
                    arm.start()
                    ver = arm.version()
                    arm.setup(secrets.token_hex(3))
                    print(f"\n[{label} {ver}, round {i + 1}] project memory: {'yes' if arm.memory else 'no (a document)'}", flush=True)
                    for tid, to, body, check in TASKS:
                        reply, took, mid, cost = arm.ask(to, body)
                        ok = check(reply) if check else None
                        mark = "—" if ok is None else ("PASS" if ok else "FAIL")
                        one = " ".join(reply.split())[:110]
                        print(f"  {tid:16} {mark:4} {took:5.0f}s {('$%.3f' % cost) if cost else '':>7}  {one}", flush=True)
                        results.append({"arm": label, "version": ver, "round": i + 1, "task": tid, "pass": ok,
                                        "seconds": round(took, 1), "cost_usd": cost, "reply": reply})
                    mem = arm.memory_body()
                    if mem is not None:
                        entries = [l for l in mem.splitlines() if l.startswith("- ")]
                        print(f"  memory at the end: {len(entries)} entries")
                        for l in entries:
                            print(f"    {l[:140]}")
                        results.append({"arm": label, "version": ver, "round": i + 1, "task": "_memory", "memory": mem})
                finally:
                    arm.stop()
    finally:
        if not a.keep:
            shutil.rmtree(tmp, ignore_errors=True)

    print("\npass rate (scored tasks):")
    tasks = [t for t, _, _, c in TASKS if c]
    print(f"  {'task':16} {'old':>8} {'new':>8}")
    for t in tasks + ["all"]:
        row = []
        for label in ("old", "new"):
            rs = [r for r in results if r["arm"] == label and r.get("pass") is not None and (t == "all" or r["task"] == t)]
            row.append(f"{sum(r['pass'] for r in rs)}/{len(rs)}" if rs else "-")
        print(f"  {t:16} {row[0]:>8} {row[1]:>8}")
    for label in ("old", "new"):
        rs = [r for r in results if r["arm"] == label and "seconds" in r]
        costs = [r["cost_usd"] for r in rs if r.get("cost_usd")]
        if rs:
            print(f"  {label}: {sum(r['seconds'] for r in rs) / len(rs):.0f}s a task" + (f", ${sum(costs) / len(costs):.3f} a task" if costs else ""))
    if a.json:
        with open(a.json, "w") as f:
            json.dump(results, f, ensure_ascii=False, indent=1)


if __name__ == "__main__":
    main()
