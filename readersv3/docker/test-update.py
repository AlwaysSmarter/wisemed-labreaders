"""Offline updater tests. Never use the real Docker, SSH agent or permission tools."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

TEMPLATE = Path(__file__).with_name('update.sh')
APPS = ['siui-bridge', 'siui-bridge-history', 'wisemed-siui-data']
VERSION = '20261027-120000'
FAKE = r'''#!/usr/bin/env python3
import gzip,json,os,pathlib,sys
n=pathlib.Path(sys.argv[0]).name;a=sys.argv[1:]
b=pathlib.Path(os.environ['TEST_DIR']);scenario=os.environ['SCENARIO'];app=os.environ['TEST_APP']
with (b/'calls').open('a') as f:f.write(json.dumps([n]+a)+'\n')
p=b/'state.json';state=json.loads(p.read_text())
def save():p.write_text(json.dumps(state))
if n=='id':print('0');sys.exit(0)
if n=='uname':print('x86_64');sys.exit(0)
if n in ['flock','chown','find','install','sleep']:sys.exit(0)
if n=='ssh-keygen':print('256 SHA256:fixture comment (ED25519)');sys.exit(0)
if n=='ssh-agent':
 if a==['-s']:print('SSH_AUTH_SOCK=fixture; export SSH_AUTH_SOCK; SSH_AGENT_PID=123; export SSH_AGENT_PID;')
 sys.exit(0)
if n=='ssh-add':
 if a==['-l']:
  if scenario=='agent' and os.environ.get('SSH_AUTH_SOCK')!='fixture':sys.exit(2)
  if scenario=='agent' and not (b/'unlocked').exists():sys.exit(1)
  print('256 SHA256:fixture comment (ED25519)')
 else:(b/'unlocked').touch()
 sys.exit(0)
if n=='git':
 if a[0]=='clone':
  if scenario=='clone-failure':sys.exit(128)
  (b/'ssh-command').write_text(os.environ['GIT_SSH_COMMAND'])
  d=pathlib.Path(a[-1]);d.mkdir()
  files=[app+'-20261026-120000.tar.gz', app+'-20261027-120000.tar.gz', 'other-app-20991231-235959.tar.gz']
  if scenario=='duplicate':files.append('nested/'+app+'-20261027-120000.tar.gz')
  if scenario=='missing-archive':files=['other-app-20991231-235959.tar.gz']
  (b/'files').write_text('\0'.join(files)+'\0')
  for name in files:
   dest=d/name;dest.parent.mkdir(exist_ok=True)
   with gzip.open(dest,'wb') as f:f.write(b'fixture')
 elif 'ls-files' in a:sys.stdout.write((b/'files').read_text())
 sys.exit(0)
if n=='docker':
 if a[0] in ['info','load','logs']:sys.exit(0)
 if a[:2]==['image','inspect']:
  field=a[a.index('-f')+1]
  if 'Architecture' in field:print('arm64' if scenario=='architecture' else 'amd64')
  elif 'Labels' in field:print('wrong' if scenario=='wrong-app' else app)
  else:print('new')
 elif a[0]=='inspect':
  name=a[-1]
  if name not in state:sys.exit(1)
  if '-f' in a:
   field=a[a.index('-f')+1]
   if 'Running' in field:print(str(state[name]['running']).lower())
   elif 'Health' in field:print('unhealthy' if scenario in ['unhealthy','first-failure'] and state[name]['image']=='new' else 'healthy')
   elif 'Image' in field:print('new' if scenario=='same' else state[name]['image'])
 elif a[0]=='run':
  if '-version' in a:print('wrong' if scenario=='version' else '20261027-120000')
  elif 'check-config' in a:sys.exit(1 if scenario=='invalid' else 0)
  elif '-d' in a:state[a[a.index('--name')+1]]={'running':True,'image':'new'};save()
  elif '-it' not in a:sys.exit('unexpected docker run '+str(a))
 elif a[0]=='rename':state[a[2]]=state.pop(a[1]);save()
 elif a[0]=='stop':state[a[-1]]['running']=False;save()
 elif a[0]=='start':state[a[-1]]['running']=True;save()
 elif a[0]=='rm':state.pop(a[-1],None);save()
 else:sys.exit('unexpected docker command '+str(a))
 sys.exit(0)
sys.exit('unexpected tool '+n)
'''


class UpdateTests(unittest.TestCase):
    def run_case(self, scenario='success', app=APPS[0], arguments=()):
        with tempfile.TemporaryDirectory() as td:
            p=Path(td);bin_dir=p/'bin';bin_dir.mkdir()
            for tool in ['docker','git','ssh-agent','ssh-add','ssh-keygen','id','uname','flock','chown','find','install','sleep']:
                f=bin_dir/tool;f.write_text(FAKE);f.chmod(0o755)
            home=p/'home with spaces';(home/'.ssh').mkdir(parents=True)
            if scenario!='missing-key':(home/'.ssh/github_dockers').write_text('fixture')
            data=p/'data';(data/'deployments').mkdir(parents=True)
            if scenario!='missing-config':(data/'deployments/config.yaml').write_text('fixture')
            state={} if scenario in ['first-failure','setup','missing-config'] else {app:{'running':True,'image':'old'}}
            (p/'state.json').write_text(json.dumps(state))
            script=p/f'update-{app}.sh';script.write_text(TEMPLATE.read_text().replace('@APP@',app))
            env={**os.environ,'PATH':str(bin_dir)+':'+os.environ['PATH'], 'HOME':str(home),
                 'TEST_DIR':td,'TEST_APP':app,'SCENARIO':scenario,'SIUI_DATA_DIR':str(data),
                 'SIUI_UPDATE_CACHE':str(p/'cache'),'SIUI_HEALTH_TIMEOUT':'1', 'SSH_AUTH_SOCK':'existing',
                 'GIT_SSH_COMMAND':'ssh -i /wrong/key'}
            r=subprocess.run(['bash',str(script),*arguments],env=env,capture_output=True,text=True,timeout=20)
            calls=[json.loads(line) for line in (p/'calls').read_text().splitlines()] if (p/'calls').exists() else []
            if (p/'ssh-command').exists():
                command=(p/'ssh-command').read_text().replace('ssh ', 'printf "%s\\n" ',1)
                args=subprocess.check_output(['sh','-c',command],env={**env,'SIUI_DEPLOY_SSH_KEY':str(home/'.ssh/github_dockers')},text=True).splitlines()
                self.assertEqual(args,['-F','/dev/null','-o','IdentitiesOnly=yes','-o','BatchMode=yes','-i',str(home/'.ssh/github_dockers')])
            self.assertFalse(list((p/'cache').glob('run.*')))
            return r,json.loads((p/'state.json').read_text()),calls

    def test_all_apps_success(self):
        for app in APPS:
            with self.subTest(app=app):
                r,state,calls=self.run_case(app=app)
                self.assertEqual(r.returncode,0,r.stderr)
                self.assertEqual(state[app],{'running':True,'image':'new'})
                self.assertTrue(any(k.startswith(app+'-backup-') and not v['running'] for k,v in state.items()))
                run=next(c for c in calls if c[:2]==['docker','run'] and '-d' in c)
                self.assertIn('--read-only',run);self.assertIn('host',run)
                load=next(c for c in calls if c[:2]==['docker','load'])
                self.assertTrue(load[-1].endswith(app+'-'+VERSION+'.tar.gz'))
                self.assertFalse(any(c[0]=='ssh-agent' for c in calls))

    def test_unlocks_and_cleans_up_only_owned_agent(self):
        r,state,calls=self.run_case('agent');self.assertEqual(r.returncode,0,r.stderr)
        self.assertIn(['ssh-agent','-s'],calls);self.assertIn(['ssh-agent','-k'],calls)
        self.assertTrue(any(c[0]=='ssh-add' and c[1]!='-l' for c in calls))

    def test_rollback(self):
        r,state,calls=self.run_case('unhealthy');self.assertNotEqual(r.returncode,0)
        self.assertEqual(state[APPS[0]],{'running':True,'image':'old'})
        self.assertIn('Rollback healthy.',r.stderr)

    def test_first_failed_container_removed(self):
        r,state,calls=self.run_case('first-failure');self.assertNotEqual(r.returncode,0)
        self.assertEqual(state,{})

    def test_preflight_failures_preserve_old_container(self):
        for scenario in ['invalid','architecture','wrong-app','version','clone-failure','duplicate','missing-archive','missing-key']:
            with self.subTest(scenario=scenario):
                r,state,calls=self.run_case(scenario);self.assertNotEqual(r.returncode,0)
                self.assertFalse(any(c[:2] in [['docker','rename'],['docker','stop']] for c in calls))
                self.assertEqual(state[APPS[0]],{'running':True,'image':'old'})

    def test_same_version_noop(self):
        r,state,calls=self.run_case('same');self.assertEqual(r.returncode,0,r.stderr)
        self.assertFalse(any(c[:2]==['docker','stop'] for c in calls))

    def test_setup_and_missing_config(self):
        r,state,calls=self.run_case('setup',arguments=['--setup']);self.assertEqual(r.returncode,0,r.stderr)
        self.assertTrue(any('-it' in c and '-reconfigure' in c for c in calls))
        self.assertFalse(any(c[:2]==['docker','run'] and '-d' in c for c in calls))
        r,state,calls=self.run_case('missing-config');self.assertNotEqual(r.returncode,0)
        self.assertIn('--setup',r.stderr)

    def test_setup_refuses_running_container(self):
        r,state,calls=self.run_case(arguments=['--setup']);self.assertNotEqual(r.returncode,0)
        self.assertFalse(any('-it' in c for c in calls))


if __name__=='__main__':unittest.main()
