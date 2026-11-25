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


class SourceResolution(unittest.TestCase):
    def resolve(self, files):
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
        green = lambda repo, value: {"id": 1} if value == parent else None
        with patch.object(source, "command", side_effect=command), patch.object(source, "green_run", side_effect=green), \
             patch.object(source, "github", return_value={"tag_name": "v1.2.3"}), patch.object(source.subprocess, "run"), \
             patch.dict(source.os.environ, {"GITHUB_REF": "refs/heads/main"}):
            return source.resolve("example/repo", sha)

    def test_metadata_only_commit_inherits_parent_ci(self):
        result = self.resolve(["VERSION", "CHANGELOG.md"])
        self.assertEqual(result["base"], "a" * 40)
        self.assertEqual(result["sha"], "b" * 40)

    def test_source_change_cannot_inherit_parent_ci(self):
        with self.assertRaises(ValueError):
            self.resolve(["VERSION", "CHANGELOG.md", "internal/app/changed.go"])


WORKFLOW = Path(__file__).resolve().parents[1] / ".github/workflows/release.yml"
CHECK = textwrap.dedent(WORKFLOW.read_text().split(
    "          # BEGIN release tag check", 1
)[1].split("          # END release tag check.", 1)[0].split("\n", 1)[1])


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
