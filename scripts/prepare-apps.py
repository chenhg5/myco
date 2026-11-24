#!/usr/bin/env python3
"""Prepare exactly the official app artifacts pinned by apps.lock.json.

Only standard Python libraries are used. No network is attempted with --offline.
The cache is content addressed; a different lock can reuse an older verified pack.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import stat
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import zipfile

ROOT = Path(__file__).resolve().parent.parent
MAX_ARCHIVE = 128 << 20
MAX_EXPANDED = 256 << 20


def digest(file):
    value = hashlib.sha256()
    with file.open('rb') as stream:
        for part in iter(lambda: stream.read(1 << 20), b''):
            value.update(part)
    return value.hexdigest()


def atomic_write(file, data):
    if file.is_file() and file.read_bytes() == data:
        return
    file.parent.mkdir(parents=True, exist_ok=True)
    fd, name = tempfile.mkstemp(prefix='.' + file.name + '-', dir=file.parent)
    try:
        with os.fdopen(fd, 'wb') as stream:
            stream.write(data)
        os.replace(name, file)
    finally:
        Path(name).unlink(missing_ok=True)


def read_lock(path):
    raw = path.read_bytes()
    lock = json.loads(raw)
    if lock.get('schema') != 1 or not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', lock.get('repository', '')):
        raise ValueError('Invalid app lock schema or repository')
    if not re.fullmatch(r'[0-9a-f]{40}', lock.get('commit', '')) or lock.get('tag') != 'apps-' + lock['commit']:
        raise ValueError('App lock must pin an immutable apps-<full source SHA> tag')
    expected = {'apps.zip', 'builtin.zip', 'app-locales.json', 'builtin.json'}
    if set(lock.get('artifacts', {})) != expected:
        raise ValueError('App lock must identify all four release artifacts')
    for artifact in lock['artifacts'].values():
        if not re.fullmatch(r'[0-9a-f]{64}', artifact.get('sha256', '')):
            raise ValueError('App lock contains an invalid SHA256')
    names = lock.get('builtin', [])
    if not names or len(set(names)) != len(names) or any(not re.fullmatch(r'[a-z0-9-]+', name) for name in names):
        raise ValueError('App lock contains an invalid builtin set')
    return lock, raw


def fetch(url, destination, accept=None):
    headers = {'User-Agent': 'myco-apps-prepare'}
    if accept:
        headers['Accept'] = accept
    request = urllib.request.Request(url, headers=headers)
    with urllib.request.urlopen(request, timeout=60) as response, destination.open('wb') as output:
        total = 0
        while part := response.read(1 << 20):
            total += len(part)
            if total > MAX_ARCHIVE:
                raise ValueError('App artifact exceeds the download limit')
            output.write(part)


def download(base, lock, name, destination, github_fallback):
    error = None
    for attempt in range(3):
        try:
            fetch(base + '/' + lock['tag'] + '/' + name, destination)
            return
        except (OSError, urllib.error.URLError) as exc:
            error = exc
            if attempt < 2:
                time.sleep(attempt + 1)
    # GitHub release links sometimes return gateway errors while the asset API
    # works. This fallback always uses the locked apps repository, not myco.
    if github_fallback:
        request = urllib.request.Request(
            'https://api.github.com/repos/' + lock['repository'] + '/releases/tags/' + lock['tag'],
            headers={'User-Agent': 'myco-apps-prepare', 'Accept': 'application/vnd.github+json'},
        )
        with urllib.request.urlopen(request, timeout=30) as response:
            release = json.load(response)
        for asset in release.get('assets', []):
            if asset.get('name') == name:
                fetch(asset['url'], destination, 'application/octet-stream')
                return
    raise RuntimeError(f'Cannot download {name}: {error}')


def artifact(name, lock, cache, args):
    expected = lock['artifacts'][name]['sha256']
    file = cache / expected / name
    if file.is_file() and digest(file) == expected:
        return file
    if args.artifacts_dir:
        source = args.artifacts_dir / name
        if not source.is_file() or digest(source) != expected:
            raise ValueError(f'Local {name} does not match the committed lock')
        atomic_write(file, source.read_bytes())
        return file
    if args.offline:
        raise ValueError(f'Offline cache missing or invalid: {name}; provide the locked artifacts or warm the cache online')
    file.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix='.download-', dir=file.parent)
    os.close(fd)
    temporary = Path(temporary)
    try:
        base = args.download_base or 'https://github.com/' + lock['repository'] + '/releases/download'
        parsed = urllib.parse.urlsplit(base)
        if parsed.scheme not in ('http', 'https') or not parsed.netloc or parsed.username or parsed.query or parsed.fragment:
            raise ValueError('Download base must be a public HTTP(S) prefix without credentials or query parameters')
        download(base.rstrip('/'), lock, name, temporary, args.download_base is None)
        if digest(temporary) != expected:
            raise ValueError(f'SHA256 mismatch: {name}; existing prepared bundle was not replaced')
        os.replace(temporary, file)
    finally:
        temporary.unlink(missing_ok=True)
    return file


def checked_archive(file, names=None):
    archive = zipfile.ZipFile(file)
    seen, expanded = set(), 0
    for entry in archive.infolist():
        path = PurePosixPath(entry.filename)
        expanded += entry.file_size
        if path.is_absolute() or '..' in path.parts or '\\' in entry.filename or entry.filename in seen:
            archive.close()
            raise ValueError('Unsafe or duplicated app archive path')
        if stat.S_ISLNK(entry.external_attr >> 16) or expanded > MAX_EXPANDED:
            archive.close()
            raise ValueError('Unsafe app archive entry or expanded size')
        seen.add(entry.filename)
    if archive.testzip() is not None:
        archive.close()
        raise ValueError('App archive CRC verification failed')
    manifests = [name for name in seen if name.count('/') == 1 and name.endswith('/myco.json')]
    actual = set()
    for name in manifests:
        app = name.split('/')[0]
        manifest = json.loads(archive.read(name))
        if manifest.get('id') != 'myco/' + app:
            archive.close()
            raise ValueError('App id does not match its archive directory')
        for field, default in [('entry', 'index.html'), ('agent', 'agent.md'), ('sample', 'sample.json')]:
            relative = PurePosixPath(manifest.get(field, default))
            if relative.is_absolute() or '..' in relative.parts or app + '/' + str(relative) not in seen:
                archive.close()
                raise ValueError(f'Incomplete app: {app}/{field}')
        actual.add(app)
    if not actual or (names is not None and (actual != set(names) or {name.split('/')[0] for name in seen} != actual)):
        archive.close()
        raise ValueError('Builtin membership does not match the committed lock')
    return archive


def prepare(args):
    root = args.root.resolve()
    path = args.lock or root / 'apps.lock.json'
    lock, raw = read_lock(path)
    cache = args.cache_dir or root / '.cache/apps'
    files = {name: artifact(name, lock, cache, args) for name in ['builtin.zip', 'builtin.json', 'app-locales.json']}
    membership = json.loads(files['builtin.json'].read_bytes())
    if membership != lock['builtin']:
        raise ValueError('Released builtin.json does not match the committed lock')
    with checked_archive(files['builtin.zip'], lock['builtin']):
        pass
    catalog = json.loads(files['app-locales.json'].read_bytes())
    fallback = root / 'web/src/lib/app-locales.json'
    if not isinstance(catalog, dict) or not set('myco/' + name for name in lock['builtin']) <= set(catalog):
        raise ValueError('Released localized catalog is incomplete')
    if not fallback.is_file() or json.loads(fallback.read_bytes()) != catalog:
        if not args.update_catalog:
            raise ValueError('Legacy UI catalog differs from the lock; run --update-catalog, make web, and commit the result')
        atomic_write(fallback, (json.dumps(catalog, ensure_ascii=False, indent=2) + '\n').encode())
    if args.apps_dir:
        full = artifact('apps.zip', lock, cache, args)
        with checked_archive(full) as archive:
            # --apps-dir is an explicit output directory for cross-repo tests.
            # Build into a sibling, then replace, so no stale assets survive.
            args.apps_dir.parent.mkdir(parents=True, exist_ok=True)
            staging = Path(tempfile.mkdtemp(prefix='.apps-', dir=args.apps_dir.parent))
            try:
                archive.extractall(staging)
                if args.apps_dir.exists():
                    shutil.rmtree(args.apps_dir)
                os.replace(staging, args.apps_dir)
            finally:
                if staging.exists():
                    shutil.rmtree(staging)
    if path.read_bytes() != raw:
        raise ValueError('apps.lock.json changed during preparation; retry with one consistent lock')
    embedded = root / 'internal/templates'
    atomic_write(embedded / 'builtin.zip', files['builtin.zip'].read_bytes())
    generated = f'''// Code generated by scripts/prepare-apps.py; DO NOT EDIT.
package templates

import _ "embed"

//go:embed builtin.zip
var preparedBuiltinZip []byte

func init() {{
\tbuiltinZip = preparedBuiltinZip
\tbundledVersion = "{lock['tag']}"
}}
'''
    atomic_write(embedded / 'builtin_generated.go', generated.encode())
    print(f"Prepared {len(lock['builtin'])} builtin apps from {lock['tag']} (SHA256 verified)")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--offline', action='store_true', help='never attempt a network request')
    parser.add_argument('--cache-dir', type=Path, help='content-addressed artifact cache (default: .cache/apps)')
    parser.add_argument('--artifacts-dir', type=Path, help='pre-downloaded exact release assets for offline builds')
    parser.add_argument('--update-catalog', action='store_true', help='explicitly update the tracked legacy UI metadata')
    parser.add_argument('--apps-dir', type=Path, help='also unpack the verified full catalog for cross-repository tests')
    parser.add_argument('--download-base', help='explicit public mirror/test download prefix; lock hashes still apply')
    parser.add_argument('--lock', type=Path, help='explicit reviewed lock file (for rollback verification)')
    parser.add_argument('--root', type=Path, default=ROOT, help=argparse.SUPPRESS)
    args = parser.parse_args()
    try:
        prepare(args)
    except (OSError, ValueError, RuntimeError, zipfile.BadZipFile) as error:
        print(f'prepare-apps: {error}', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
