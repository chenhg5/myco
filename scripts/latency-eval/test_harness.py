import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("latency_harness", Path(__file__).with_name("harness.py"))
bench = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bench)


class HarnessTimingTest(unittest.TestCase):
    def invoke(self, events, code):
        program = "import sys\n" + "\n".join("print(" + repr(json.dumps(event)) + ",flush=True)" for event in events) + f"\nsys.exit({code})"
        with tempfile.TemporaryDirectory() as workdir, patch.object(bench, "command", return_value=[sys.executable, "-c", program]):
            return bench.run("claude", "private-prompt", workdir, 5)

    def test_error_assistant_is_not_a_successful_first_word(self):
        result, session = self.invoke([
            {"type": "assistant", "message": {"content": [{"type": "text", "text": "401 private-secret"}]}},
            {"type": "result", "is_error": True, "result": "401 private-secret", "session_id": "private-session"},
        ], 1)
        self.assertIsNone(result["first_text_ms"])
        self.assertEqual(result["failure"], "authentication")
        self.assertEqual(session, "")
        self.assertNotIn("private-", json.dumps(result))

    def test_session_metadata_is_not_model_work(self):
        result, session = self.invoke([
            {"type": "system", "subtype": "init", "session_id": "benchmark-session"},
            {"type": "stream_event", "event": {"type": "message_start"}},
            {"type": "assistant", "message": {"content": [{"type": "text", "text": "OK"}]}},
            {"type": "result", "is_error": False, "session_id": "benchmark-session"},
        ], 0)
        self.assertEqual(result["status"], "ok")
        self.assertTrue(result["reply_ok"])
        self.assertEqual(session, "benchmark-session")
        self.assertLessEqual(result["stamps_ms"]["session_announced"], result["stamps_ms"]["first_model_event"])
        self.assertNotIn("benchmark-session", json.dumps(result))


if __name__ == "__main__":
    unittest.main()
