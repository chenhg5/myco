#!/usr/bin/env python3
"""Balance race packages and require exact-source, complete per-package results."""
import argparse
from collections import deque
import hashlib
import json
import math
import os
from pathlib import Path
import re
import subprocess
import sys
import time


def test_environment():
    env = {k: v for k, v in os.environ.items() if not k.startswith(("MYCO_", "CLAUDE"))}
    # An inherited -run, -short or build tag must not silently reduce coverage.
    env["GOFLAGS"] = ""
    return env


def output(*args, cwd=None):
    return subprocess.check_output(args, cwd=cwd, env=test_environment(), text=True).strip()


def objects(stream):
    decoder = json.JSONDecoder()
    while stream.strip():
        value, end = decoder.raw_decode(stream.lstrip())
        yield value
        stream = stream.lstrip()[end:]


def inventory():
    records = list(objects(output("go", "list", "-race", "-json", "./...")))
    packages = [{"name": p["ImportPath"], "test_files": bool(p.get("TestGoFiles") or p.get("XTestGoFiles"))}
                for p in records]
    if not packages or len({p["name"] for p in packages}) != len(packages):
        raise ValueError("go list must return a nonempty unique package inventory")
    normal = {p["ImportPath"] for p in objects(output("go", "list", "-json", "./..."))}
    excluded = normal - {p["name"] for p in packages}
    if excluded:
        raise ValueError("Packages excluded from the race build: " + ", ".join(sorted(excluded)))
    return sorted(packages, key=lambda p: p["name"])


def plan(packages, weights, count):
    if count not in (2, 3) or not packages or len({p["name"] for p in packages}) != len(packages):
        raise ValueError("Invalid package inventory/shard count")
    if any(isinstance(v, bool) or not isinstance(v, (int, float)) or not math.isfinite(v) or v < 0
           for v in weights.values()):
        raise ValueError("Package weights must be finite nonnegative seconds")
    weight = lambda p: weights.get(p["name"], 30 if p["test_files"] else 1)
    # Shard 1 also runs vet, Python checks and smoke once (about 40 seconds).
    loads = [40.0] + [0.0] * (count - 1)
    shards = [[] for _ in range(count)]
    for package in sorted(packages, key=lambda p: (-weight(p), p["name"])):
        eligible = [i for i in range(count) if not shards[i]] or list(range(count))
        index = min(eligible, key=lambda i: (loads[i], i))
        shards[index].append(package["name"])
        loads[index] += weight(package)
    if any(not shard for shard in shards):
        raise ValueError("Every race shard must have packages")
    return [sorted(shard) for shard in shards]


def manifest(args):
    source = output("git", "rev-parse", "HEAD")
    tools = output("git", "rev-parse", "HEAD", cwd=Path(__file__).resolve().parents[1])
    for expected, actual in ((args.source, source), (args.tools, tools)):
        if not re.fullmatch(r"[0-9a-f]{40}", expected) or expected != actual:
            raise ValueError("Source/tools SHA does not match the checked-out commit")
    packages = inventory()
    weights = json.loads(args.weights.read_text())
    return {"schema": 1, "source": source, "tools": tools, "packages": packages,
            "platform": json.loads(output("go", "env", "-json", "GOVERSION", "GOOS", "GOARCH", "CGO_ENABLED", "GOEXPERIMENT")),
            "shards": plan(packages, weights, args.count)}


def digest(value):
    return hashlib.sha256(json.dumps(value, sort_keys=True, separators=(",", ":")).encode()).hexdigest()


def check_results(packages, events, code):
    expected = {p["name"]: p for p in packages}
    results, no_tests, errors = {}, set(), []
    for event in events:
        package = event.get("Package")
        if event.get("Test"):
            if event.get("Action") == "fail":
                errors.append(f"Failed test: {package}/{event['Test']}")
            continue
        if event.get("Action") == "output" and "[no test files]" in event.get("Output", ""):
            no_tests.add(package)
        if event.get("Action") not in ("pass", "fail", "skip"):
            continue
        if package not in expected or package in results:
            errors.append(f"Unexpected or duplicate terminal result: {package}")
            continue
        action = event["Action"]
        status = "pass" if action == "pass" else "fail"
        if action == "skip" and not expected[package]["test_files"] and package in no_tests:
            status = "build_pass_no_tests"
        results[package] = {"status": status, "seconds": event.get("Elapsed", 0)}
    if set(results) != set(expected):
        errors.append("Missing terminal results: " + ", ".join(sorted(set(expected) - set(results))))
    if any(r["status"] == "fail" for r in results.values()):
        errors.append("A package failed or a package with test files was skipped")
    if code != 0:
        errors.append(f"go test exited {code}")
    return results, errors


def run(args):
    value = manifest(args)
    assigned = value["shards"][args.index - 1]
    packages = [p for p in value["packages"] if p["name"] in assigned]
    args.out.mkdir(parents=True, exist_ok=True)
    if any((args.out / name).exists() for name in ("manifest.json", "report.json", "events.jsonl")):
        raise ValueError("Refusing to overwrite existing race results")
    (args.out / "manifest.json").write_text(json.dumps(value, indent=2) + "\n")
    command = ["go", "test", "-race", "-count=1", "-json", *assigned]
    started = time.monotonic()
    protocol_errors, tail = [], deque(maxlen=20)
    with (args.out / "events.jsonl").open("w") as raw:
        process = subprocess.Popen(command, env=test_environment(), stdin=subprocess.DEVNULL,
                                   stdout=subprocess.PIPE, text=True)
        for line in process.stdout:
            raw.write(line)
            try:
                event = json.loads(line)
                if event.get("Action") == "output":
                    tail.append(event.get("Output", "").rstrip())
                elif event.get("Action") in ("pass", "fail", "skip") and not event.get("Test"):
                    print(f"race: {event['Action']} {event.get('Package')} ({event.get('Elapsed', 0)}s)", flush=True)
            except (json.JSONDecodeError, AttributeError):
                protocol_errors.append("Invalid go test JSON event")
        code = process.wait()
    with (args.out / "events.jsonl").open() as raw:
        events = (json.loads(line) for line in raw) if not protocol_errors else []
        results, errors = check_results(packages, events, code)
    errors.extend(protocol_errors)
    report = {"schema": 1, "index": args.index, "manifest": digest(value), "run_id": args.run_id,
              "attempt": os.environ.get("GITHUB_RUN_ATTEMPT", "local"), "command": command,
              "exit_code": code, "results": results, "errors": errors,
              "seconds": round(time.monotonic() - started, 3)}
    (args.out / "report.json").write_text(json.dumps(report, indent=2) + "\n")
    if errors:
        print("\n".join(tail), file=sys.stderr)
        raise ValueError("; ".join(errors))
    print(f"Shard {args.index}: {len(results)} packages passed race; "
          f"{sum(r['status'] == 'build_pass_no_tests' for r in results.values())} have no test files")


def verify(value, reports, run_id):
    if len(reports) != len(value["shards"]):
        raise ValueError("Missing or extra race shard reports")
    seen, coverage = set(), set()
    lookup = {p["name"]: p for p in value["packages"]}
    for report in reports:
        index = report.get("index")
        if type(index) is not int or not 1 <= index <= len(value["shards"]) or index in seen:
            raise ValueError("Invalid or duplicate race shard index")
        seen.add(index)
        assigned = value["shards"][index - 1]
        if (report.get("schema") != 1 or report.get("manifest") != digest(value) or report.get("run_id") != run_id
                or type(report.get("exit_code")) is not int or report["exit_code"] != 0 or report.get("errors") != []
                or report.get("command") != ["go", "test", "-race", "-count=1", "-json", *assigned]
                or set(report.get("results", {})) != set(assigned)):
            raise ValueError("Race report source, command, success or assigned package coverage mismatch")
        for name, result in report["results"].items():
            if (not isinstance(result, dict) or set(result) != {"status", "seconds"}
                    or type(result["seconds"]) not in (int, float) or not math.isfinite(result["seconds"]) or result["seconds"] < 0
                    or name in coverage or result.get("status") not in ("pass", "build_pass_no_tests")):
                raise ValueError("Duplicate or unsuccessful package result")
            if result["status"] == "build_pass_no_tests" and lookup[name]["test_files"]:
                raise ValueError("A tested package cannot count as no-tests build success")
            coverage.add(name)
    if coverage != set(lookup):
        raise ValueError("Race package inventory is incomplete")
    return coverage


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("run", "verify"))
    parser.add_argument("--count", type=int, choices=(2, 3), default=3)
    parser.add_argument("--index", type=int, choices=(1, 2, 3))
    parser.add_argument("--weights", type=Path, default=Path(__file__).with_name("ci-race-weights.json"))
    parser.add_argument("--source", required=True)
    parser.add_argument("--tools", required=True)
    parser.add_argument("--run-id", required=True)
    parser.add_argument("--out", type=Path)
    parser.add_argument("--reports", type=Path)
    args = parser.parse_args()
    if args.action == "run":
        if not args.index or args.index > args.count or not args.out:
            parser.error("run needs an in-range --index and --out")
        run(args)
    else:
        if not args.reports:
            parser.error("verify needs --reports")
        value = manifest(args)  # Independent go list from the frozen checkout.
        reports = []
        for path in sorted(args.reports.glob("*/report.json")):
            if json.loads(path.with_name("manifest.json").read_text()) != value:
                raise ValueError("Race shard inventory differs from the independently listed frozen source")
            reports.append(json.loads(path.read_text()))
        coverage = verify(value, reports, args.run_id)
        no_tests = sum(not p["test_files"] for p in value["packages"])
        summary = f"Race coverage: {len(coverage)} packages, {no_tests} compile-only (no test files), all {args.count} shards passed."
        print(summary)
        if os.environ.get("GITHUB_STEP_SUMMARY"):
            with open(os.environ["GITHUB_STEP_SUMMARY"], "a") as stream:
                stream.write(summary + "\n\n| Package | Race result | Seconds |\n| --- | --- | --- |\n")
                for report in reports:
                    for name, result in sorted(report["results"].items()):
                        stream.write(f"| `{name}` | {result['status']} | {result['seconds']} |\n")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        print(f"::error::{error}", file=sys.stderr)
        sys.exit(1)
