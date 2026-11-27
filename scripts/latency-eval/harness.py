#!/usr/bin/env python3
"""Content-free Claude/Codex CLI timing on macOS and WSL (standard library).

Uses the existing CLI login/provider configuration. Creates only synthetic
benchmark conversations; never reads an existing conversation. No proxy,
network, provider, model, or reasoning-effort setting is changed.
"""
import argparse
import json
import os
from pathlib import Path
import selectors
import signal
import subprocess
import tempfile
import threading
import time


def failure_class(text):
    text = text.lower()
    for name, needles in (
        ("permission", ("eperm", "eacces", "operation not permitted", "permission denied", "read-only file")),
        ("authentication", ("401", "not logged in", "invalid api key", "authentication", "unauthorized", "login required")),
        ("rate_limit", ("429", "rate limit", "usage limit", "quota exceeded")),
        ("network", ("connection refused", "connection reset", "timed out", "tls", "network error", "fetch failed")),
    ):
        if any(n in text for n in needles):
            return name
    return "cli_failure"


def command(cli, session, ephemeral):
    if cli == "claude":
        args = [cli, "-p", "--output-format", "stream-json", "--verbose", "--include-partial-messages", "--tools", "", "--max-turns", "1"]
        if session:
            args += ["--resume", session]
        if ephemeral:
            args += ["--no-session-persistence"]
        return args
    args = [cli, "exec", "--json", "--skip-git-repo-check", "-c", 'sandbox_mode="read-only"']
    if ephemeral:
        args += ["--ephemeral"]
    if session:
        args += ["resume", session]
    return args + ["-"]


def run(cli, prompt, workdir, timeout, session="", ephemeral=False):
    env = os.environ.copy()
    # A benchmark child must not inherit the current myco task/session identity.
    for key in list(env):
        if key.startswith("MYCO_") or key in ("CLAUDECODE", "CLAUDE_CODE_SESSION_ID", "CLAUDE_CODE_REMOTE_SESSION_ID"):
            env.pop(key, None)
    start = time.monotonic()
    stamps = {}
    now = lambda: round((time.monotonic() - start) * 1000, 3)
    mark = lambda name: stamps.setdefault(name, now())
    errors, answer, child_session, input_tokens = [], "", "", None
    result_error, tool_used = False, False
    try:
        child = subprocess.Popen(command(cli, session, ephemeral), cwd=workdir, env=env, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
    except OSError as exc:
        return {"status": "start_failed", "failure": failure_class(str(exc)), "stamps_ms": stamps}, ""
    mark("spawned")

    def write_prompt():
        try:
            child.stdin.write(prompt.encode())
            child.stdin.close()
        except (BrokenPipeError, OSError):
            pass

    writer = threading.Thread(target=write_prompt, daemon=True)
    writer.start()
    select = selectors.DefaultSelector()
    select.register(child.stdout, selectors.EVENT_READ, "stdout")
    select.register(child.stderr, selectors.EVENT_READ, "stderr")
    pending = b""
    timed_out = False
    try:
        while select.get_map():
            if time.monotonic() - start > timeout:
                timed_out = True
                break
            for key, _ in select.select(.1):
                data = os.read(key.fileobj.fileno(), 65536)
                if not data:
                    select.unregister(key.fileobj)
                    continue
                if key.data == "stderr":
                    errors.append(data.decode(errors="replace")[-4096:])
                    errors = errors[-4:]
                    continue
                pending += data
                if len(pending) > 4 * 1024 * 1024:
                    pending = b""  # Bound diagnostics; never persist raw CLI output.
                while b"\n" in pending:
                    line, pending = pending.split(b"\n", 1)
                    try:
                        event = json.loads(line)
                    except (ValueError, UnicodeDecodeError):
                        continue
                    typ = event.get("type")
                    if not typ:
                        continue
                    mark("first_cli_event")
                    if typ == "thread.started" or typ == "system" and event.get("subtype") == "init":
                        mark("session_announced")
                        child_session = event.get("thread_id") or event.get("session_id") or child_session
                    if typ == "assistant":
                        if not event.get("isApiErrorMessage") and not event.get("error"):
                            mark("first_model_event")
                        for block in event.get("message", {}).get("content", []):
                            if block.get("type") == "text" and block.get("text"):
                                mark("first_text")
                                answer = block["text"]
                            if block.get("type") == "tool_use":
                                tool_used = True
                    if typ == "stream_event":
                        ev = event.get("event", {})
                        if ev.get("type") == "message_start":
                            mark("first_model_event")
                        delta = ev.get("delta", {})
                        if delta.get("type") == "text_delta" and delta.get("text"):
                            mark("first_text")
                    if typ.startswith("item."):
                        item = event.get("item", {})
                        if item.get("type") in ("agent_message", "reasoning", "command_execution", "mcp_tool_call", "file_change"):
                            mark("first_model_event")
                        if item.get("type") == "agent_message" and item.get("text"):
                            mark("first_text")
                            answer = item["text"]
                        if item.get("type") in ("command_execution", "mcp_tool_call", "file_change"):
                            tool_used = True
                    if typ == "result":
                        child_session = event.get("session_id") or child_session
                        result_error = bool(event.get("is_error"))
                        if result_error:
                            errors.append(str(event.get("result", ""))[-4096:])
                    if typ in ("error", "turn.failed"):
                        result_error = True
                        errors.append(str(event.get("error", event.get("message", "")))[-4096:])
                    if typ == "turn.completed":
                        input_tokens = event.get("usage", {}).get("input_tokens")
        if timed_out:
            try:
                os.killpg(child.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
        try:
            code = child.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(child.pid, signal.SIGKILL)
            code = child.wait()
    finally:
        # Only this benchmark's fresh process group, never pre-existing CLI
        # processes. Also runs when the enclosing exec node stops the script.
        if child.poll() is None:
            try:
                os.killpg(child.pid, signal.SIGTERM)
                child.wait(timeout=3)
            except ProcessLookupError:
                pass
            except subprocess.TimeoutExpired:
                os.killpg(child.pid, signal.SIGKILL)
                child.wait()
        select.close()
        for pipe in (child.stdout, child.stderr):
            pipe.close()
        writer.join(timeout=1)
    ok = code == 0 and not result_error and "first_text" in stamps and not tool_used
    status = "ok" if ok else "timeout" if timed_out else "tool_used" if tool_used else "failed"
    result = {"status": status, "exit_code": code, "failure": "" if ok else failure_class("\n".join(errors) + answer), "stamps_ms": stamps, "elapsed_ms": now(), "input_tokens": input_tokens}
    # An API-error assistant block is not a successful first-word observation.
    result["first_text_ms"] = stamps.get("first_text") if ok else None
    result["reply_ok"] = ok and answer.strip().strip(".! ").upper() == "OK"
    return result, child_session if ok else ""


def main():
    def stopped(_signum, _frame):
        raise SystemExit(143)

    signal.signal(signal.SIGTERM, stopped)
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cli", choices=("claude", "codex", "both"), default="both")
    parser.add_argument("--trials", type=int, default=3)
    parser.add_argument("--history-bytes", type=int, default=65536)
    parser.add_argument("--timeout", type=float, default=120)
    parser.add_argument("--ephemeral-only", action="store_true", help="New conversation only; no persisted session required")
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    if not 1 <= args.trials <= 10 or not 0 <= args.history_bytes <= 1048576:
        parser.error("trials must be 1..10 and history bytes 0..1048576")
    rows = []
    args.out.parent.mkdir(parents=True, exist_ok=True)
    limitations = ["CLI-only: App/Hub/runner stages are not measured here", "session_announced is not verified restore completion", "model/network/provider-queue time remains mixed", "failed runs have no valid first_text_ms", "existing provider/model/effort configuration is retained"]

    def save(complete=False):
        # Keep completed observations if the operator cancels a later run.
        with tempfile.NamedTemporaryFile(mode="w", dir=args.out.parent, prefix=args.out.name + ".", delete=False) as output:
            json.dump({"schema": 1, "complete": complete, "rows": rows, "limitations": limitations}, output, indent=2)
            temporary = output.name
        os.replace(temporary, args.out)

    def record(row):
        rows.append(row)
        save()
        print(json.dumps({k: row.get(k) for k in ("cli", "conversation", "trial", "status", "first_text_ms")}), flush=True)

    save()
    instruction = "Reply exactly OK. Do not use tools, inspect files, or run commands."
    with tempfile.TemporaryDirectory(prefix="myco-latency-") as workdir:
        for cli in ("claude", "codex") if args.cli == "both" else (args.cli,):
            for trial in range(args.trials):
                row, _ = run(cli, instruction, workdir, args.timeout, ephemeral=args.ephemeral_only)
                record(dict(cli=cli, conversation="new", trial=trial + 1, **row))
            if args.ephemeral_only:
                continue
            line = "Synthetic record: the quick brown fox counts seven blue boxes.\n"
            corpus = (line * (args.history_bytes // len(line) + 1))[:args.history_bytes]
            seed, session = run(cli, corpus + "\n" + instruction, workdir, args.timeout)
            record(dict(cli=cli, conversation="seed", trial=0, history_bytes=len(corpus.encode()), **seed))
            for trial in range(args.trials):
                if session:
                    row, _ = run(cli, instruction, workdir, args.timeout, session=session)
                else:
                    row = {"status": "skipped", "failure": "seed_failed", "first_text_ms": None}
                record(dict(cli=cli, conversation="long_resume", trial=trial + 1, history_bytes=len(corpus.encode()), **row))
    save(complete=True)
    print(json.dumps({"file": str(args.out), "runs": len(rows), "ok": sum(r["status"] == "ok" for r in rows)}))


if __name__ == "__main__":
    main()
