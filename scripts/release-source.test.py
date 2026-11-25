"""Release gates must reject missing/skipped jobs and use the latest retry."""
import importlib.util
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


if __name__ == "__main__":
    unittest.main()
