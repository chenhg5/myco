#!/usr/bin/env python3
"""No-network preparation tests, including failed verification and pin rollback."""
from pathlib import Path
import argparse
import contextlib
import hashlib
import importlib.util
import io
import json
import tempfile
import unittest
from unittest.mock import patch
import zipfile

spec = importlib.util.spec_from_file_location('prepare_apps', Path(__file__).with_name('prepare-apps.py'))
prepare = importlib.util.module_from_spec(spec)
spec.loader.exec_module(prepare)


def archive(version='1.0.0', extra=None):
    stream = io.BytesIO()
    with zipfile.ZipFile(stream, 'w') as output:
        files = {'table/myco.json': json.dumps({'id': 'myco/table', 'name': 'Fixture', 'version': version}), 'table/index.html': 'fixture', 'table/agent.md': 'test', 'table/sample.json': '{}'}
        files.update(extra or {})
        for name, body in files.items():
            output.writestr(name, body)
    return stream.getvalue()


class PreparationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.source = self.root / 'downloaded'
        self.source.mkdir()
        self.catalog = {'myco/table': {'en': {'name': 'Fixture'}}}
        target = self.root / 'web/src/lib/app-locales.json'
        target.parent.mkdir(parents=True)
        target.write_text(json.dumps(self.catalog))
        self.lock = self.release('a', '1.0.0')
        self.options = argparse.Namespace(root=self.root, lock=None, cache_dir=None, artifacts_dir=self.source, offline=True, update_catalog=False, apps_dir=None, download_base=None)

    def release(self, revision, version):
        data = {'builtin.zip': archive(version), 'apps.zip': archive(version), 'builtin.json': b'["table"]', 'app-locales.json': json.dumps(self.catalog).encode()}
        lock = {'schema': 1, 'repository': 'myco-ai/apps', 'commit': revision * 40, 'tag': 'apps-' + revision * 40, 'builtin': ['table'], 'artifacts': {name: {'sha256': hashlib.sha256(body).hexdigest()} for name, body in data.items()}}
        for name, body in data.items():
            (self.source / name).write_bytes(body)
        (self.root / 'apps.lock.json').write_text(json.dumps(lock))
        return lock

    def run_prepare(self):
        with contextlib.redirect_stdout(io.StringIO()), patch.object(prepare.urllib.request, 'urlopen', side_effect=AssertionError('offline tried network')):
            prepare.prepare(self.options)

    def test_offline_empty_cache_fails_without_network(self):
        self.options.artifacts_dir = None
        with self.assertRaisesRegex(ValueError, 'Offline cache missing'):
            self.run_prepare()
        self.assertFalse((self.root / 'internal/templates/builtin_generated.go').exists())

    def test_exact_local_files_warm_cache_and_generate_real_embed(self):
        self.run_prepare()
        output = self.root / 'internal/templates/builtin.zip'
        self.assertEqual(prepare.digest(output), self.lock['artifacts']['builtin.zip']['sha256'])
        generated = (output.parent / 'builtin_generated.go').read_text()
        self.assertIn('//go:embed builtin.zip', generated)
        self.assertIn(self.lock['tag'], generated)
        before = output.stat().st_mtime_ns
        self.options.artifacts_dir = None
        self.run_prepare()
        self.assertEqual(output.stat().st_mtime_ns, before, 'cache hit must not invalidate Go build cache')

    def test_changed_pin_and_rollback_use_distinct_verified_cache_entries(self):
        self.run_prepare()
        old = self.lock
        self.release('b', '2.0.0')
        self.run_prepare()
        new_hash = prepare.digest(self.root / 'internal/templates/builtin.zip')
        self.assertNotEqual(new_hash, old['artifacts']['builtin.zip']['sha256'])
        (self.root / 'apps.lock.json').write_text(json.dumps(old))
        self.options.artifacts_dir = None
        self.run_prepare()
        self.assertEqual(prepare.digest(self.root / 'internal/templates/builtin.zip'), old['artifacts']['builtin.zip']['sha256'])

    def test_corrupted_input_preserves_previous_bundle(self):
        self.run_prepare()
        old_hash = prepare.digest(self.root / 'internal/templates/builtin.zip')
        self.release('b', '2.0.0')
        (self.source / 'builtin.zip').write_bytes(b'corrupted bytes')
        with self.assertRaisesRegex(ValueError, 'committed lock'):
            self.run_prepare()
        self.assertEqual(prepare.digest(self.root / 'internal/templates/builtin.zip'), old_hash)

    def test_corrupt_cache_refused_offline(self):
        self.run_prepare()
        file = self.root / '.cache/apps' / self.lock['artifacts']['builtin.zip']['sha256'] / 'builtin.zip'
        file.write_bytes(b'corrupt cache')
        self.options.artifacts_dir = None
        with self.assertRaisesRegex(ValueError, 'Offline cache missing'):
            self.run_prepare()

    def test_catalog_changes_require_explicit_reviewed_update(self):
        self.catalog['myco/table']['en']['name'] = 'New name'
        self.release('b', '2.0.0')
        with self.assertRaisesRegex(ValueError, 'Legacy UI catalog differs'):
            self.run_prepare()
        self.options.update_catalog = True
        self.run_prepare()
        self.assertEqual(json.loads((self.root / 'web/src/lib/app-locales.json').read_bytes()), self.catalog)

    def test_rolling_tag_and_changed_membership_are_rejected(self):
        self.lock['tag'] = 'apps'
        (self.root / 'apps.lock.json').write_text(json.dumps(self.lock))
        with self.assertRaisesRegex(ValueError, 'immutable'):
            self.run_prepare()
        self.lock['tag'] = 'apps-' + self.lock['commit']
        self.lock['builtin'] = ['table', 'unknown']
        (self.root / 'apps.lock.json').write_text(json.dumps(self.lock))
        with self.assertRaisesRegex(ValueError, 'builtin.json'):
            self.run_prepare()

    def test_archive_paths_and_extra_apps_are_rejected(self):
        file = self.root / 'bad.zip'
        file.write_bytes(archive(extra={'../escape': 'bad'}))
        with self.assertRaisesRegex(ValueError, 'Unsafe'):
            prepare.checked_archive(file, ['table'])
        file.write_bytes(archive(extra={'extra/file': 'bad'}))
        with self.assertRaisesRegex(ValueError, 'membership'):
            prepare.checked_archive(file, ['table'])


if __name__ == '__main__':
    unittest.main()
