#!/usr/bin/env python3
"""Check release secret isolation, explicit job permissions and immutable action pins."""
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[1]


def jobs(text):
    body = text.split('\njobs:\n', 1)[1]
    return [(m[1], m[2]) for m in re.finditer(r'^  ([\w-]+):\n(.*?)(?=^  [\w-]+:\n|\Z)', body, re.M | re.S)]


class WorkflowPolicy(unittest.TestCase):
    def test_actions_are_pinned_and_job_permissions_explicit(self):
        for path in (ROOT / '.github/workflows').glob('*.yml'):
            text = path.read_text()
            for action in re.findall(r'uses:\s*(\S+)', text):
                if not action.startswith('./'):
                    self.assertRegex(action, r'^[\w.-]+/[\w./-]+@[0-9a-f]{40}$', str(path))
            for name, body in jobs(text):
                self.assertRegex(body, r'(?m)^    permissions:', str(path) + ':' + name)
                self.assertNotIn('write-all', body)

    def test_pull_request_has_no_release_environment_or_signing_secrets(self):
        for path in (ROOT / '.github/workflows').glob('*.yml'):
            text = path.read_text()
            pr = bool(re.search(r'^  pull_request(?:_target)?:', text, re.M))
            if pr:
                self.assertNotRegex(text, r'(?m)^    environment:')
                self.assertNotRegex(text, r'secrets\.(?:GITEE_TOKEN|MAC_CERT_\w+|NOTARY_\w+)')
                self.assertNotIn('secrets: inherit', text)
            for name, body in jobs(text):
                if re.search(r'secrets\.(?:GITEE_TOKEN|MAC_CERT_\w+|NOTARY_\w+)', body):
                    self.assertRegex(body, r'(?m)^    environment: release$', str(path) + ':' + name)
                    self.assertFalse(pr)

    def test_manual_planning_does_not_reference_environment(self):
        text = (ROOT / '.github/workflows/release-orchestrate.yml').read_text()
        self.assertNotIn('secrets.', text)
        self.assertNotRegex(text, r'(?m)^    environment:')
        self.assertNotRegex(text, r'(?m)^  (schedule|pull_request|push):')
        self.assertIn("github.ref == 'refs/heads/main'", text)
        mapping = dict(jobs(text))
        self.assertIn('contents: read', mapping['plan'])
        self.assertNotIn('contents: write', mapping['plan'])
        self.assertNotIn('actions: write', mapping['plan'])
        self.assertIn('if: ${{ !inputs.dry_run }}', mapping['dispatch'])

    def test_mirror_is_post_publish_and_independent_of_metadata_sync(self):
        mapping = dict(jobs((ROOT / '.github/workflows/release.yml').read_text()))
        self.assertIn('needs: [version, release]', mapping['mirror'])
        self.assertNotIn('mirror', mapping['metadata_sync'])
        self.assertIn('timeout-minutes: 12', mapping['mirror'])
        self.assertIn('environment: release', mapping['app'])


if __name__ == '__main__':
    unittest.main()
