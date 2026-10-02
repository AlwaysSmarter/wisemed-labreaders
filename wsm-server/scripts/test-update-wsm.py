"""Offline deployment orchestration tests; no Docker daemon or SSH required."""
import gzip
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[2] / 'update-wsm.sh'
FAKE = r'''#!/usr/bin/env python3
import sys,os,json,pathlib,gzip,shutil
name=pathlib.Path(sys.argv[0]).name;a=sys.argv[1:];base=pathlib.Path(os.environ['TEST_DIR']);scenario=os.environ['SCENARIO']
with (base/'calls').open('a') as f:f.write(json.dumps([name]+a)+'\n')
p=base/'state.json';state=json.loads(p.read_text()) if p.exists() else {'wsm-server':{'running':True,'image':'old'}}
def save():p.write_text(json.dumps(state))
if name=='uname':print('x86_64');sys.exit(0)
if name=='flock':sys.exit(0)
if name=='git':
 if a[0]=='clone':
  d=pathlib.Path(a[-1]);d.mkdir();
  with gzip.open(d/'wisemed-wsm-20260924-160000.tar.gz','wb') as f:f.write(b'fixture')
 elif 'ls-files' in a:sys.stdout.write('wisemed-wsm-20260924-160000.tar.gz\0')
 sys.exit(0)
if name=='curl':
 ok=state.get('wsm-server',{}).get('running',False)
 if scenario=='unhealthy' and state.get('wsm-server',{}).get('image')=='new':ok=False
 if ok: print('{"status":"ok","service":"wsm-server"}')
 sys.exit(0 if ok else 22)
if name=='docker':
 if a[0] in ['info','load','logs']:sys.exit(0)
 if a[:2]==['image','inspect']:
  if 'Architecture' in a[3]: print('arm64' if scenario=='architecture' else 'amd64')
  else:print('new')
 elif a[0]=='inspect':
  n=a[-1]
  if n not in state:sys.exit(1)
  if '-f' in a:
   field=a[a.index('-f')+1]
   if 'Running' in field:print(str(state[n]['running']).lower())
   elif 'Image' in field:print('new' if scenario=='same' else state[n]['image'])
 elif a[0]=='run':
  if '--version' in a:print('20260924-160000')
  elif '-check-config' in a:sys.exit(1 if scenario=='invalid' else 0)
  elif '-d' in a:state[a[a.index('--name')+1]]={'running':True,'image':'new'};save()
 elif a[0]=='rename':state[a[2]]=state.pop(a[1]);save()
 elif a[0]=='stop':state[a[-1]]['running']=False;save()
 elif a[0]=='start':state[a[-1]]['running']=True;save()
 elif a[0]=='rm':state.pop(a[-1],None);save()
 else:sys.exit('unexpected docker command '+str(a))
'''

class UpdateTests(unittest.TestCase):
    def run_case(self, scenario):
        with tempfile.TemporaryDirectory() as td:
            p=Path(td); bindir=p/'bin';bindir.mkdir()
            for name in ['docker','git','curl','flock','uname']:
                f=bindir/name;f.write_text(FAKE);f.chmod(0o755)
            for name in ['deployments','state']:(p/'data'/name).mkdir(parents=True)
            env={**os.environ,'PATH':str(bindir)+':'+os.environ['PATH'],'TEST_DIR':td,
                 'SCENARIO':scenario,'WSM_DATA_DIR':str(p/'data'),
                 'WSM_UPDATE_CACHE':str(p/'cache'),'WSM_HEALTH_TIMEOUT':'1'}
            result=subprocess.run(['bash',str(SCRIPT)],env=env,capture_output=True,text=True,timeout=20)
            state=json.loads((p/'state.json').read_text()) if (p/'state.json').exists() else {}
            calls=[json.loads(s) for s in (p/'calls').read_text().splitlines()]
            return result,state,calls
    def test_success(self):
        r,s,c=self.run_case('success');self.assertEqual(r.returncode,0,r.stderr)
        self.assertEqual(s['wsm-server'],{'running':True,'image':'new'})
        self.assertTrue(any(k.startswith('wsm-server-backup-') and not v['running'] for k,v in s.items()))
    def test_rollback(self):
        r,s,c=self.run_case('unhealthy');self.assertNotEqual(r.returncode,0)
        self.assertEqual(s['wsm-server'],{'running':True,'image':'old'})
        self.assertIn('Rollback healthy',r.stderr)
    def test_preflight_failure_does_not_stop_old(self):
        for scenario in ['invalid','architecture']:
            r,s,c=self.run_case(scenario);self.assertNotEqual(r.returncode,0)
            self.assertFalse(any(x[:2] in [['docker','stop'],['docker','rename']] for x in c))
    def test_same_image_is_noop(self):
        r,s,c=self.run_case('same');self.assertEqual(r.returncode,0,r.stderr)
        self.assertFalse(any(x[:2]==['docker','stop'] for x in c))

if __name__=='__main__':unittest.main()
