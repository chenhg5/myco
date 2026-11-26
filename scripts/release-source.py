#!/usr/bin/env python3
"""Resolve immutable release sources and require successful CI code jobs."""
import argparse
import json
import os
import re
import subprocess
from pathlib import Path

REQUIRED = {"go", "windows", "web", "app-macos"}
UI_COMBOS = ("1200-light", "1200-dark", "760-light", "760-dark")
UI_LEGACY = {f"ui ({combo})" for combo in UI_COMBOS}
UI_SINGLE = {f"ui ({combo} 1/1)" for combo in UI_COMBOS}
UI_SHARDED = {f"ui ({combo} {part}/3)" for combo in UI_COMBOS for part in (1, 2, 3)}


def command(*args):
    return subprocess.check_output(args, text=True, timeout=90).strip()


def github(repo, path):
    return json.loads(command("gh", "api", f"repos/{repo}/{path}"))


def green(jobs, *, full=False):
    # The latest result per job matters. Missing, skipped and cancelled are not passes.
    by_name = {job["name"]: job for job in sorted(jobs, key=lambda j: j["id"])}
    required = REQUIRED | ({"app-contract"} if full else set())
    if not all(by_name.get(name, {}).get("conclusion") == "success" for name in required):
        return False
    if full:
        ui = {name for name in by_name if name.startswith("ui (")}
        if ui not in (UI_LEGACY, UI_SINGLE, UI_SHARDED):
            return False
        return all(by_name[name].get("conclusion") == "success" for name in ui)
    return True


def green_run(repo, sha, *, full=False):
    page = 1
    while True:
        runs = github(repo, f"actions/workflows/ci.yml/runs?head_sha={sha}&branch=main&event=push&per_page=100&page={page}")["workflow_runs"]
        for run in runs:
            if full:
                if run.get("head_sha") != sha or run.get("head_branch") != "main" or run.get("event") != "push":
                    continue
                # A cancelled/incomplete/latest failed run cannot lend an older
                # success for the same SHA to a new publication.
                if run.get("status") != "completed" or run.get("conclusion") != "success":
                    return None
            jobs = []
            n = 1
            while True:
                batch = github(repo, f"actions/runs/{run['id']}/jobs?filter=latest&per_page=100&page={n}")["jobs"]
                jobs.extend(batch)
                if len(batch) < 100:
                    break
                n += 1
            if green(jobs, full=full):
                return run
            if full:
                return None
        if len(runs) < 100:
            return None
        page += 1


def resolve(repo, ref, *, sign_only=False, full_ci=False, defer_ci=False, force_ci=False):
    sha = command("git", "rev-parse", "--verify", f"{ref}^{{commit}}")
    version = command("git", "show", f"{sha}:VERSION")
    if not re.fullmatch(r"v\d+\.\d+\.\d+", version):
        raise ValueError("VERSION must be vMAJOR.MINOR.PATCH")
    if sign_only:
        return {"sha": sha, "version": version, "base": sha, "ci_run": ""}
    parents = command("git", "rev-list", "--parents", "-n", "1", sha).split()[1:]
    base = sha
    if len(parents) == 1:
        files = set(command("git", "diff", "--name-only", parents[0], sha).splitlines())
        if files == {"VERSION", "CHANGELOG.md"}:
            base = parents[0]
    subprocess.run(["git", "merge-base", "--is-ancestor", base, "origin/main"], check=True)
    if force_ci and not defer_ci:
        raise ValueError("--force-ci requires deferred qualification")
    run = None if force_ci else (green_run(repo, base, full=True) if full_ci else green_run(repo, base))
    if not run and not defer_ci:
        scope = "code, UI and app-contract jobs" if full_ci else "all four required jobs"
        raise ValueError(f"No main push CI with {scope} successful for {base}")
    section = command("git", "show", f"{sha}:CHANGELOG.md")
    match = re.search(rf"^## {re.escape(version)}\s*\n(.*?)(?=^## |\Z)", section, re.M | re.S)
    if not match or "**中文**" not in match[1] or not match[1].strip():
        raise ValueError("A bilingual CHANGELOG section for VERSION is required")
    latest = github(repo, "releases/latest")
    parse_version = lambda v: tuple(map(int, v.lstrip("v").split(".")))
    if parse_version(version) < parse_version(latest["tag_name"]):
        raise ValueError("VERSION is older than the latest published release")
    last_sha = command("git", "rev-parse", f"{latest['tag_name']}^{{commit}}")
    last_parents = command("git", "rev-list", "--parents", "-n", "1", last_sha).split()[1:]
    if len(last_parents) == 1 and set(command("git", "diff", "--name-only", last_parents[0], last_sha).splitlines()) == {"VERSION", "CHANGELOG.md"}:
        last_sha = last_parents[0]
    subprocess.run(["git", "merge-base", "--is-ancestor", last_sha, base], check=True)
    if os.environ.get("GITHUB_REF", "").startswith("refs/tags/") and os.environ["GITHUB_REF"][10:] != version:
        raise ValueError("Tag does not match VERSION")
    return {"sha": sha, "version": version, "base": base, "ci_run": str(run["id"]) if run else ""}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ref", required=True)
    parser.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY", "myco-ai/myco"))
    parser.add_argument("--sign-only", action="store_true")
    parser.add_argument("--full-ci", action="store_true", help="Require complete UI matrix and application contracts as well as code jobs")
    parser.add_argument("--defer-ci", action="store_true", help="Validate source/metadata now; missing CI proof must be supplied by the caller before publishing")
    parser.add_argument("--force-ci", action="store_true", help="Ignore existing proof and require the caller to run fresh deferred checks")
    parser.add_argument("--output", type=Path)
    args = parser.parse_args()
    result = resolve(args.repo, args.ref, sign_only=args.sign_only, full_ci=args.full_ci, defer_ci=args.defer_ci, force_ci=args.force_ci)
    if args.output:
        with args.output.open("a") as stream:
            for key, value in result.items():
                stream.write(f"{key}={value}\n")
    print(json.dumps(result))


if __name__ == "__main__":
    main()
