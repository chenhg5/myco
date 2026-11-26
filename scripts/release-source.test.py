"""Regression checks for release source selection and the inline tag gate."""
import importlib.util
import os
import subprocess
import tempfile
import textwrap
from pathlib import Path
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("release_source", Path(__file__).with_name("release-source.py"))
source = importlib.util.module_from_spec(spec)
spec.loader.exec_module(source)


class Gates(unittest.TestCase):
    def jobs(self):
        return [{"id": n, "name": name, "conclusion": "success"} for n, name in enumerate(sorted(source.REQUIRED))]

    def test_all_four(self):
        self.assertTrue(source.green(self.jobs()))

    def test_missing_or_skipped_is_not_green(self):
        jobs = self.jobs()
        self.assertFalse(source.green(jobs[:-1]))
        jobs[-1]["conclusion"] = "skipped"
        self.assertFalse(source.green(jobs))

    def test_latest_retry_failure_overrides_old_success(self):
        jobs = self.jobs()
        self.assertFalse(source.green(jobs + [{**jobs[0], "id": 100, "conclusion": "failure"}]))
        self.assertTrue(source.green([{**jobs[0], "id": -1, "conclusion": "failure"}] + jobs))

    def full_jobs(self, ui=source.UI_SHARDED):
        return [{"id": n, "name": name, "conclusion": "success"}
                for n, name in enumerate(sorted(source.REQUIRED | {"app-contract"} | ui))]

    def test_complete_legacy_and_sharded_matrices(self):
        self.assertTrue(source.green(self.full_jobs(), full=True))
        self.assertTrue(source.green(self.full_jobs(source.UI_LEGACY), full=True))
        self.assertTrue(source.green(self.full_jobs(source.UI_SINGLE), full=True))

    def test_code_only_success_is_not_full_proof(self):
        self.assertFalse(source.green(self.jobs(), full=True))
        self.assertFalse(source.green(self.full_jobs(set()), full=True))

    def test_each_ui_shard_and_contract_must_pass(self):
        jobs = self.full_jobs()
        for name in source.UI_SHARDED | {"app-contract"}:
            with self.subTest(name=name):
                self.assertFalse(source.green([j for j in jobs if j["name"] != name], full=True))
                for conclusion in ("skipped", "cancelled", "failure", None):
                    changed = [{**j, "conclusion": conclusion} if j["name"] == name else j for j in jobs]
                    self.assertFalse(source.green(changed, full=True))

    def test_unknown_ui_matrix_cannot_count_as_complete(self):
        self.assertFalse(source.green(self.full_jobs() + [
            {"id": 100, "name": "ui (new-combo)", "conclusion": "success"}], full=True))

    def test_latest_full_job_retry_overrides_success(self):
        jobs = self.full_jobs()
        self.assertFalse(source.green(jobs + [{**jobs[-1], "id": 100, "conclusion": "failure"}], full=True))


class CompleteRunProof(unittest.TestCase):
    jobs = Gates.jobs
    full_jobs = Gates.full_jobs

    def run_record(self, **fields):
        return {"id": 42, "head_sha": "a" * 40, "head_branch": "main", "event": "push",
                "status": "completed", "conclusion": "success", **fields}

    def test_exact_source_with_complete_matrix(self):
        def api(repo, path):
            return {"workflow_runs": [self.run_record()]} if "workflows/" in path else {"jobs": self.full_jobs()}
        with patch.object(source, "github", side_effect=api):
            self.assertEqual(source.green_run("example/repo", "a" * 40, full=True)["id"], 42)

    def test_failed_or_pending_latest_run_cannot_borrow_old_proof(self):
        for change in ({"conclusion": "cancelled"}, {"conclusion": "failure"}, {"status": "in_progress"}):
            with self.subTest(change=change), patch.object(source, "github", return_value={
                    "workflow_runs": [self.run_record(**change), self.run_record(id=41)]}) as api:
                self.assertIsNone(source.green_run("example/repo", "a" * 40, full=True))
                self.assertEqual(api.call_count, 1)

    def test_other_source_branch_or_event_cannot_supply_proof(self):
        for change in ({"head_sha": "b" * 40}, {"head_branch": "topic"}, {"event": "workflow_dispatch"}):
            with self.subTest(change=change), patch.object(source, "github", return_value={
                    "workflow_runs": [self.run_record(**change)]}):
                self.assertIsNone(source.green_run("example/repo", "a" * 40, full=True))

    def test_successful_workflow_with_skipped_ui_is_requalified(self):
        def api(repo, path):
            return {"workflow_runs": [self.run_record()]} if "workflows/" in path else {"jobs": self.jobs()}
        with patch.object(source, "github", side_effect=api):
            self.assertIsNone(source.green_run("example/repo", "a" * 40, full=True))


class SourceResolution(unittest.TestCase):
    def resolve(self, files, **options):
        sha, parent, last = "b" * 40, "a" * 40, "c" * 40
        def command(*args):
            if args[:3] == ("git", "rev-parse", "--verify"):
                return sha
            if args == ("git", "show", sha + ":VERSION"):
                return "v1.2.4"
            if args == ("git", "rev-list", "--parents", "-n", "1", sha):
                return sha + " " + parent
            if args[:3] == ("git", "diff", "--name-only"):
                return "\n".join(files)
            if args == ("git", "show", sha + ":CHANGELOG.md"):
                return "## v1.2.4\n\n- Fix\n\n**中文**\n- 修复\n"
            if args[:2] == ("git", "rev-parse"):
                return last
            if args == ("git", "rev-list", "--parents", "-n", "1", last):
                return last
            raise AssertionError(args)
        green = lambda repo, value, **kwargs: {"id": 1} if value == parent else None
        with patch.object(source, "command", side_effect=command), patch.object(source, "green_run", side_effect=green), \
             patch.object(source, "github", return_value={"tag_name": "v1.2.3"}), patch.object(source.subprocess, "run"), \
             patch.dict(source.os.environ, {"GITHUB_REF": "refs/heads/main"}):
            return source.resolve("example/repo", sha, **options)

    def test_metadata_only_commit_inherits_parent_ci(self):
        result = self.resolve(["VERSION", "CHANGELOG.md"])
        self.assertEqual(result["base"], "a" * 40)
        self.assertEqual(result["sha"], "b" * 40)

    def test_source_change_cannot_inherit_parent_ci(self):
        with self.assertRaises(ValueError):
            self.resolve(["VERSION", "CHANGELOG.md", "internal/app/changed.go"])

    def test_missing_proof_can_only_defer_when_explicit(self):
        result = self.resolve(["VERSION", "CHANGELOG.md", "internal/app/changed.go"], full_ci=True, defer_ci=True)
        self.assertEqual(result["base"], "b" * 40)
        self.assertEqual(result["ci_run"], "")

    def test_forced_rehearsal_requires_fresh_checks(self):
        result = self.resolve(["VERSION", "CHANGELOG.md"], full_ci=True, defer_ci=True, force_ci=True)
        self.assertEqual(result["ci_run"], "")
        with self.assertRaises(ValueError):
            self.resolve(["VERSION", "CHANGELOG.md"], force_ci=True)


WORKFLOW = Path(__file__).resolve().parents[1] / ".github/workflows/release.yml"
CHECK = textwrap.dedent(WORKFLOW.read_text().split(
    "          # BEGIN release tag check", 1
)[1].split("          # END release tag check.", 1)[0].split("\n", 1)[1])


class UIShardCompatibility(unittest.TestCase):
    def test_legacy_script_runs_each_window_once_and_new_script_uses_three_parts(self):
        ci = WORKFLOW.with_name("ci.yml").read_text()
        prefix = textwrap.dedent(ci.split("        run: |\n", 1)[1].split("          if [ \"$FORCE_FULL\"", 1)[0])
        for script, count in (("async function runMatrix() {}", "1"), ("const SHARD = {};", "3")):
            with self.subTest(count=count), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                (root / "web/scripts").mkdir(parents=True)
                (root / "web/scripts/ui-smoke.mjs").write_text(script)
                output = root / "output"
                subprocess.run(["bash", "-euo", "pipefail", "-c", prefix], cwd=root,
                               env={**os.environ, "GITHUB_OUTPUT": str(output)}, check=True)
                values = dict(line.split("=", 1) for line in output.read_text().splitlines())
                self.assertEqual(values["parts"], count)
                self.assertEqual(values["shards"], "[1]" if count == "1" else "[1,2,3]")


class ReleaseTags(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.repo = self.root / "source"
        self.remote = self.root / "remote.git"
        self.repo.mkdir()
        self.git("init", "--bare", str(self.remote))
        self.git("init")
        self.git("config", "user.name", "Example Developer")
        self.git("config", "user.email", "developer@example.com")
        self.git("remote", "add", "origin", str(self.remote))
        self.git("commit", "--allow-empty", "-m", "First")
        self.first = self.git("rev-parse", "HEAD").strip()
        self.git("commit", "--allow-empty", "-m", "Release")
        self.sha = self.git("rev-parse", "HEAD").strip()

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.repo, text=True, stderr=subprocess.DEVNULL)

    def tag(self, name, sha, annotated=False):
        args = ["tag"]
        if annotated:
            args += ["-a", "-m", "Release"]
        self.git(*args, name, sha)
        self.git("push", "origin", "refs/tags/" + name)

    def check(self, **env):
        return subprocess.run(["bash", "-euo", "pipefail", "-c", CHECK], cwd=self.repo,
                              env={**os.environ, "v": "v1.2.3", "RELEASE_SHA": self.sha, **env},
                              capture_output=True, text=True)

    def test_absent_tag_and_similarly_named_tag(self):
        self.tag("v1.2.30", self.first)
        result = self.check()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_matching_lightweight_and_annotated_tags(self):
        for annotated in (False, True):
            with self.subTest(annotated=annotated):
                name = "v1.2." + str(4 + annotated)
                self.tag(name, self.sha, annotated)
                result = self.check(v=name)
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_conflicting_lightweight_and_annotated_tags(self):
        for annotated in (False, True):
            with self.subTest(annotated=annotated):
                name = "v1.2." + str(4 + annotated)
                self.tag(name, self.first, annotated)
                result = self.check(v=name)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("tag already points to a different commit", result.stdout)

    def test_unreachable_remote_fails_closed(self):
        self.git("remote", "set-url", "origin", str(self.root / "missing.git"))
        result = self.check()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("could not query release tag", result.stdout)

    def test_query_error_body_is_never_a_tag(self):
        # The original gh query captured HTTP error JSON on stdout despite
        # failure. Also reject malformed output even if a query exits zero.
        bin_dir = self.root / "bin"
        bin_dir.mkdir()
        git = bin_dir / "git"
        for status in (1, 0):
            with self.subTest(status=status):
                git.write_text('#!/bin/sh\nprintf \'{"message":"failure","status":422}\\n\'\nexit ' + str(status) + '\n')
                git.chmod(0o755)
                result = self.check(PATH=str(bin_dir) + os.pathsep + os.environ["PATH"])
                self.assertNotEqual(result.returncode, 0)
                expected = "could not query release tag" if status else "invalid release tag query result"
                self.assertIn(expected, result.stdout)
                self.assertNotIn("tag already points", result.stdout)


if __name__ == "__main__":
    unittest.main()
