"""Crypto-service-only private Unix JSON-lines broker; never a global API.

One completion at a time; peer UID + owner-only directory/socket authorization.
Same-UID processes remain trusted (not a tenant isolation boundary).
"""
import argparse
import json
import math
import os
import signal
import socket
import socketserver
import stat
import struct
import subprocess
import sys
import threading
from functools import partial
from pathlib import Path
from jsonschema import Draft202012Validator
from .subscription_auth import (
    DEVELOPMENT_ACCOUNT_ID, DEVELOPMENT_ACCOUNT_POLICY,
    PRODUCTION_ACCOUNT_ID, PRODUCTION_ACCOUNT_POLICY, is_shared_route,
)
from .model_policy import ALLOWED_MODELS,DEFAULT_RESEARCH_MODEL

MODEL=DEFAULT_RESEARCH_MODEL  # compatibility default; runtime configuration is authoritative
PROVIDER='openai-codex'
MAX_REQUEST=160*1024
MAX_OUTPUT=256*1024
MAX_TIMEOUT=120
SAFE_ERRORS={'auth_unavailable','upstream_bad_request','upstream_unauthorized','upstream_rate_limited','upstream_failed','provider_timeout','invalid_request','invalid_output','output_limit','tools_forbidden','worker_failed','unauthorized','busy','request_limit','transport_unavailable'}
ROUTE_POLICIES={
    'development':(DEVELOPMENT_ACCOUNT_POLICY,DEVELOPMENT_ACCOUNT_ID),
    'production':(PRODUCTION_ACCOUNT_POLICY,PRODUCTION_ACCOUNT_ID),
}
CONFIG_KEYS={'socket','allowed_uid','model','provider','tools','paid_fallback','stage','account_policy','expected_account_id'}

class BrokerError(RuntimeError): pass


def validate_request(req):
    if not isinstance(req,dict) or set(req)-{'prompt','timeout','schema'}: raise BrokerError('invalid_request')
    prompt=req.get('prompt')
    timeout=req.get('timeout',90)
    if not isinstance(prompt,str) or not prompt or len(prompt.encode())>131072: raise BrokerError('invalid_request')
    if isinstance(timeout,bool) or not isinstance(timeout,(int,float)) or not math.isfinite(timeout) or not 0<timeout<=MAX_TIMEOUT: raise BrokerError('invalid_request')
    schema=req.get('schema')
    if schema is not None:
        try:
            encoded=json.dumps(schema)
            if len(encoded.encode())>16384: raise ValueError()
            def walk(value,depth=0):
                if depth>16: raise ValueError()
                if isinstance(value,dict):
                    if any(k in value for k in ('$ref','$dynamicRef','$recursiveRef')): raise ValueError()
                    for v in value.values(): walk(v,depth+1)
                elif isinstance(value,list):
                    for v in value: walk(v,depth+1)
            walk(schema)
            Draft202012Validator.check_schema(schema)
        except Exception: raise BrokerError('invalid_request') from None
    return {**req,'timeout':timeout}


def run_worker(
    req,
    *,
    model=MODEL,
    stage='development',
    account_policy=DEVELOPMENT_ACCOUNT_POLICY,
    expected_account_id=DEVELOPMENT_ACCOUNT_ID,
):
    """Run a worker with a fixed route; production wiring always overrides both.

    The development defaults preserve the internal deterministic timeout-test
    helper contract. They select proton explicitly and never consult a primary
    credential as a fallback. Runtime broker configuration remains mandatory.
    """
    # Minimal environment: no API keys, alternate endpoints, inherited proxies,
    # user module search paths, auth-home overrides or CLI refresh mechanisms.
    env={'PATH':'/usr/local/bin:/usr/bin:/bin','PYTHONDONTWRITEBYTECODE':'1','PYTHONIOENCODING':'utf-8'}
    for name in ("DOTS_PROTON_ACCOUNT_ID", "DOTS_PM_ACCOUNT_ID"):
        if name in os.environ:
            env[name] = os.environ[name]
    proc=subprocess.Popen([
        sys.executable,
        str(Path(__file__).with_name('hermes_worker.py')),
        '--model',model,
        '--stage',stage,
        '--account-policy',account_policy,
        '--expected-account-id',expected_account_id,
    ],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True,env=env,start_new_session=True)
    try:
        out,_=proc.communicate(json.dumps(req),timeout=req['timeout']+3)
    except BaseException as exc:
        if proc.poll() is None:
            os.killpg(proc.pid,signal.SIGKILL)
        proc.communicate()
        if isinstance(exc,subprocess.TimeoutExpired): raise BrokerError('provider_timeout') from None
        raise
    try:
        if len(out.encode())>MAX_OUTPUT+8192: raise BrokerError('output_limit')
        result=json.loads(out)
        if proc.returncode or 'error' in result:
            code=result.get('error')
            raise BrokerError(code if code in SAFE_ERRORS else 'worker_failed')
        return result
    except BrokerError: raise
    except Exception: raise BrokerError('worker_failed') from None


class Handler(socketserver.StreamRequestHandler):
    def handle(self):
        self.connection.settimeout(5)
        acquired=False
        try:
            _,uid,_=struct.unpack('3i',self.connection.getsockopt(socket.SOL_SOCKET,socket.SO_PEERCRED,12))
            rejection=None
            if uid!=self.server.allowed_uid:
                rejection='unauthorized'
            else:
                acquired=self.server.slot.acquire(False)
                if not acquired: rejection='busy'
            # Decide admission once, but consume the bounded request frame before
            # replying/closing. Otherwise accept can beat the client's sendall:
            # it sees EPIPE/reset instead of the actual busy/unauthorized reply.
            # Rejected peers never parse JSON, acquire later, or invoke a worker.
            raw=self.rfile.readline(MAX_REQUEST+1)
            if rejection: raise BrokerError(rejection)
            if len(raw)>MAX_REQUEST: raise BrokerError('request_limit')
            if not raw.endswith(b'\n'): raise BrokerError('invalid_request')
            try: req=validate_request(json.loads(raw))
            except BrokerError: raise
            except Exception: raise BrokerError('invalid_request') from None
            result=self.server.complete(req)
            text=result.get('text')
            if not isinstance(text,str) or len(text.encode())>MAX_OUTPUT: raise BrokerError('output_limit')
            if result.get('model')!=self.server.model or result.get('provider')!=PROVIDER or result.get('tool_calls')!=0: raise BrokerError('invalid_output')
            if req.get('schema') is not None:
                try: Draft202012Validator(req['schema']).validate(json.loads(text))
                except Exception: raise BrokerError('invalid_output') from None
            # Return only the narrow allowlisted envelope, never arbitrary worker fields.
            result={k:result[k] for k in ('text','model','provider','tool_calls','usage')}
        except BrokerError as exc:
            result={'error':str(exc) if str(exc) in SAFE_ERRORS else 'worker_failed'}
        except Exception:
            result={'error':'worker_failed'}
        finally:
            if acquired: self.server.slot.release()
        try: self.wfile.write(json.dumps(result,ensure_ascii=False).encode()+b'\n')
        except OSError: pass


class Broker(socketserver.ThreadingUnixStreamServer):
    daemon_threads=True
    request_queue_size=4
    def __init__(self,path,allowed_uid,complete=run_worker,model=MODEL):
        parent=Path(path).parent
        info=parent.stat()
        if parent.is_symlink() or info.st_uid!=os.geteuid() or info.st_mode & 0o077:
            raise BrokerError('private socket parent required')
        if Path(path).exists() or Path(path).is_symlink(): raise BrokerError('socket path already exists; refusing overwrite')
        if allowed_uid!=os.geteuid(): raise BrokerError('private socket requires same runtime uid')
        if model not in ALLOWED_MODELS: raise BrokerError('fixed subscription policy required')
        self.allowed_uid=allowed_uid
        self.model=model
        self.complete=complete
        self.slot=threading.BoundedSemaphore(1)
        old=os.umask(0o177)
        try: super().__init__(path,Handler)
        finally: os.umask(old)
        os.chmod(path,0o600)
        self.socket_identity=os.stat(path).st_ino
    def server_close(self):
        super().server_close()
        try:
            info=os.lstat(self.server_address)
            if stat.S_ISSOCK(info.st_mode) and info.st_ino==self.socket_identity: os.unlink(self.server_address)
        except FileNotFoundError: pass


def request(path,req):
    req=validate_request(req)
    data=json.dumps(req,ensure_ascii=False).encode()+b'\n'
    if len(data)>MAX_REQUEST: raise BrokerError('request_limit')
    try:
        with socket.socket(socket.AF_UNIX,socket.SOCK_STREAM) as sock:
            sock.settimeout(req['timeout']+5)
            sock.connect(str(path)); sock.sendall(data)
            with sock.makefile('rb') as stream: raw=stream.readline(MAX_OUTPUT+8193)
        if len(raw)>MAX_OUTPUT+8192: raise BrokerError('output_limit')
        result=json.loads(raw)
        if 'error' in result:
            code=result['error']; raise BrokerError(code if code in SAFE_ERRORS else 'worker_failed')
        return result
    except BrokerError: raise
    except socket.timeout: raise BrokerError('provider_timeout') from None
    except Exception: raise BrokerError('transport_unavailable') from None


def load_config(path):
    path=Path(path)
    try:
        info=path.lstat()
        if not stat.S_ISREG(info.st_mode) or info.st_uid!=os.geteuid() or stat.S_IMODE(info.st_mode)!=0o600:
            raise SystemExit('private config required')
        config=json.loads(path.read_text())
    except SystemExit:
        raise
    except Exception:
        raise SystemExit('private config required') from None
    if (
        not isinstance(config,dict)
        or set(config)!=CONFIG_KEYS
        or config.get('model') not in ALLOWED_MODELS
        or config.get('provider')!=PROVIDER
        or config.get('tools')!=[]
        or config.get('paid_fallback') is not False
        or (not is_shared_route(config.get('stage'), config.get('account_policy'), config.get('expected_account_id')) and ROUTE_POLICIES.get(config.get('stage'))!=(config.get('account_policy'),config.get('expected_account_id')))
    ):
        raise SystemExit('fixed subscription policy required')
    return config


def main():
    parser=argparse.ArgumentParser(); parser.add_argument('--config',required=True)
    args=parser.parse_args()
    config=load_config(args.config)
    complete=partial(
        run_worker,
        model=config['model'],
        stage=config['stage'],
        account_policy=config['account_policy'],
        expected_account_id=config['expected_account_id'],
    )
    with Broker(config['socket'],config['allowed_uid'],complete=complete,model=config['model']) as server:
        try: server.serve_forever()
        except KeyboardInterrupt: pass

if __name__=='__main__': main()
