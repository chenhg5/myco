#!/usr/bin/env python3
"""Run Android API tests against an isolated, real Hub; never touch ~/.myco."""
import argparse
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import tempfile
import time
import urllib.request
from urllib.parse import urlparse

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--binary', default='myco')
parser.add_argument('command', nargs=argparse.REMAINDER)
args = parser.parse_args()
command = args.command[1:] if args.command[:1] == ['--'] else args.command
if not command:
    parser.error('provide a command after --')
with tempfile.TemporaryDirectory(prefix='android-hub-', dir=os.environ.get('TMPDIR')) as folder:
    root = Path(folder)
    admin = secrets.token_hex(32)
    admin_file = root / 'admin'
    admin_file.write_text(admin)
    admin_file.chmod(0o600)
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    url = f'http://127.0.0.1:{port}'
    env = {k: v for k, v in os.environ.items() if not k.startswith(('MYCO_', 'CLAUDE'))}
    env['MYCO_NO_UPDATE_CHECK'] = '1'
    with (root / 'server.log').open('w') as log:
        server = subprocess.Popen([args.binary, 'hub', 'serve', '--addr', f'127.0.0.1:{port}', '--db', str(root / 'hub.db'), '--admin-token-file', str(admin_file), '--no-update-check'], stdout=log, stderr=log, env=env)
        def request(path, body=None):
            payload = None if body is None else json.dumps(body).encode()
            req = urllib.request.Request(url + path, data=payload, headers={'Authorization': 'Bearer ' + admin, 'Content-Type': 'application/json'})
            with urllib.request.urlopen(req, timeout=10) as response:
                return json.load(response)
        try:
            for _ in range(100):
                if server.poll() is not None:
                    raise RuntimeError('Fixture Hub exited during startup')
                try:
                    request('/v1/health')
                    break
                except OSError:
                    time.sleep(.1)
            else:
                raise RuntimeError('Fixture Hub did not become ready')
            agent = request('/v1/agents', {'name': 'android-test', 'description': 'An isolated Android test agent'})
            pairing = request('/v1/console/me/pairings', {})
            code = urlparse(pairing['link']).path.rsplit('/', 1)[1]
            paired = request('/v1/console/pair', {'code': code, 'device': 'Android API tests'})
            fixture = root / 'fixture.json'
            fixture.write_text(json.dumps({'url': url, 'console': paired['token'], 'agent': agent['token'], 'name': 'android-test'}))
            fixture.chmod(0o600)
            env['MYCO_ANDROID_FIXTURE'] = str(fixture)
            result = subprocess.run(command, env=env)
        finally:
            server.terminate()
            try:
                server.wait(timeout=10)
            except subprocess.TimeoutExpired:
                server.kill()
                server.wait()
    raise SystemExit(result.returncode)
