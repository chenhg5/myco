#!/usr/bin/env python3
"""Exercise mirror idempotency and fail-closed publication without GitHub writes."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('mirror-apps.sh')


class MirrorTests(unittest.TestCase):
    def run_mirror(self, corrupt=False, current=False):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            source = root / 'source'
            source.mkdir()
            names = ['apps.zip', 'builtin.zip', 'app-locales.json', 'builtin.json']
            for name in names:
                (source / name).write_bytes(('fixture: ' + name).encode())
            sums = ''.join(f'{hashlib.sha256((source / name).read_bytes()).hexdigest()}  {name}\n' for name in names)
            (source / 'SHA256SUMS').write_text(sums)
            if corrupt:
                (source / 'apps.zip').write_bytes(b'corrupted transfer')
            (root / 'gh').write_text('''#!/usr/bin/env python3
import json, os, pathlib, shutil, sys
args = sys.argv[1:]
root = pathlib.Path(os.environ['MIRROR_FIXTURE'])
with (root / 'calls').open('a') as output: output.write(json.dumps(args) + '\\n')
if args[:2] == ['release', 'download']:
    target = pathlib.Path(args[args.index('--dir') + 1])
    if args[args.index('--repo') + 1] == 'myco-ai/apps':
        for file in (root / 'source').iterdir(): shutil.copyfile(file, target / file.name)
    else: (target / 'SHA256SUMS').write_text((root / 'source/SHA256SUMS').read_text() if os.environ['MIRROR_CURRENT'] == '1' else 'old')
''')
            (root / 'gh').chmod(0o755)
            (root / 'sleep').write_text('#!/bin/sh\nexit 0\n')
            (root / 'sleep').chmod(0o755)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ['PATH'], MIRROR_FIXTURE=str(root), MIRROR_CURRENT='1' if current else '0', GITHUB_REPOSITORY='myco-ai/myco')
            result = subprocess.run(['bash', str(SCRIPT)], env=env, capture_output=True, text=True)
            calls = [json.loads(line) for line in (root / 'calls').read_text().splitlines()]
            writes = [args for args in calls if args[:2] in [['release', 'upload'], ['release', 'edit'], ['release', 'create']]]
            return result, writes

    def test_verified_assets_uploaded_before_checksum(self):
        result, writes = self.run_mirror()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([args[1] for args in writes], ['upload', 'upload', 'edit'])
        self.assertTrue(any(arg.endswith('/apps.zip') for arg in writes[0]))
        self.assertTrue(any(arg.endswith('/SHA256SUMS') for arg in writes[1]))

    def test_bad_checksum_leaves_previous_release_untouched(self):
        result, writes = self.run_mirror(corrupt=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(writes, [])

    def test_current_release_is_noop(self):
        result, writes = self.run_mirror(current=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(writes, [])


if __name__ == '__main__':
    unittest.main()
