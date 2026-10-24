#!/usr/bin/env bash
# Real shared module; fake process boundaries only. No live proxy/apps.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
python3 - "$root/config/shell/pi-proxy.sh" <<'PY'
import json, os, pathlib, shutil, subprocess, sys, tempfile
module = sys.argv[1]
with tempfile.TemporaryDirectory(prefix='dots-pi-wrapper-') as work:
    root = pathlib.Path(work)
    stub = root / 'stub'
    stub.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
name=pathlib.Path(sys.argv[0]).name
if name=='pi' and sys.argv[1:]==['update','--extensions']: sys.exit(0)
proxy_env={k:v for k,v in os.environ.items() if k.lower() in ('http_proxy','https_proxy','all_proxy','no_proxy','ws_proxy','wss_proxy','node_use_env_proxy')}
with open(os.environ['TEST_CALLS'],'a') as f:
    f.write(json.dumps({'name':name,'args':sys.argv[1:],'stdin':sys.stdin.read(), 'env':proxy_env})+'\\n')
if name=='dots-tools':
    variable='DOCTOR_STATUS' if sys.argv[1:]==['proxy','beatapi','doctor','--if-connected'] else 'READY_STATUS'
    sys.exit(int(os.environ.get(variable,'3' if variable=='DOCTOR_STATUS' else '0')))
sys.exit(7)
''')
    stub.chmod(0o755)
    for name in ['dots-tools', 'pi', 'jg']:
        (root/name).symlink_to(stub)
    log = root/'calls'
    env = {**os.environ, 'PATH':str(root)+os.pathsep+os.environ['PATH'],
           'TEST_CALLS':str(log), 'HTTP_PROXY':'parent-value', 'PI_PROXY_PORT':'11810'}
    proxy = 'http://127.0.0.1:11810'
    for client in ['pi','jg']:
        log.write_text('')
        script = 'source "$1"; shift; '+client+' "$@"; rc=$?; [ "$HTTP_PROXY" = parent-value ] || exit 99; exit "$rc"'
        cmd = ['bash','-c',script,'--',module,'--daemon','two words','']
        result = subprocess.run(cmd,input='prompt stdin\n',text=True,capture_output=True,env=env)
        calls = [json.loads(line) for line in log.read_text().splitlines()]
        assert result.returncode == 7, (client,result)
        assert len(calls)==2 and calls[0]['name']=='dots-tools' and calls[0]['args']==['proxy','pi','--ensure'], calls
        assert calls[0]['stdin']=='', calls
        app=calls[1]
        assert app['name']==client and app['args']==['--daemon','two words',''] and app['stdin']=='prompt stdin\n', app
        for key in ['HTTP_PROXY','HTTPS_PROXY','ALL_PROXY','http_proxy','https_proxy','all_proxy']:
            assert app['env'][key]==proxy, (client,key)
        if client=='pi':
            for key in ['WS_PROXY','WSS_PROXY','ws_proxy','wss_proxy']: assert app['env'][key]==proxy
            assert 'private.invalid' in app['env']['NO_PROXY']
        else: assert app['env']['NODE_USE_ENV_PROXY']=='1'
        log.write_text('')
        failed=subprocess.run(cmd,input='do not consume',text=True,capture_output=True,env={**env,'READY_STATUS':'1'})
        calls = [json.loads(line) for line in log.read_text().splitlines()]
        assert failed.returncode != 0 and len(calls)==1 and calls[0]['name']=='dots-tools', calls
        assert '--install-service' in failed.stderr, failed.stderr
    # A local Jev diagnosis must finish with its actual status, not fall through
    # to official jg's multi-hour quota cooldown. Other providers remain official.
    shells=['bash'] + (['zsh'] if shutil.which('zsh') else [])
    for shell in shells:
        for errexit in ['', 'set -e; ']:
            for status in ['0','1','3']:
                log.write_text('')
                result=subprocess.run([shell,'-f','-c',errexit+'source "$1"; jg doctor','--',module], input='',text=True,capture_output=True,env={**env,'DOCTOR_STATUS':status})
                calls=[json.loads(line) for line in log.read_text().splitlines()]
                assert calls[0]['args']==['proxy','beatapi','doctor','--if-connected'] and calls[0]['stdin']=='', calls
                if status=='3':
                    assert result.returncode==7 and len(calls)==3 and calls[1]['args']==['proxy','pi','--ensure'] and calls[2]['name']=='jg' and calls[2]['args']==['doctor'], (shell,result,calls)
                else:
                    assert result.returncode==int(status) and len(calls)==1, (shell,result,calls)
    print('PASS Pi/jg startup, child env, stdin, fail-closed behavior and bounded local Jev doctor')
PY
