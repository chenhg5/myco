"""No live CI calls: package partition, race terminal results and proof guards."""
import copy
import importlib.util
import json
import os
import subprocess
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("race", Path(__file__).with_name("ci-race.py"))
race = importlib.util.module_from_spec(spec)
spec.loader.exec_module(race)


class RaceCoverage(unittest.TestCase):
    def packages(self):
        return [{"name": name, "test_files": name != "c"} for name in "abcdef"]

    def value(self):
        packages = self.packages()
        return {"schema": 1, "source": "a" * 40, "tools": "b" * 40,
                "platform": {"GOOS": "linux", "GOVERSION": "go1.26.0"},
                "packages": packages, "shards": race.plan(packages, {"a": 296, "b": 271, "d": 238}, 3)}

    def reports(self, value):
        return [{"schema": 1, "index": i, "manifest": race.digest(value), "run_id": "123", "attempt": "1",
                 "command": ["go", "test", "-race", "-count=1", "-json", *names], "exit_code": 0, "errors": [],
                 "results": {name: {"status": "build_pass_no_tests" if name == "c" else "pass", "seconds": 1}
                             for name in names}} for i, names in enumerate(value["shards"], 1)]

    def test_plan_is_stable_complete_disjoint_and_accepts_new_packages(self):
        packages = self.packages()
        for count in (2, 3):
            result = race.plan(packages, {"a": 296, "b": 271, "d": 238}, count)
            self.assertEqual(result, race.plan(list(reversed(packages)), {"a": 296, "b": 271, "d": 238}, count))
            self.assertEqual(sorted(p for shard in result for p in shard), list("abcdef"))
            self.assertTrue(all(result))
        result = race.plan(packages, {"a": 296, "b": 271, "d": 238}, 3)
        self.assertEqual(len({next(i for i, shard in enumerate(result) if p in shard) for p in "abd"}), 3)

    def test_inventory_keeps_test_and_no_test_packages(self):
        data = {"ImportPath": "a", "TestGoFiles": ["a_test.go"]}, {"ImportPath": "b"}
        with patch.object(race, "output", return_value="\n".join(json.dumps(p) for p in data)):
            self.assertEqual(race.inventory(), [{"name": "a", "test_files": True}, {"name": "b", "test_files": False}])
        with patch.object(race, "output", return_value="{}"):
            with self.assertRaises(KeyError): race.inventory()
        for raw in ("", json.dumps(data[0]) * 2):
            with patch.object(race, "output", return_value=raw), self.assertRaises(ValueError): race.inventory()

    def test_default_packages_excluded_by_race_tags_cannot_disappear(self):
        race_data = json.dumps({"ImportPath": "a"})
        normal_data = race_data + json.dumps({"ImportPath": "excluded"})
        with patch.object(race, "output", side_effect=[race_data, normal_data]), self.assertRaisesRegex(ValueError, "excluded"):
            race.inventory()

    def test_invalid_weights_or_too_few_packages_fail(self):
        for weight in (-1, True, float("nan"), float("inf"), "1"):
            with self.subTest(weight=weight), self.assertRaises(ValueError):
                race.plan(self.packages(), {"a": weight}, 3)
        with self.assertRaises(ValueError): race.plan(self.packages()[:1], {}, 3)

    def test_package_pass_not_individual_test_pass_is_required(self):
        packages = self.packages()[:2]
        individual = [{"Action": "pass", "Package": "a", "Test": "TestOne"}]
        _, errors = race.check_results(packages, individual, 0)
        self.assertTrue(errors)
        events = [{"Action": "pass", "Package": p["name"], "Elapsed": 1} for p in packages]
        results, errors = race.check_results(packages, events, 0)
        self.assertFalse(errors)
        self.assertEqual(set(results), {"a", "b"})
        for changed in (events[:-1], events + events[:1], events + [{"Action": "pass", "Package": "foreign"}]):
            self.assertTrue(race.check_results(packages, changed, 0)[1])
        self.assertTrue(race.check_results(packages, events, 1)[1])
        self.assertTrue(race.check_results(packages, events + [
            {"Action": "fail", "Package": "a", "Test": "TestFailure"}], 0)[1])

    def test_no_tests_is_explicit_and_cannot_mask_tested_package_skip(self):
        events = [{"Action": "output", "Package": "c", "Output": "? c [no test files]\n"},
                  {"Action": "skip", "Package": "c", "Elapsed": 0}]
        result, errors = race.check_results([{"name": "c", "test_files": False}], events, 0)
        self.assertFalse(errors)
        self.assertEqual(result["c"]["status"], "build_pass_no_tests")
        self.assertTrue(race.check_results([{"name": "c", "test_files": True}], events, 0)[1])
        self.assertTrue(race.check_results([{"name": "c", "test_files": False}], events[1:], 0)[1])

    def test_all_shards_pass_and_failed_retry_cannot_lend_old_success(self):
        value = self.value(); reports = self.reports(value)
        self.assertEqual(race.verify(value, reports, "123"), set("abcdef"))
        # A failed retry or duplicate positive report cannot be ignored.
        reports[0]["exit_code"] = 1
        with self.assertRaises(ValueError): race.verify(value, reports, "123")
        for changed in (self.reports(value)[:-1], self.reports(value) + self.reports(value)[:1]):
            with self.assertRaises(ValueError): race.verify(value, changed, "123")

    def test_source_tools_platform_inventory_and_run_are_bound(self):
        value = self.value(); reports = self.reports(value)
        for key, changed in (("source", "c" * 40), ("tools", "d" * 40), ("platform", {}),
                             ("packages", value["packages"][:-1])):
            other = {**value, key: changed}
            with self.subTest(key=key), self.assertRaises(ValueError): race.verify(other, reports, "123")
        with self.assertRaises(ValueError): race.verify(value, reports, "another-run")

    def test_missing_failed_foreign_and_tampered_results_are_rejected(self):
        value = self.value()
        for mutation in ("missing", "failure", "foreign", "skip", "negative_time", "no_race", "filtered", "errors"):
            reports = self.reports(value); target = reports[0]; name = next(iter(target["results"]))
            if mutation == "missing": del target["results"][name]
            if mutation == "failure": target["results"][name]["status"] = "fail"
            if mutation == "foreign": target["results"]["other"] = {"status": "pass", "seconds": 1}
            if mutation == "skip": target["results"][name]["status"] = "build_pass_no_tests"
            if mutation == "negative_time": target["results"][name]["seconds"] = -1
            if mutation == "no_race": target["command"].remove("-race")
            if mutation == "filtered": target["command"].insert(2, "-run=TestOnly")
            if mutation == "errors": target["errors"] = ["failure"]
            with self.subTest(mutation=mutation), self.assertRaises(ValueError): race.verify(value, reports, "123")

    def test_caller_filters_and_agent_credentials_are_not_inherited(self):
        with patch.dict(os.environ, {"MYCO_TOKEN": "fake", "CLAUDE_MODEL": "fake", "GOFLAGS": "-short -run=One"}):
            env = race.test_environment()
        self.assertNotIn("MYCO_TOKEN", env)
        self.assertNotIn("CLAUDE_MODEL", env)
        self.assertEqual(env["GOFLAGS"], "")

    def test_expected_source_and_tools_must_match_both_checkouts(self):
        args = SimpleNamespace(source="a" * 40, tools="b" * 40, count=3, weights=Path("unused"))
        for identities in (("c" * 40, "b" * 40), ("a" * 40, "c" * 40)):
            with patch.object(race, "output", side_effect=identities), self.assertRaises(ValueError): race.manifest(args)

    def test_cli_runs_all_shards_and_independent_gate_on_source_without_helper(self):
        script = Path(race.__file__).resolve()
        tools = race.output("git", "rev-parse", "HEAD", cwd=script.parents[1])
        with tempfile.TemporaryDirectory(prefix="race source 空格 ") as temp:
            root = Path(temp)
            (root / "go.mod").write_text("module example.com/race-fixture\n\ngo 1.24\n")
            for name in "abcd":
                package = root / name; package.mkdir()
                tag = "//go:build race\n\n" if name == "d" else ""
                (package / "main.go").write_text(tag + "package " + name + "\nfunc Value() int { return 1 }\n")
                if name != "c":
                    (package / "main_test.go").write_text(tag + "package " + name + '\nimport "testing"\nfunc TestValue(t *testing.T) { if Value() != 1 { t.Fatal("wrong value") } }\n')
            def execute(command):
                env = race.test_environment()
                env.pop("GITHUB_STEP_SUMMARY", None)  # Fixture results must not enter real CI summaries.
                return subprocess.run(command, cwd=root, env=env, text=True,
                                      stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=60)
            for command in (["git", "init", "-q"], ["git", "add", "."],
                            ["git", "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "fixture"]):
                result = execute(command); self.assertEqual(result.returncode, 0, result.stdout)
            source = execute(["git", "rev-parse", "HEAD"]).stdout.strip()
            common = ["--source", source, "--tools", tools, "--run-id", "test", "--count", "3"]
            for index in (1, 2, 3):
                result = execute([os.sys.executable, str(script), "run", *common, "--index", str(index),
                                  "--out", str(root / ".results" / str(index))])
                self.assertEqual(result.returncode, 0, result.stdout)
            command = [os.sys.executable, str(script), "verify", *common, "--reports", str(root / ".results")]
            result = execute(command); self.assertEqual(result.returncode, 0, result.stdout)
            self.assertIn("4 packages, 1 compile-only", result.stdout)
            (root / ".results/1/manifest.json").unlink()
            self.assertNotEqual(execute(command).returncode, 0)


if __name__ == "__main__":
    unittest.main()
