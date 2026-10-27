"""Run built images with synthetic configs and isolated networking; no real DB/CNAS."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import uuid


def docker(*args, **kwargs):
    return subprocess.run(['docker',*args],text=True,capture_output=True,**kwargs)


for image in sys.argv[1:]:
    app,version=image.rsplit(':',1)
    protocol={'siui-bridge':'siui','siui-bridge-history':'siui-history','wisemed-siui-data':'siui-data'}[app]
    metadata=json.loads(docker('image','inspect',image,check=True).stdout)[0]
    assert metadata['Architecture']=='amd64',metadata['Architecture']
    assert metadata['Config']['User']=='65532:65532'
    assert metadata['Config']['Labels']['eu.wisemed.app']==app
    result=docker('run','--rm','--network','none','--entrypoint','/opt/wisemed/bin/app',image,'-version',check=True)
    assert result.stdout.strip()==version,result.stdout
    name='siui-docker-smoke-'+uuid.uuid4().hex[:10]
    with tempfile.TemporaryDirectory() as td:
        root=Path(td)
        for directory in ['deployments','data']:
            (root/directory).mkdir();(root/directory).chmod(0o777)
        config={
            'reader':{'id':app,'label':app,'analyzer_name':app,'analyzer_code':app,'db_name':app+'.db'},
            'analyzer':{'comm_type':'utility','protocol':protocol},
            'local_http':{'enabled':True,'address':'127.0.0.1:19199','tls':True},
            'wisemed_ws':{'enabled':False,'url':'wss://127.0.0.1:1/ws'},
            'modules':{
                'app-updates':{'enabled':False,'app_id':app,'channel':'stable','base_url':'http://127.0.0.1:1','auto_download':True,'download_dir':'../data/updates'},
                'wisemed-ws':{'enabled':False,'url':'wss://127.0.0.1:1/ws'},
                'wisemed-api':{'cfg_wisemed_protocol':'http','cfg_wisemed_ip':'127.0.0.1','cfg_wisemed_port':'1',
                    'cfg_wisemed_path':'/test','cfg_wisemed_key':'synthetic-no-network-key',
                    'unitate_medicala_id':'1','tip_de_echipament_id':'1','cod_echipament':'synthetic',
                    'numar_serial_echipament':'synthetic','echipament_id':1,'producator_echipament':'test',
                    'nume_pe_raport_final':'test','nr_rackuri':0,'pozitii_pe_rack':0},
            },
        }
        if app=='wisemed-siui-data':
            config['modules']['siui-data']={'postgres_host':'127.0.0.1','postgres_port':1,'postgres_database':'synthetic','postgres_user':'synthetic'}
        path=root/'deployments/config.yaml';path.write_text(json.dumps(config));path.chmod(0o666)
        try:
            result=docker('run','-d','--name',name,'--network','none','--read-only','--cap-drop','ALL',
                '--security-opt','no-new-privileges:true','--tmpfs','/tmp:rw,nosuid,nodev,mode=1777',
                '--mount',f'type=bind,src={root}/deployments,dst=/data/deployments',
                '--mount',f'type=bind,src={root}/data,dst=/data/data',image,check=True)
            deadline=time.monotonic()+45
            while time.monotonic()<deadline:
                check=docker('exec',name,'/opt/wisemed/bin/docker-runtime','health','/data/deployments/config.yaml')
                if check.returncode==0:break
                time.sleep(1)
            else:
                raise RuntimeError(check.stderr+'\n'+docker('logs',name).stdout+docker('logs',name).stderr)
            template=(root/'deployments/config.install.yaml').read_text()
            expected={'siui-bridge':'database: ../data/siui-jobs.db',
                      'siui-bridge-history':'base_path: ../data/history',
                      'wisemed-siui-data':'upload_dir: ../data/catalogue-uploads'}[app]
            assert expected in template,expected
            assert (root/'deployments/tls/wisemed-local-root-ca.pem').is_file()
            assert (root/'deployments/help').is_dir()
            print(f'{image}: UID, version, persistent config/data, Help and verified TLS health passed',flush=True)
        finally:
            docker('rm','-f',name)
