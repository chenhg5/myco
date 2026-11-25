"""Source labels must advance with VERSION even when release tags leave main."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name("build-version.sh").resolve()


@unittest.skipIf(os.name == "nt", "The Make source-label helper uses a POSIX shell")
class BuildVersion(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.env = {**os.environ, "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1"}

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, env=self.env, text=True, stderr=subprocess.PIPE).strip()

    def label(self):
        return subprocess.check_output(["sh", str(SCRIPT)], cwd=self.root, env=self.env, text=True).strip()

    def initialize(self):
        self.git("init", "--quiet", "-b", "main")
        self.git("config", "user.name", "Example")
        self.git("config", "user.email", "example@example.com")
        self.git("config", "commit.gpgsign", "false")
        (self.root / "VERSION").write_text("v1.0.0\n")
        self.commit("Base")
        self.git("tag", "v1.0.0")

    def commit(self, subject):
        self.git("add", ".")
        self.git("commit", "--quiet", "-m", subject)

    def test_exact_development_and_dirty_labels(self):
        self.initialize()
        self.assertEqual(self.label(), "v1.0.0")
        (self.root / "feature.txt").write_text("feature\n")
        self.commit("Feature")
        self.assertRegex(self.label(), r"^v1\.0\.0-1-g[0-9a-f]+$")
        (self.root / "feature.txt").write_text("changed\n")
        self.assertTrue(self.label().endswith("-dirty"))

    def test_release_tag_outside_main_uses_synchronized_version(self):
        self.initialize()
        self.git("switch", "--quiet", "-c", "release/v1.0.1")
        (self.root / "VERSION").write_text("v1.0.1\n")
        self.commit("Release metadata")
        self.git("tag", "v1.0.1")
        self.git("switch", "--quiet", "main")
        (self.root / "feature.txt").write_text("later source\n")
        self.commit("Later source")
        (self.root / "VERSION").write_text("v1.0.1\n")
        self.commit("Synchronize published metadata")
        self.assertTrue(self.git("describe", "--tags").startswith("v1.0.0-"))
        self.assertRegex(self.label(), r"^v1\.0\.1-2-g[0-9a-f]+$")

    def test_archive_without_git_uses_recorded_version(self):
        (self.root / "VERSION").write_text("v1.2.3\n")
        self.assertEqual(self.label(), "v1.2.3")


if __name__ == "__main__":
    unittest.main()
