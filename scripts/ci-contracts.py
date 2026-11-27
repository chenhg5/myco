#!/usr/bin/env python3
"""Run native/browser contracts without installing system packages at job time."""
import argparse
from collections import deque
import json
import os
import shutil
from pathlib import Path
import subprocess
import sys

SUITES = {
    "internal/integration": ["TestAgentActionsSDK", "TestPageExportApplications", "TestWhiteboardSharedExport", "TestPaletteAgentToPage", "TestShoppingComparisonAgentToPage"],
    "internal/console": ["TestDatabaseNativeBridgeAndProductionUI"],
    "internal/sshcap": ["TestRealSSHOverviewTemplateParser"],
}


def verify(events, expected, code):
    results = {}
    for event in events:
        name, action = event.get("Test"), event.get("Action")
        if name in expected and action in ("pass", "fail", "skip"):
            if name in results:
                raise ValueError("Duplicate contract outcome: " + name)
            results[name] = action
        if action == "fail":
            raise ValueError("Contract fixture failed: " + (name or event.get("Package", "unknown")))
    if code or set(results) != set(expected) or any(v != "pass" for v in results.values()):
        raise ValueError("Contract fixtures must all execute and pass, with no skips: " + json.dumps(results))
    return results


def clean_env():
    return {k:v for k,v in os.environ.items() if not k.startswith(("MYCO_", "CLAUDE")) and k != "GOFLAGS"}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", required=True)
    parser.add_argument("--playwright-prefix", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    root = Path.cwd().resolve()
    apps = Path(os.environ["APPS_TEST_ROOT"]).resolve()
    prefix = args.playwright_prefix.resolve()
    args.out = args.out.resolve()
    args.out.mkdir(parents=True, exist_ok=False)
    env = clean_env()
    docker = ["docker", "run", "--rm", "--init", "--ipc=host", "--user", f"{os.getuid()}:{os.getgid()}",
              "--mount", f"type=bind,source={root},target={root}",
              "--mount", f"type=bind,source={args.out},target={args.out}",
              "--mount", f"type=bind,source={prefix},target={prefix},readonly",
              "--env", "HOME=/tmp", "--env", "CI=1", "--env", f"APPS_TEST_ROOT={apps}",
              "--env", f"NPM_CONFIG_PREFIX={prefix}"]
    # Prove the image has CJK fonts and can launch its bundled Chromium. The
    # fixtures also reject skips below, so missing browser dependencies fail CI.
    subprocess.run([*docker, "--workdir", str(root), args.image, "sh", "-ec",
                    "fc-list :lang=zh | grep -q .; node -e \"const p=require(process.env.NPM_CONFIG_PREFIX+'/lib/node_modules/playwright'); p.chromium.launch({headless:true,args:['--no-sandbox']}).then(b=>b.close()).catch(e=>{console.error(e.message);process.exit(1)})\""], env=env, check=True)
    # The real OpenSSH fixture needs the host's installed server and account;
    # it runs on Ubuntu itself, without a browser or container account changes.
    for program in ("sshd", "ssh-keygen", "node"):
        if not shutil.which(program):
            raise ValueError("Missing host contract dependency: " + program)
    all_results = {}
    for package, expected in SUITES.items():
        executable = args.out / (package.replace("/", "-") + ".test")
        subprocess.run(["go", "test", "-c", "-o", str(executable), "./" + package], env=env, check=True)
        module = subprocess.check_output(["go", "list", "-m"], env=env, text=True).strip()
        execution = [str(executable)] if package == "internal/sshcap" else [
            *docker, "--workdir", str(root / package), args.image, str(executable)]
        command = ["go", "tool", "test2json", "-p", module + "/" + package, "-t", *execution,
                   "-test.v=test2json", "-test.count=1", "-test.timeout=10m", "-test.run=^(" + "|".join(expected) + ")$"]
        path = args.out / (package.replace("/", "-") + ".jsonl")
        tail = deque(maxlen=20)
        process = subprocess.Popen(command, env=env, cwd=root / package, stdout=subprocess.PIPE, text=True)
        with path.open("w") as stream:
            for line in process.stdout:
                stream.write(line)
                try:
                    event = json.loads(line)
                    if event.get("Action") == "output":
                        tail.append(event.get("Output", "").strip())
                except json.JSONDecodeError:
                    pass  # The final verifier below fails malformed output.
        code = process.wait()
        try:
            results = verify([json.loads(line) for line in path.read_text().splitlines()], expected, code)
        except (ValueError, json.JSONDecodeError):
            print("\n".join(tail), file=sys.stderr)
            raise
        all_results.update(results)
        print(package + ": " + ", ".join(results), flush=True)
    (args.out / "results.json").write_text(json.dumps(all_results, indent=2) + "\n")
    print(f"All {len(all_results)} native/application contract fixtures executed and passed; no apt or browser download.")
    if os.environ.get("GITHUB_STEP_SUMMARY"):
        with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as stream:
            stream.write("## Native/application contracts\n\n" + "\n".join("- PASS `" + name + "`" for name in all_results) + "\n")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, OSError, subprocess.SubprocessError) as error:
        print(f"::error::{error}", file=sys.stderr)
        sys.exit(1)
