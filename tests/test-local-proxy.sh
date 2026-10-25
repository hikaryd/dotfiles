#!/usr/bin/env bash
# Exercise the shared module, replacing only external process boundaries.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
python3 - "${DOTS_PROXY_TEST_MODULE:-$root/config/shell/local-proxy.sh}" <<'PY'
import json, os, pathlib, shutil, subprocess, sys, tempfile
module = sys.argv[1]
with tempfile.TemporaryDirectory(prefix='dots-local-proxy-') as work:
    root = pathlib.Path(work)
    stub = root / 'stub'
    stub.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
name=pathlib.Path(sys.argv[0]).name
keys=('http_proxy','https_proxy','all_proxy','no_proxy','node_use_env_proxy')
with open(os.environ['TEST_CALLS'],'a') as f:
    f.write(json.dumps({'name':name,'args':sys.argv[1:],'stdin':sys.stdin.read(),'env':{k:v for k,v in os.environ.items() if k.lower() in keys}})+'\\n')
if name=='dots-tools':
    doctor=sys.argv[1:]==['proxy','beatapi','doctor','--if-connected']
    sys.exit(int(os.environ.get('DOCTOR_STATUS' if doctor else 'READY_STATUS','3' if doctor else '0')))
sys.exit(7)
''')
    stub.chmod(0o755)
    for name in ['dots-tools', 'jg']:
        (root/name).symlink_to(stub)
    log = root/'calls'
    env = {**os.environ, 'PATH':str(root)+os.pathsep+os.environ['PATH'],
           'TEST_CALLS':str(log), 'HTTP_PROXY':'parent-value', 'DOTS_PROXY_PORT':'11810',
           'BASH_ENV':'', 'ENV':'', 'ZDOTDIR':str(root/'zsh-config')}
    shells=['bash'] + (['zsh'] if shutil.which('zsh') else [])
    for shell in shells:
        log.write_text('')
        script='source "$1"; shift; if typeset -f pi >/dev/null; then exit 98; fi; jg "$@"; rc=$?; [ "$HTTP_PROXY" = parent-value ] || exit 99; exit "$rc"'
        cmd=[shell,'-f','-c',script,'--',module,'--daemon','two words','']
        result=subprocess.run(cmd,input='query stdin\n',text=True,capture_output=True,env=env)
        calls=[json.loads(line) for line in log.read_text().splitlines()]
        assert result.returncode==7, (shell,result)
        assert len(calls)==2 and calls[0]['args']==['proxy','local','--ensure'] and calls[0]['stdin']=='', calls
        app=calls[1]
        assert app['name']=='jg' and app['args']==['--daemon','two words',''] and app['stdin']=='query stdin\n', app
        for key in ['HTTP_PROXY','HTTPS_PROXY','ALL_PROXY','http_proxy','https_proxy','all_proxy']:
            assert app['env'][key]=='http://127.0.0.1:11810', (key,app)
        assert app['env']['NODE_USE_ENV_PROXY']=='1'
        log.write_text('')
        result=subprocess.run(cmd,input='untouched',text=True,capture_output=True,env={**env,'READY_STATUS':'1'})
        calls=[json.loads(line) for line in log.read_text().splitlines()]
        assert result.returncode!=0 and len(calls)==1 and '--install-service' in result.stderr, (result,calls)
        for errexit in ['', 'set -e; ']:
            for status in ['0','1','3']:
                log.write_text('')
                result=subprocess.run([shell,'-f','-c',errexit+'source "$1"; jg doctor','--',module],input='',text=True,capture_output=True,env={**env,'DOCTOR_STATUS':status})
                calls=[json.loads(line) for line in log.read_text().splitlines()]
                assert calls[0]['args']==['proxy','beatapi','doctor','--if-connected'] and calls[0]['stdin']=='', calls
                if status=='3':
                    assert result.returncode==7 and len(calls)==3 and calls[1]['args']==['proxy','local','--ensure'] and calls[2]['args']==['doctor'], (result,calls)
                else:
                    assert result.returncode==int(status) and len(calls)==1, (result,calls)
    print('PASS shared jg proxy: no retired agent wrapper, child environment, stdin, fail-closed and bounded doctor')
PY
