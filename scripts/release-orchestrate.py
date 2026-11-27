#!/usr/bin/env python3
"""Plan and dispatch a frozen-source release on GitHub Actions (no AI or local credentials)."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import urllib.error
import urllib.request

POLICY = 'actions-release-v1'
SHA = re.compile(r'[0-9a-f]{40}')
VERSION = re.compile(r'v(\d+)\.(\d+)\.(\d+)')
BRANCHES = {'main', 'release/*-source', 'release/*-metadata'}
HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('release_source', HERE / 'release-source.py')
source = importlib.util.module_from_spec(spec)
spec.loader.exec_module(source)


class ReleaseError(RuntimeError):
    pass


def git(*args, data=None, env=None):
    result = subprocess.run(['git', *args], input=data, text=True, capture_output=True,
                            env=env, timeout=90)
    if result.returncode:
        raise ReleaseError('Git operation failed (details suppressed)')
    return result.stdout.strip()


class GitHub:
    def __init__(self, repo):
        if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repo):
            raise ReleaseError('Invalid repository')
        self.repo = repo

    def call(self, path, method='GET', data=None, missing=False):
        token = os.environ.get('GH_TOKEN', '')
        headers = {'Accept': 'application/vnd.github+json', 'User-Agent': POLICY,
                   'X-GitHub-Api-Version': '2022-11-28'}
        if token:
            headers['Authorization'] = 'Bearer ' + token
        if data is not None:
            headers['Content-Type'] = 'application/json'
        req = urllib.request.Request(f'https://api.github.com/repos/{self.repo}/{path}',
                                     data=None if data is None else json.dumps(data).encode(),
                                     headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=45) as response:
                raw = response.read(8 * 1024 * 1024 + 1)
                if len(raw) > 8 * 1024 * 1024:
                    raise ReleaseError('GitHub response exceeds budget')
                return json.loads(raw) if raw else None
        except urllib.error.HTTPError as error:
            if error.code == 404 and missing:
                return None
            raise ReleaseError(f'GitHub {method} failed: HTTP {error.code}; observe before retrying mutations') from None
        except (OSError, ValueError):
            raise ReleaseError(f'GitHub {method} outcome unknown; observe before retrying mutations') from None

    def pages(self, path, key=None):
        result = []
        for page in range(1, 21):
            data = self.call(f'{path}{"&" if "?" in path else "?"}per_page=100&page={page}')
            batch = data[key] if key else data
            result.extend(batch)
            if len(batch) < 100:
                return result
        raise ReleaseError('Pagination coverage exceeds budget')


def require_sha(value):
    if not SHA.fullmatch(value or ''):
        raise ReleaseError('Expected full commit SHA')
    return value


def version_tuple(value):
    if not VERSION.fullmatch(value or ''):
        raise ReleaseError('Expected vMAJOR.MINOR.PATCH')
    return tuple(map(int, VERSION.fullmatch(value).groups()))


def metadata_base(sha):
    parents = git('rev-list', '--parents', '-n', '1', sha).split()[1:]
    if len(parents) == 1 and set(git('diff', '--name-only', parents[0], sha).splitlines()) == {'VERSION', 'CHANGELOG.md'}:
        return parents[0]
    return sha


def ancestor(a, b):
    return subprocess.run(['git', 'merge-base', '--is-ancestor', a, b], capture_output=True).returncode == 0


def compatible(sha):
    workflow = git('show', f'{sha}:.github/workflows/release.yml')
    # Never dispatch an old workflow that predates Environment isolation.
    return '# actions-release-v1' in workflow


def environment_policy(api):
    env = api.call('environments/release')
    policy = env.get('deployment_branch_policy') or {}
    if policy != {'protected_branches': False, 'custom_branch_policies': True}:
        raise ReleaseError('release Environment must use selected deployment branches')
    rules = api.pages('environments/release/deployment-branch-policies', 'branch_policies')
    allowed = {(r.get('name'), r.get('type')) for r in rules}
    if allowed != {(name, 'branch') for name in BRANCHES}:
        raise ReleaseError('release Environment allows unexpected/missing branches or tags')
    # This marker is repository-scoped: the version job deliberately has no
    # credentialed Environment, and cannot read Environment-only variables.
    if os.environ.get('RELEASE_ENVIRONMENT_READY') != POLICY:
        raise ReleaseError(f'Repository Actions variable RELEASE_ENVIRONMENT_READY={POLICY} is required '
                           '(Settings > Secrets and variables > Actions > Variables). '
                           'Setting it only on the release Environment does not reach the version job.')


def brief(previous, sha, version):
    records = git('log', '--first-parent', '--reverse', '--format=%H%x09%s', f'{previous}..{sha}').splitlines()
    items = []
    for record in records:
        commit, title = record.split('\t', 1)
        files = git('diff-tree', '--no-commit-id', '--name-only', '-r', '--root', commit).splitlines()
        if files and set(files) <= {'VERSION', 'CHANGELOG.md'}:
            continue
        component = ('App' if any(p.startswith('web/src/') for p in files)
                     else 'CLI' if any(p.startswith('internal/cli/') for p in files)
                     else 'Runner' if any(p.startswith(('internal/runner/', 'internal/harness/')) for p in files)
                     else 'Hub' if any(p.startswith(('internal/app/', 'internal/transport/', 'internal/hubserver/')) for p in files)
                     else 'Maintenance')
        # Literal titles are quoted, not translated or interpreted as product claims.
        title = title.replace('`', "'").replace('<', '&lt;').replace('>', '&gt;')
        items.append((component, title[:240], commit[:12]))
    lines = [f'## {version}', '', 'Brief release notes (original commit titles; formal notes may follow).', '']
    for label, title, commit in items[:12]:
        lines.append(f'- [{label}] `{title}` ({commit})')
    if len(items) > 12:
        lines.append(f'- {len(items)-12} additional commits in the comparison below.')
    lines += ['', '**中文**', '', '简要说明（保留提交原标题；正式说明稍后补充）。', '']
    for label, title, commit in items[:12]:
        lines.append(f'- [{label}] `{title}` ({commit})')
    if len(items) > 12:
        lines.append(f'- 另有 {len(items)-12} 条提交，见下方完整对比。')
    lines += ['', f'https://github.com/myco-ai/myco/compare/{previous}...{sha}']
    return '\n'.join(lines) + '\n'


def select(api, version, *, dry_run=False):
    version_tuple(version)
    latest = api.call('releases/latest')
    if latest.get('draft') or latest.get('prerelease'):
        raise ReleaseError('Expected latest stable release')
    if version_tuple(version) <= version_tuple(latest['tag_name']):
        raise ReleaseError('Requested version must be newer than latest stable release')
    previous = metadata_base(git('rev-parse', latest['tag_name'] + '^{commit}'))
    runs = api.pages('actions/workflows/ci.yml/runs?branch=main&event=push', 'workflow_runs')
    seen = set()
    for run in runs:
        sha = require_sha(run['head_sha'])
        if sha in seen or run.get('head_branch') != 'main' or run.get('event') != 'push':
            continue
        seen.add(sha)
        if run.get('status') != 'completed' or run.get('conclusion') != 'success':
            continue
        if not ancestor(sha, 'origin/main') or not ancestor(previous, sha):
            continue
        jobs = api.pages(f'actions/runs/{run["id"]}/jobs?filter=latest', 'jobs')
        if not source.green(jobs, full=True):
            continue
        # This is a deployment compatibility gate, not permission to use main's workflow with old source.
        if not dry_run and not compatible(sha):
            continue
        return {'policy': POLICY, 'repo': api.repo, 'version': version, 'source_sha': sha,
                'previous_source': previous, 'previous_version': latest['tag_name'],
                'compatible_dispatch': compatible(sha), 'ci_run_id': run['id'], 'ci_run_attempt': run['run_attempt'],
                'ci_workflow_blob': git('rev-parse', f'{sha}:.github/workflows/ci.yml'),
                'release_workflow_blob': git('rev-parse', f'{sha}:.github/workflows/release.yml'),
                'source_branch': f'release/{version}-source',
                'metadata_branch': f'release/{version}-metadata',
                'brief_notes': brief(previous, sha, version)}
    raise ReleaseError('No compatible fully green main/push source; do not qualify another SHA as this source')


def metadata(plan, requested_at):
    sha = plan['source_sha']
    old = git('show', f'{sha}:CHANGELOG.md')
    pos = re.search(r'^## ', old, re.M)
    if not pos:
        raise ReleaseError('CHANGELOG has no insertion point')
    notes = plan['brief_notes']
    log = old[:pos.start()] + notes + '\n' + old[pos.start():]
    env = {**os.environ, 'GIT_AUTHOR_NAME': 'github-actions[bot]', 'GIT_AUTHOR_EMAIL': '41898282+github-actions[bot]@users.noreply.github.com',
           'GIT_COMMITTER_NAME': 'github-actions[bot]', 'GIT_COMMITTER_EMAIL': '41898282+github-actions[bot]@users.noreply.github.com',
           'GIT_AUTHOR_DATE': requested_at, 'GIT_COMMITTER_DATE': requested_at}
    with tempfile.TemporaryDirectory() as temp:
        env['GIT_INDEX_FILE'] = str(Path(temp) / 'index')
        git('read-tree', sha, env=env)
        for path, body in [('VERSION', plan['version'] + '\n'), ('CHANGELOG.md', log)]:
            blob = git('hash-object', '-w', '--stdin', data=body, env=env)
            git('update-index', '--add', '--cacheinfo', f'100644,{blob},{path}', env=env)
        tree = git('write-tree', env=env)
    commit = git('commit-tree', tree, '-p', sha, data=f'Release {plan["version"]}\n\n{POLICY}\n', env=env)
    assert set(git('diff', '--name-only', sha, commit).splitlines()) == {'VERSION', 'CHANGELOG.md'}
    return commit


def ensure_ref(api, name, sha):
    current = api.call('git/ref/heads/' + name, missing=True)
    if current:
        if current['object']['sha'] != sha:
            raise ReleaseError('Existing release ref differs; never overwrite it')
        return
    api.call('git/refs', 'POST', {'ref': 'refs/heads/' + name, 'sha': sha})


def child_runs(api, plan):
    return [r for r in api.pages('actions/workflows/release.yml/runs?event=workflow_dispatch&branch=' + plan['source_branch'], 'workflow_runs')
            if r.get('event') == 'workflow_dispatch' and r.get('head_branch') == plan['source_branch']
            and r.get('head_sha') == plan['source_sha'] and r.get('display_title') == 'release ' + plan['metadata_sha']]


def prepare(api, plan, requested_at, dry_run):
    plan = dict(plan)
    plan['dry_run'] = dry_run
    plan['requested_at'] = requested_at
    plan['orchestrator_run_id'] = os.environ.get('GITHUB_RUN_ID', '')
    plan['metadata_sha'] = metadata(plan, requested_at)
    if dry_run:
        plan['remote_mutations'] = False
        plan['dispatch'] = {'workflow': 'release.yml', 'ref': plan['source_branch'], 'release_ref': plan['metadata_sha']}
        return plan
    environment_policy(api)
    # Existing metadata wins on resume (timestamp may differ across a deliberate manual retry).
    existing = api.call('git/ref/heads/' + plan['metadata_branch'], missing=True)
    if existing:
        sha = require_sha(existing['object']['sha'])
        git('fetch', 'origin', sha)
        if (metadata_base(sha) != plan['source_sha'] or git('show', f'{sha}:VERSION') != plan['version']
                or plan['brief_notes'].strip() not in git('show', f'{sha}:CHANGELOG.md')):
            raise ReleaseError('Version is reserved with different source/notes')
        plan['metadata_sha'] = sha
    lock = api.call('git/ref/heads/release/publish-lock', missing=True)
    if lock and lock['object']['sha'] != plan['metadata_sha']:
        raise ReleaseError('Another release owns the lease; inspect it before resuming')
    # Persist metadata objects before remote refs. Bot credentials use checkout's ephemeral helper, never URLs.
    git('push', 'origin', f'{plan["metadata_sha"]}:refs/heads/{plan["metadata_branch"]}')
    ensure_ref(api, 'release/publish-lock', plan['metadata_sha'])
    ensure_ref(api, plan['source_branch'], plan['source_sha'])
    old = child_runs(api, plan)
    if old:
        plan['child_run_id'] = max(old, key=lambda r: r['id'])['id']
        plan['recovered'] = True
        return plan
    intent_ref = f'release/{plan["version"]}-dispatch'
    if api.call('git/ref/heads/' + intent_ref, missing=True):
        raise ReleaseError('Dispatch outcome unknown; observe existing run, never automatically redispatch')
    # Immutable intent is durable before sending workflow_dispatch. A crash cannot silently duplicate publication.
    ensure_ref(api, intent_ref, plan['metadata_sha'])
    plan['dispatch_intent'] = True
    save_plan(plan)
    api.call('actions/workflows/release.yml/dispatches', 'POST', {'ref': plan['source_branch'],
             'inputs': {'release_ref': plan['metadata_sha'], 'dry_run': False, 'sign_only': False,
                        'force_checks': False, 'managed_release': True}})
    plan['dispatched'] = True
    return plan


def save_plan(plan, path='release-plan.json'):
    Path(path).write_text(json.dumps(plan, ensure_ascii=False, indent=2) + '\n')


def finish(api, version, metadata_sha):
    """Only after publication, merge two metadata files into current main using a CAS update."""
    version_tuple(version); require_sha(metadata_sha)
    release = api.call('releases/tags/' + version)
    tag = api.call('commits/' + version)
    if release.get('draft') or not release.get('published_at') or tag['sha'] != metadata_sha:
        raise ReleaseError('Publication/tag identity mismatch')
    git('fetch', 'origin', 'main', metadata_sha)
    main = git('rev-parse', 'origin/main')
    current_version = git('show', f'{main}:VERSION')
    if version_tuple(current_version) > version_tuple(version):
        raise ReleaseError('Main metadata already advanced; do not overwrite newer version')
    source_log = git('show', f'{metadata_sha}:CHANGELOG.md')
    match = re.search(rf'^## {re.escape(version)}\s*\n(.*?)(?=^## |\Z)', source_log, re.M | re.S)
    if not match:
        raise ReleaseError('Missing metadata section')
    section = '## ' + version + '\n' + match[1].rstrip() + '\n'
    main_log = git('show', f'{main}:CHANGELOG.md')
    existing = re.search(rf'^## {re.escape(version)}\s*\n(.*?)(?=^## |\Z)', main_log, re.M | re.S)
    if existing and existing[1].strip() != match[1].strip():
        raise ReleaseError('Main has owner-edited notes; leave pending')
    if not existing:
        pos = re.search(r'^## ', main_log, re.M)
        main_log = main_log[:pos.start()] + section + '\n' + main_log[pos.start():]
    if not existing or current_version != version:
        changes = []
        for path, text in [('VERSION', version + '\n'), ('CHANGELOG.md', main_log)]:
            changes.append({'path': path, 'mode': '100644', 'type': 'blob', 'content': text})
        tree = api.call('git/trees', 'POST', {'base_tree': api.call('git/commits/' + main)['tree']['sha'], 'tree': changes})
        commit = api.call('git/commits', 'POST', {'message': f'Record release {version}', 'tree': tree['sha'], 'parents': [main]})
        # Non-forced API update fails if main moved, preserving concurrent source changes.
        api.call('git/refs/heads/main', 'PATCH', {'sha': commit['sha'], 'force': False})
    # GitHub ref deletion has no expected-SHA parameter: use git's atomic force-with-lease.
    for name, expected in [(f'release/{version}-source', metadata_base(metadata_sha)),
                           (f'release/{version}-dispatch', metadata_sha), ('release/publish-lock', metadata_sha)]:
        ref = api.call('git/ref/heads/' + name, missing=True)
        if ref and ref['object']['sha'] == expected:
            git('push', 'origin', '--force-with-lease=refs/heads/' + name + ':' + expected, ':refs/heads/' + name)
    return {'published': True, 'version': version, 'metadata_synced': True, 'release_url': release['html_url']}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('step', choices=['plan', 'dispatch', 'finish'])
    parser.add_argument('--version', required=True)
    parser.add_argument('--repo', default=os.environ.get('GITHUB_REPOSITORY', 'myco-ai/myco'))
    parser.add_argument('--dry-run', action='store_true')
    parser.add_argument('--metadata-sha')
    parser.add_argument('--plan', type=Path, default=Path('release-plan.json'))
    args = parser.parse_args()
    api = GitHub(args.repo)
    try:
        if args.step == 'finish':
            if args.dry_run:
                result = {'dry_run': True, 'would_sync_metadata': True, 'would_cleanup_source_lease': True}
            else:
                result = finish(api, args.version, args.metadata_sha)
        else:
            run = api.call('actions/runs/' + os.environ['GITHUB_RUN_ID'])
            # Only main manual entry may create release intent; PRs cannot call this privileged path.
            if os.environ.get('GITHUB_EVENT_NAME') != 'workflow_dispatch' or os.environ.get('GITHUB_REF') != 'refs/heads/main':
                raise ReleaseError('Release orchestration requires a manual main dispatch')
            if args.step == 'dispatch':
                plan = json.loads(args.plan.read_text())
                if (plan.get('policy') != POLICY or plan.get('repo') != args.repo or plan.get('version') != args.version
                        or plan.get('orchestrator_run_id') != os.environ['GITHUB_RUN_ID']):
                    raise ReleaseError('Dispatch plan is not bound to this workflow/run')
                sha = require_sha(plan['source_sha'])
                if not compatible(sha) or not ancestor(sha, 'origin/main'):
                    raise ReleaseError('Selected source predates secure dispatch or is not main history')
                proof = source.green_run(args.repo, sha, full=True)
                if not proof or proof['id'] != plan['ci_run_id']:
                    raise ReleaseError('Selected complete CI proof changed; do not release this plan')
                for field, path in [('ci_workflow_blob', '.github/workflows/ci.yml'), ('release_workflow_blob', '.github/workflows/release.yml')]:
                    if git('rev-parse', f'{sha}:{path}') != plan[field]:
                        raise ReleaseError('Frozen workflow fingerprint changed')
                if metadata(plan, plan['requested_at']) != plan['metadata_sha']:
                    raise ReleaseError('Metadata preview changed before dispatch')
                result = prepare(api, plan, plan['requested_at'], args.dry_run)
            else:
                result = prepare(api, select(api, args.version, dry_run=True), run['created_at'], True)
            save_plan(result)
        print(json.dumps({k: v for k, v in result.items() if k != 'brief_notes'}, ensure_ascii=False))
    except (ReleaseError, KeyError, subprocess.SubprocessError) as error:
        print('::error::' + (str(error) if isinstance(error, ReleaseError) else 'Release protocol/command failed; inspect saved intent'))
        raise SystemExit(1)


if __name__ == '__main__':
    main()
