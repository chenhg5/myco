#!/usr/bin/env python3
"""Bounded post-publication Gitee mirror; dry-run probes public metadata only."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

VERSION = re.compile(r'v(\d+)\.(\d+)\.(\d+)')


class MirrorError(RuntimeError):
    pass


def version_tuple(value):
    match = VERSION.fullmatch(value or '')
    if not match:
        raise MirrorError('Invalid stable version')
    return tuple(map(int, match.groups()))


def blocked(status, response):
    return status == 403 and isinstance(response, dict) and ' '.join(str(response.get('message', '')).lower().strip().rstrip('.').split()) == 'malicious file detected and rejected'


def safe_assets(release):
    assets = release.get('assets', [])
    names = [a['name'] for a in assets]
    if not names or len(names) != len(set(names)) or 'SHA256SUMS' not in names:
        raise MirrorError('Missing/duplicate release assets')
    if any(not re.fullmatch(r'(?:myco[\w.-]*|SHA256SUMS)', n) for n in names):
        raise MirrorError('Unexpected asset filename')
    return assets


class Mirror:
    def __init__(self, repo, gitee_repo, budget):
        for name in (repo, gitee_repo):
            if not re.fullmatch(r'[\w.-]+/[\w.-]+', name):
                raise MirrorError('Invalid repository')
        self.repo, self.gitee_repo = repo, gitee_repo
        self.deadline = time.monotonic() + budget
        self.started = time.monotonic()

    def remaining(self):
        remaining = self.deadline - time.monotonic()
        if remaining <= 0:
            raise MirrorError('Mirror budget exhausted; GitHub publication remains valid')
        return max(1, min(90, int(remaining)))

    def github(self, path):
        p = subprocess.run(['gh', 'api', f'repos/{self.repo}/{path}'], capture_output=True, timeout=self.remaining())
        if p.returncode:
            raise MirrorError('GitHub metadata unavailable')
        return json.loads(p.stdout)

    def api(self, path, method='GET', data=None, file=None):
        token = os.environ.get('GITEE_TOKEN', '')
        if any(c in token for c in '\r\n"\\'):
            raise MirrorError('Invalid credential format')
        # Authentication travels through stdin config, never URL, argv, logs or artifacts.
        config = f'header = "Authorization: token {token}"\n' if token else ''
        with tempfile.TemporaryDirectory() as temp:
            body = Path(temp) / 'body.json'
            args = ['curl', '-q', '-sS', '--config', '-', '--max-time', str(self.remaining()),
                    '--connect-timeout', '15', '-X', method, '-o', str(body), '-w', '%{http_code}']
            if data is not None:
                payload = Path(temp) / 'input.json'
                payload.write_text(json.dumps(data, ensure_ascii=False))
                args += ['-H', 'Content-Type: application/json', '--data-binary', '@' + str(payload)]
            if file:
                args += ['-F', 'file=@' + str(file)]
            args.append(f'https://gitee.com/api/v5/repos/{self.gitee_repo}/{path}')
            p = subprocess.run(args, input=config, text=True, capture_output=True, timeout=self.remaining() + 2)
            if p.returncode or not re.fullmatch(r'\d{3}', p.stdout):
                raise MirrorError('Gitee request failed/unknown; no automatic mutation retry')
            if body.stat().st_size > 4 * 1024 * 1024:
                raise MirrorError('Gitee response exceeds budget')
            try:
                value = json.loads(body.read_text())
            except ValueError:
                value = None
            return int(p.stdout), value

    def newest(self, version):
        latest = self.github('releases/latest')
        return latest['tag_name'] == version

    def require_current(self, version):
        if not self.newest(version):
            raise MirrorError('Release superseded; stop before mutating Gitee')

    def tag(self, version, sha, temp):
        askpass = Path(temp) / 'askpass.py'
        askpass.write_text('#!/usr/bin/env python3\nimport os,sys\nprint(os.environ.get("GITEE_LOGIN", "oauth2") if "username" in sys.argv[1].lower() else os.environ["GITEE_TOKEN"])\n')
        askpass.chmod(0o700)
        env = {**os.environ, 'GIT_ASKPASS': str(askpass), 'GIT_TERMINAL_PROMPT': '0'}
        remote = f'https://gitee.com/{self.gitee_repo}.git'
        def git(*args):
            p = subprocess.run(['git', *args], env=env, text=True, capture_output=True, timeout=self.remaining())
            if p.returncode:
                raise MirrorError('Gitee tag transport failed (details suppressed)')
            return p.stdout.strip()
        refs = git('ls-remote', '--tags', remote, f'refs/tags/{version}', f'refs/tags/{version}^{{}}')
        values = dict(line.split()[::-1] for line in refs.splitlines())
        actual = values.get(f'refs/tags/{version}^{{}}', values.get(f'refs/tags/{version}'))
        if actual and actual != sha:
            raise MirrorError('Gitee tag points to different source; never move it')
        if not actual:
            self.require_current(version)
            git('push', remote, f'refs/tags/{version}:refs/tags/{version}')

    def run(self, version, dry_run):
        version_tuple(version)
        release = self.github('releases/tags/' + version)
        if release.get('draft') or release.get('prerelease') or not release.get('published_at'):
            raise MirrorError('Expected a published stable release')
        assets = safe_assets(release)
        if not self.newest(version):
            return {'superseded': True, 'version': version}
        status, remote = self.api('releases/tags/' + version)
        if status not in (200, 404):
            raise MirrorError(f'Gitee metadata read failed: HTTP {status}')
        if dry_run:
            return {'dry_run': True, 'version': version, 'assets': len(assets),
                    'total_bytes': sum(a['size'] for a in assets), 'gitee_http_status': status,
                    'remote_mutations': False, 'upload_throughput': 'unknown (no credential or upload)',
                    'elapsed_seconds': round(time.monotonic() - self.started, 3)}
        if not os.environ.get('GITEE_TOKEN'):
            raise MirrorError('release Environment GITEE_TOKEN is required')
        sha = self.github('commits/' + version)['sha']
        with tempfile.TemporaryDirectory() as temp:
            files = Path(temp) / 'assets'; files.mkdir()
            p = subprocess.run(['gh', 'release', 'download', version, '--repo', self.repo, '--dir', str(files)],
                               capture_output=True, timeout=max(1, self.deadline - time.monotonic()))
            if p.returncode:
                raise MirrorError('Asset download failed')
            expected = {}
            for line in (files / 'SHA256SUMS').read_text().splitlines():
                match = re.fullmatch(r'([0-9a-f]{64}) [ *](myco[\w.-]*)', line)
                if not match or match[2] in expected:
                    raise MirrorError('Invalid checksum manifest')
                expected[match[2]] = match[1]
            if set(expected) != {a['name'] for a in assets if a['name'] != 'SHA256SUMS'}:
                raise MirrorError('Checksum asset coverage mismatch')
            for asset in assets:
                path = files / asset['name']
                if path.stat().st_size != asset['size']:
                    raise MirrorError('Asset size mismatch')
                if asset['name'] in expected:
                    with path.open('rb') as stream:
                        digest = hashlib.file_digest(stream, 'sha256').hexdigest()
                    if digest != expected[asset['name']]:
                        raise MirrorError('Asset checksum mismatch')
            self.tag(version, sha, temp)
            pointer = f'Release notes / 发布说明: https://github.com/{self.repo}/releases/tag/{version}'
            body = {'tag_name': version, 'name': version, 'body': pointer}
            self.require_current(version)
            if remote:
                status, remote = self.api(f'releases/{remote["id"]}', 'PATCH', body)
            else:
                status, remote = self.api('releases', 'POST', {**body, 'target_commitish': sha, 'prerelease': False})
            if status not in (200, 201) or not isinstance(remote, dict) or not remote.get('id'):
                raise MirrorError('Gitee Release mutation failed/unknown; observe before retry')
            rid = remote['id']
            # Only older stable host releases may be pruned. Preserve apps and every non-host prerelease.
            for page in range(1, 21):
                status, batch = self.api(f'releases?per_page=100&page={page}')
                if status != 200 or not isinstance(batch, list):
                    raise MirrorError('Cannot enumerate Gitee releases')
                for old in batch:
                    old_tag = old.get('tag_name', '')
                    if not VERSION.fullmatch(old_tag) or old.get('prerelease') or version_tuple(old_tag) >= version_tuple(version):
                        continue
                    status, old_assets = self.api(f'releases/{old["id"]}/attach_files?per_page=100')
                    if status != 200 or not isinstance(old_assets, list) or len(old_assets) >= 100:
                        raise MirrorError('Incomplete old attachment inventory')
                    for asset in old_assets:
                        self.require_current(version)
                        status, _ = self.api(f'releases/{old["id"]}/attach_files/{asset["id"]}', 'DELETE')
                        if status not in (200, 204):
                            raise MirrorError('Old attachment deletion failed/unknown')
                    self.require_current(version)
                    status, _ = self.api(f'releases/{old["id"]}', 'PATCH', {'body': f'Release assets / 安装包: https://github.com/{self.repo}/releases/tag/{old_tag}'})
                    if status != 200:
                        raise MirrorError('Old download pointer update failed')
                if len(batch) < 100:
                    break
            else:
                raise MirrorError('Incomplete Gitee release inventory')
            status, have = self.api(f'releases/{rid}/attach_files?per_page=100')
            if status != 200 or not isinstance(have, list) or len(have) >= 100:
                raise MirrorError('Incomplete attachment inventory')
            indexed = {a['name']: a for a in have}
            rejected = []
            for asset in assets:
                name = asset['name']
                self.require_current(version)
                if name in indexed:
                    # Existing immutable release assets can resume by name/size; final manifest readback is mandatory.
                    if indexed[name]['size'] == asset['size']:
                        continue
                    status, _ = self.api(f'releases/{rid}/attach_files/{indexed[name]["id"]}', 'DELETE')
                    if status not in (200, 204):
                        raise MirrorError('Replacement deletion failed')
                status, response = self.api(f'releases/{rid}/attach_files', 'POST', file=files / name)
                if blocked(status, response):
                    rejected.append(name)
                elif status not in (200, 201):
                    raise MirrorError(f'Upload failed/unknown: HTTP {status} (not an accepted partial mirror)')
            if rejected:
                body = pointer + '\n\n' + '\n'.join(f'- [{name}](https://github.com/{self.repo}/releases/download/{version}/{name})' for name in rejected)
                status, response = self.api(f'releases/{rid}', 'PATCH', {'body': body})
                if status != 200 or any(f'/releases/download/{version}/{name}' not in (response.get('body') or '') for name in rejected):
                    raise MirrorError('Gitee did not preserve fallback links')
            status, have = self.api(f'releases/{rid}/attach_files?per_page=100')
            if status != 200 or {a['name']: a['size'] for a in have} != {a['name']: a['size'] for a in assets if a['name'] not in rejected}:
                raise MirrorError('Final Gitee asset set differs from verified GitHub release')
            if 'SHA256SUMS' not in rejected:
                req = urllib.request.Request(f'https://gitee.com/{self.gitee_repo}/releases/download/{version}/SHA256SUMS', headers={'User-Agent': 'myco-release'})
                with urllib.request.urlopen(req, timeout=self.remaining()) as response:
                    if response.read(1024 * 1024) != (files / 'SHA256SUMS').read_bytes():
                        raise MirrorError('Gitee manifest readback differs')
            return {'version': version, 'mirrored': True, 'partial': bool(rejected),
                    'blocked': rejected, 'elapsed_seconds': round(time.monotonic() - self.started, 3)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('--repo', default=os.environ.get('GITHUB_REPOSITORY', 'myco-ai/myco'))
    parser.add_argument('--gitee-repo', default=os.environ.get('GITEE_REPOSITORY', 'cg33/myco'))
    parser.add_argument('--budget', type=int, default=600)
    parser.add_argument('--dry-run', action='store_true')
    args = parser.parse_args()
    try:
        if not 1 <= args.budget <= 900:
            raise MirrorError('Budget must be 1..900 seconds')
        result = Mirror(args.repo, args.gitee_repo, args.budget).run(args.version, args.dry_run)
        print(json.dumps(result, ensure_ascii=False))
    except (MirrorError, OSError, ValueError, KeyError, subprocess.SubprocessError):
        # Never echo raw HTTP/transport exceptions, URLs with credentials or provider response bodies.
        print('::error::Mirror failed; GitHub remains published. Inspect/retry only the mirror job.')
        raise SystemExit(1)


if __name__ == '__main__':
    main()
