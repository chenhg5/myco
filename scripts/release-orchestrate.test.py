#!/usr/bin/env python3
"""Exercise metadata identity, missing proof, immutable leases and dispatch recovery."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('orchestrate', Path(__file__).with_name('release-orchestrate.py'))
o = importlib.util.module_from_spec(spec); spec.loader.exec_module(o)
gspec = importlib.util.spec_from_file_location('gitee', Path(__file__).with_name('release-gitee.py'))
g = importlib.util.module_from_spec(gspec); gspec.loader.exec_module(g)
A='a'*40
B='b'*40


class Fake:
    repo='org/project'
    def __init__(self):
        self.calls=[]; self.refs={}; self.runs=[]
    def call(self,path,method='GET',data=None,missing=False):
        self.calls.append((path,method,data))
        if path=='environments/release':
            return {'deployment_branch_policy':{'protected_branches':False,'custom_branch_policies':True}}
        if path=='actions/variables/RELEASE_ENVIRONMENT_READY':
            return {'value':o.POLICY}
        if path.startswith('git/ref/heads/'):
            sha=self.refs.get(path.removeprefix('git/ref/heads/'))
            return {'object':{'sha':sha}} if sha else None
        if path=='git/refs':
            self.refs[data['ref'].removeprefix('refs/heads/')]=data['sha']; return {}
        return None
    def pages(self,path,key=None):
        if 'deployment-branch-policies' in path:
            return [{'name':name,'type':'branch'} for name in o.BRANCHES]
        return self.runs


def plan():
    return {'repo':'org/project','version':'v1.2.4','source_sha':A,
            'source_branch':'release/v1.2.4-source','metadata_branch':'release/v1.2.4-metadata','brief_notes':'notes'}


class ReleaseTests(unittest.TestCase):
    def test_dry_run_makes_no_api_mutations(self):
        api=Fake()
        with patch.object(o,'metadata',return_value=B):
            result=o.prepare(api,plan(),'2026-01-01T00:00:00Z',True)
        self.assertFalse(result['remote_mutations']); self.assertEqual([],api.calls)
        self.assertEqual('release/v1.2.4-source',result['dispatch']['ref'])

    def test_moved_ref_is_not_overwritten(self):
        api=Fake(); api.refs['release/v1.2.4-source']=B
        with self.assertRaises(o.ReleaseError): o.ensure_ref(api,'release/v1.2.4-source',A)
        self.assertEqual(['GET'],[m for _,m,_ in api.calls])

    def test_uncertain_dispatch_never_redispatches(self):
        api=Fake(); api.refs['release/v1.2.4-dispatch']=B
        with patch.object(o,'metadata',return_value=B),patch.object(o,'git',return_value=''),patch.dict(os.environ,{'RELEASE_ENVIRONMENT_READY':o.POLICY}):
            with self.assertRaisesRegex(o.ReleaseError,'outcome unknown'):
                o.prepare(api,plan(),'2026-01-01T00:00:00Z',False)
        self.assertFalse(any('dispatches' in p for p,_,_ in api.calls))

    def test_prior_run_recovered_only_with_all_identities(self):
        api=Fake(); p={**plan(),'metadata_sha':B}
        valid={'id':123,'event':'workflow_dispatch','head_branch':p['source_branch'], 'head_sha':A,'display_title':'release '+B}
        api.runs=[valid,{**valid,'head_branch':'main'},{**valid,'head_sha':B},{**valid,'display_title':'release '+A}]
        self.assertEqual([valid],o.child_runs(api,p))

    def test_other_global_lease_blocks_before_any_push(self):
        api=Fake(); api.refs['release/publish-lock']='c'*40
        with patch.object(o,'metadata',return_value=B),patch.object(o,'git') as git,patch.dict(os.environ,{'RELEASE_ENVIRONMENT_READY':o.POLICY}):
            with self.assertRaisesRegex(o.ReleaseError,'Another release'):
                o.prepare(api,plan(),'2026-01-01T00:00:00Z',False)
            git.assert_not_called()

    def test_environment_rejects_wildcard_and_tag_deployments(self):
        api=Fake()
        api.pages=lambda *args:[{'name':'*','type':'branch'}]
        with self.assertRaises(o.ReleaseError): o.environment_policy(api)

    def test_ready_marker_error_explains_repository_variable_scope(self):
        with patch.dict(os.environ, {'RELEASE_ENVIRONMENT_READY': ''}):
            with self.assertRaisesRegex(o.ReleaseError, 'Repository Actions variable.*Environment.*version job'):
                o.environment_policy(Fake())

    def test_literal_titles_and_metadata_do_not_change_working_tree(self):
        cwd=os.getcwd()
        with tempfile.TemporaryDirectory() as tmp:
            os.chdir(tmp)
            try:
                for args in [('init','-q'),('config','user.name','Test'),('config','user.email','test@example.invalid')]:o.git(*args)
                Path('VERSION').write_text('v1.2.3\n');Path('CHANGELOG.md').write_text('# Changes\n\n## v1.2.3\nold\n\n**中文**\n旧\n')
                o.git('add','.');o.git('commit','-qm','Base')
                previous=o.git('rev-parse','HEAD')
                Path('product.txt').write_text('product')
                o.git('add','.');o.git('commit','-qm','Literal $(not executed) <tag>')
                sha=o.git('rev-parse','HEAD');p={**plan(),'source_sha':sha,'brief_notes':o.brief(previous,sha,'v1.2.4')}
                before=Path('VERSION').read_text(); first=o.metadata(p,'2026-01-01T00:00:00Z');second=o.metadata(p,'2026-01-01T00:00:00Z')
                self.assertEqual(first,second);self.assertEqual(before,Path('VERSION').read_text())
                self.assertIn('$(not executed)',p['brief_notes']);self.assertIn('&lt;tag&gt;',p['brief_notes'])
                self.assertEqual({'VERSION','CHANGELOG.md'},set(o.git('diff','--name-only',sha,first).splitlines()))
                self.assertEqual('',o.git('status','--porcelain'))
            finally:os.chdir(cwd)

    def test_complete_ci_rejects_skipped_or_cancelled_ui(self):
        names=o.source.REQUIRED|{'app-contract'}|o.source.UI_SHARDED
        jobs=[{'id':i,'name':name,'conclusion':'success'} for i,name in enumerate(sorted(names))]
        self.assertTrue(o.source.green(jobs,full=True))
        ui=next(j for j in jobs if j['name'].startswith('ui ('));ui['conclusion']='skipped'
        self.assertFalse(o.source.green(jobs,full=True))

    def test_finish_preserves_concurrent_main_and_does_not_cleanup_after_conflict(self):
        api=Fake(); calls=[]
        log='# Changelog\n\n## v1.2.4\nbrief\n\n**中文**\n简要\n'
        def git(*args, **kwargs):
            calls.append(args)
            if args[:1]==('rev-parse',):return A
            if args[:1]==('show',):return 'v1.2.3' if args[1].endswith(':VERSION') else (log if args[1].startswith(B) else '# Changelog\n\n## v1.2.3\nold\n')
            return ''
        def call(path,method='GET',data=None,missing=False):
            api.calls.append((path,method,data))
            if path.startswith('releases/tags/'):
                return {'draft':False,'published_at':'2026-01-01','html_url':'https://example.invalid/release'}
            if path.startswith('commits/'):return {'sha':B}
            if path.startswith('git/commits/') and method=='GET':return {'tree':{'sha':'c'*40}}
            if path=='git/trees':return {'sha':'d'*40}
            if path=='git/commits':return {'sha':'e'*40}
            if path=='git/refs/heads/main':raise o.ReleaseError('non-fast-forward')
            raise AssertionError(path)
        api.call=call
        with patch.object(o,'git',side_effect=git):
            with self.assertRaisesRegex(o.ReleaseError,'non-fast-forward'):o.finish(api,'v1.2.4',B)
        self.assertFalse(any(args[0]=='push' for args in calls))
        tree=next(data for path,method,data in api.calls if path=='git/trees')
        self.assertEqual({'VERSION','CHANGELOG.md'},{item['path'] for item in tree['tree']})
        self.assertFalse(next(data for path,method,data in api.calls if path=='git/refs/heads/main')['force'])

    def test_finish_idempotent_when_metadata_already_recorded(self):
        api=Fake(); calls=[]
        log='# Changelog\n\n## v1.2.4\nbrief\n\n**中文**\n简要\n'
        def git(*args,**kwargs):
            calls.append(args)
            if args[0]=='rev-parse':return A
            if args[0]=='show':return 'v1.2.4' if args[1].endswith(':VERSION') else log
            return ''
        def call(path,method='GET',data=None,missing=False):
            api.calls.append((path,method,data))
            if path.startswith('releases/tags/'):return {'draft':False,'published_at':'2026-01-01','html_url':'https://example.invalid/release'}
            if path.startswith('commits/'):return {'sha':B}
            if path.startswith('git/ref/heads/'):return None
            raise AssertionError(path)
        api.call=call
        with patch.object(o,'git',side_effect=git),patch.object(o,'metadata_base',return_value='c'*40):o.finish(api,'v1.2.4',B)
        self.assertTrue(all(method=='GET' for _,method,_ in api.calls))
        self.assertFalse(any(args[0]=='push' for args in calls))

    def test_generic_403_is_not_partial_mirror(self):
        self.assertFalse(g.blocked(403,{'message':'Forbidden'}))
        self.assertFalse(g.blocked(500,{'message':'malicious file detected and rejected'}))
        self.assertTrue(g.blocked(403,{'message':'Malicious file detected and rejected.'}))

    def test_asset_path_and_manifest_coverage(self):
        with self.assertRaises(g.MirrorError):g.safe_assets({'assets':[{'name':'../bad'},{'name':'SHA256SUMS'}]})
        with self.assertRaises(g.MirrorError):g.safe_assets({'assets':[{'name':'myco.dmg'}]})


if __name__=='__main__':unittest.main()
