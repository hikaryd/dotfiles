import fcntl
import base64
import getpass
import hashlib
import http.client
import io
import json
import os
from pathlib import Path
import re
import secrets
import sys
from urllib.parse import unquote
import warnings
import uuid
import zipfile

from guard import validate_proxy
from install_local import check_path, open_private, write_owned

HOME = Path.home()
DIRECTORY = HOME / '.config/claude-guard'
CONFIG = DIRECTORY / 'donut.json'
LOCK = DIRECTORY / 'donut.lock'
LIMIT = 1024 * 1024
PATCH = {'webrtc_mode': 'block', 'randomize_fingerprint_on_launch': False, 'restore_session': False}
PUBLIC = {'sync_mode': 'Disabled', 'proxy_bypass_rules': [], 'start_urls': [], 'dns_server': ''}


def safe_directory():
    check_path(HOME, DIRECTORY, directory=True)
    info = DIRECTORY.lstat()
    if info.st_mode & 0o077:
        raise RuntimeError('Donut config directory must be private')


def load():
    check_path(HOME, CONFIG, private=True)
    if not CONFIG.exists():
        raise RuntimeError('Сначала выполните claude-browser configure')
    try:
        with open_private(CONFIG) as stream:
            data = stream.read(LIMIT + 1)
        value = json.loads(data)
    except FileNotFoundError:
        raise RuntimeError('Сначала выполните claude-browser configure') from None
    except (ValueError, UnicodeError):
        raise RuntimeError('Некорректный Donut config') from None
    if len(data) > LIMIT or not isinstance(value, dict):
        raise RuntimeError('Некорректный Donut config')
    if not isinstance(value.get('token'), str) or not re.fullmatch(r'[\x21-\x7e]{32,512}', value['token']):
        raise RuntimeError('Некорректный Donut token')
    if type(value.get('port')) is not int or not 1 <= value['port'] <= 65535:
        raise RuntimeError('Некорректный Donut port')
    if value.get('stage') == 'created' or 'initial_public' in value:
        validate_initial(value.get('initial_public'))
    return value


def validate_initial(value):
    if (not isinstance(value, dict) or value.keys() != PUBLIC.keys()
            or not isinstance(value['sync_mode'], str) or value['dns_server'] is not None and not isinstance(value['dns_server'], str)
            or any(not isinstance(value[k], list) or any(not isinstance(item, str) for item in value[k]) for k in ('proxy_bypass_rules', 'start_urls'))):
        raise RuntimeError('Некорректный начальный снимок Donut')


def save(state):
    safe_directory()
    check_path(HOME, CONFIG, private=True)
    write_owned(CONFIG, json.dumps(state, indent=2) + '\n')
    with open_private(CONFIG) as stream:
        os.fsync(stream.fileno())
    descriptor = os.open(DIRECTORY, os.O_RDONLY | getattr(os, 'O_DIRECTORY', 0) | getattr(os, 'O_NOFOLLOW', 0))
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


def configure():
    if not sys.stdin.isatty() or not sys.stdout.isatty():
        raise ValueError('configure требует интерактивный терминал')
    safe_directory()
    with open_private(LOCK) as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        state = load() if CONFIG.exists() else {}
        try:
            with warnings.catch_warnings():
                warnings.simplefilter('error', getpass.GetPassWarning)
                token = getpass.getpass('Donut Local API token (пусто: сохранить): ') or state.get('token', '')
        except getpass.GetPassWarning:
            raise ValueError('Не удалось скрыть ввод, настройки не изменены') from None
        port = input('Donut Local API port [10108]: ') or str(state.get('port', 10108))
        if not re.fullmatch(r'[\x21-\x7e]{32,512}', token) or not port.isascii() or not port.isdecimal() or not 1 <= int(port) <= 65535:
            raise ValueError('Некорректный token или port')
        state.update(token=token, port=int(port))
        save(state)
    print('Donut настройки сохранены отдельно.')


def request(state, method, path, payload=None):
    body = None if payload is None else json.dumps(payload).encode()
    timeout = 120 if (method == 'POST' and path == '/v1/profiles') or path.endswith('/fingerprint') else 10
    conn = http.client.HTTPConnection('127.0.0.1', state['port'], timeout=timeout)
    try:
        conn.request(method, path, body, {'Authorization': 'Bearer ' + state['token'], 'Content-Type': 'application/json'})
        response = conn.getresponse()
        data = response.read(LIMIT + 1)
        if response.status != 200 and not (response.status == 201 and method == 'POST' and path in ('/v1/extensions', '/v1/extension-groups')):
            raise RuntimeError('Donut API HTTP {}. Проверьте Local API, прокси и установленный Wayfern в Donut.'.format(response.status))
        if len(data) > LIMIT:
            raise RuntimeError('Donut API response too large')
        result = json.loads(data)
        if method == 'GET' and path == '/v1/browsers/wayfern/versions':
            if not isinstance(result, list) or any(not isinstance(version, str) for version in result):
                raise ValueError()
        elif not isinstance(result, dict):
            raise ValueError()
        return result
    except (OSError, http.client.HTTPException, ValueError, UnicodeError):
        raise RuntimeError('Donut Local API недоступен или вернул некорректный ответ') from None
    finally:
        conn.close()


def identity(value):
    try:
        if not isinstance(value, str) or str(uuid.UUID(value)) != value:
            raise ValueError()
    except (ValueError, AttributeError):
        raise RuntimeError('Некорректный ID Donut, дальнейшие действия остановлены') from None
    return value


def profile(state):
    if any(key not in state for key in ('profile_id', 'proxy_id', 'name')):
        raise RuntimeError('Некорректное состояние владения Donut')
    ident = identity(state['profile_id'])
    value = request(state, 'GET', '/v1/profiles/' + ident).get('profile')
    if not isinstance(value, dict) or value.get('id') != ident or value.get('name') != state['name'] or value.get('browser') != 'wayfern' or value.get('proxy_id') != state['proxy_id']:
        raise RuntimeError('Профиль Donut изменён или не принадлежит сохранённому состоянию')
    if type(value.get('is_running')) is not bool:
        raise RuntimeError('Не удалось проверить состояние профиля Donut')
    return value


def proxy_settings(url):
    parsed = validate_proxy(url)
    try:
        username = unquote(parsed.username, errors='strict') if parsed.username is not None else None
        password = unquote(parsed.password, errors='strict') if parsed.password is not None else None
    except UnicodeError:
        raise ValueError('Некорректная кодировка учётных данных прокси') from None
    return {'proxy_type': 'httpstls' if parsed.scheme == 'https' else 'http', 'host': parsed.hostname, 'port': parsed.port,
            'username': username, 'password': password}


def verify_proxy(state, expected):
    proxy = request(state, 'GET', '/v1/proxies/' + identity(state['proxy_id']))
    settings = proxy.get('proxy_settings')
    if proxy.get('id') != state['proxy_id'] or proxy.get('name') != state['name'] or not isinstance(settings, dict) or any(settings.get(k) != v for k, v in expected.items()):
        raise RuntimeError('Прокси Donut изменён, автоматическое исправление запрещено')


def webrtc(state, current):
    def stopped():
        item = profile(state)
        if item['is_running'] or 'extension_group_id' not in item or item['extension_group_id'] != (state.get('webrtc_group_id') if state.get('webrtc_assigned') else None):
            raise RuntimeError('Profile running or extension group changed')

    def matches(item, expected):
        return all(k in item and item[k] == v for k, v in expected.items())

    name = state['name'] + ' WebRTC'
    source = Path(__file__).resolve().parent
    manifest = json.loads((source / 'webrtc-manifest.json').read_text())
    script = (source / 'webrtc-policy.js').read_bytes()
    manifest['name'] = name
    manifest_bytes = json.dumps(manifest, sort_keys=True).encode()
    digest = hashlib.sha256(manifest_bytes + b'\0' + script).hexdigest()
    if state.get('webrtc_digest', digest) != digest:
        raise RuntimeError('WebRTC extension source changed, manual migration required')
    if current['is_running'] or 'extension_group_id' not in current or current['extension_group_id'] != (state.get('webrtc_group_id') if state.get('webrtc_assigned') else None):
        raise RuntimeError('Profile extension group changed, manual inspection required')
    if 'webrtc_extension_id' not in state:
        archive = io.BytesIO()
        with zipfile.ZipFile(archive, 'w', zipfile.ZIP_DEFLATED) as bundle:
            bundle.writestr('manifest.json', manifest_bytes)
            bundle.writestr('policy.js', script)
        stopped()
        state['pending'] = 'WebRTC extension create'
        save(state)
        result = request(state, 'POST', '/v1/extensions', {'file_name': 'claude-webrtc.zip', 'file_data_base64': base64.b64encode(archive.getvalue()).decode()})
        state['webrtc_extension_id'] = identity(result.get('id'))
        state['webrtc_digest'] = digest
        extension = request(state, 'GET', '/v1/extensions/' + state['webrtc_extension_id'])
        if not matches(extension, {'id': state['webrtc_extension_id'], 'name': name, 'manifest_name': name, 'version': manifest['version'], 'source_kind': 'archive', 'linked_path': None, 'sync_enabled': False}):
            raise RuntimeError('WebRTC extension metadata mismatch')
        state.pop('pending')
        save(state)
    extension_id = identity(state['webrtc_extension_id'])
    if state.get('webrtc_digest') != digest:
        raise RuntimeError('Invalid WebRTC extension state')
    extension = request(state, 'GET', '/v1/extensions/' + extension_id)
    if not matches(extension, {'id': extension_id, 'name': name, 'manifest_name': name, 'version': manifest['version'], 'source_kind': 'archive', 'linked_path': None, 'sync_enabled': False}):
        raise RuntimeError('WebRTC extension changed, manual inspection required')
    if 'webrtc_group_id' not in state:
        stopped()
        state['pending'] = 'WebRTC group create'
        save(state)
        result = request(state, 'POST', '/v1/extension-groups', {'name': name})
        state['webrtc_group_id'] = identity(result.get('id'))
        group = request(state, 'GET', '/v1/extension-groups/' + state['webrtc_group_id'])
        if any(group.get(k) != v for k, v in {'id': state['webrtc_group_id'], 'name': name, 'extension_ids': [], 'sync_enabled': False}.items()):
            raise RuntimeError('WebRTC group metadata mismatch')
        state['webrtc_group_empty'] = True
        state.pop('pending')
        save(state)
    group_id = identity(state['webrtc_group_id'])
    group = request(state, 'GET', '/v1/extension-groups/' + group_id)
    members = [] if state.get('webrtc_group_empty') is True else [extension_id]
    if any(group.get(k) != v for k, v in {'id': group_id, 'name': name, 'extension_ids': members, 'sync_enabled': False}.items()):
        raise RuntimeError('WebRTC group changed, manual inspection required')
    if not members:
        stopped()
        state['pending'] = 'WebRTC group configure'
        save(state)
        request(state, 'PUT', '/v1/extension-groups/' + group_id, {'extension_ids': [extension_id]})
        group = request(state, 'GET', '/v1/extension-groups/' + group_id)
        if any(group.get(k) != v for k, v in {'id': group_id, 'name': name, 'extension_ids': [extension_id], 'sync_enabled': False}.items()):
            raise RuntimeError('WebRTC group update not verified')
        state.pop('webrtc_group_empty')
        state.pop('pending')
        save(state)
    if not state.get('webrtc_assigned'):
        current = profile(state)
        if current['is_running'] or 'extension_group_id' not in current or current['extension_group_id'] is not None:
            raise RuntimeError('Profile running or extension group changed')
        state['pending'] = 'WebRTC profile assign'
        save(state)
        request(state, 'PUT', '/v1/profiles/' + identity(state['profile_id']), {'extension_group_id': group_id})
        current = profile(state)
        if current['is_running'] or current.get('extension_group_id') != group_id:
            raise RuntimeError('WebRTC profile assignment not verified')
        state['webrtc_assigned'] = True
        state.pop('pending')
        save(state)


def prepare(url):
    expected = proxy_settings(url)
    digest = hashlib.sha256(url.encode()).hexdigest()
    safe_directory()
    with open_private(LOCK) as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        state = load()
        if state.get('pending'):
            operation = state['pending'] if state['pending'] in ('proxy create', 'profile create', 'profile configure', 'WebRTC extension create', 'WebRTC group create', 'WebRTC group configure', 'WebRTC profile assign') else 'unknown'
            raise RuntimeError('Неопределённый результат операции {}. Проверьте Donut вручную и состояние {}. Автоматический повтор запрещён'.format(operation, CONFIG))
        if state.get('proxy_digest') and state['proxy_digest'] != digest:
            raise RuntimeError('proxy_url изменился. Настройте новый профиль вручную в Donut, автоматическая смена запрещена')
        if 'name' not in state:
            state.update(name='Claude UK ' + secrets.token_hex(8), proxy_digest=digest)
            save(state)
        if not isinstance(state['name'], str) or not re.fullmatch(r'Claude UK [0-9a-f]{16}', state['name']) or state.get('proxy_digest') != digest:
            raise RuntimeError('Некорректное состояние владения Donut')
        if 'proxy_id' not in state or 'profile_id' not in state:
            try:
                request(state, 'GET', '/v1/browsers/wayfern/versions')
            except RuntimeError:
                raise RuntimeError('Donut Local API недоступен. Включите Local API в Donut, проверьте token через claude-browser configure и повторите prepare') from None
        if 'proxy_id' not in state:
            if 'profile_id' in state:
                raise RuntimeError('Некорректное состояние владения Donut')
            state['pending'] = 'proxy create'
            save(state)
            result = request(state, 'POST', '/v1/proxies', {'name': state['name'], 'proxy_settings': expected})
            state['proxy_id'] = identity(result.get('id'))
            state.pop('pending')
            save(state)
        verify_proxy(state, expected)
        if 'profile_id' not in state:
            state['pending'] = 'profile create'
            save(state)
            result = request(state, 'POST', '/v1/profiles', {'name': state['name'], 'browser': 'wayfern', 'version': 'latest', 'proxy_id': state['proxy_id'], 'dns_server': None, 'wayfern_config': PATCH})
            obj = result.get('profile')
            state['profile_id'] = identity(obj.get('id') if isinstance(obj, dict) else None)
            state['initial_public'] = {k: obj.get(k) for k in PUBLIC if k in obj}
            validate_initial(state['initial_public'])
            state['stage'] = 'created'
            state.pop('pending')
            save(state)
        current = profile(state)
        if current['is_running']:
            raise RuntimeError('Остановите профиль в Donut перед подготовкой')
        if state.get('stage') == 'created':
            if any(current.get(k) != v or k not in current for k, v in state['initial_public'].items()):
                raise RuntimeError('Настройки профиля изменились, исправьте вручную')
            state['pending'] = 'profile configure'
            save(state)
            request(state, 'PUT', '/v1/profiles/' + identity(state['profile_id']), PUBLIC)
            state['stage'] = 'configured'
            state.pop('pending')
            state.pop('initial_public')
            save(state)
        if state.get('stage') != 'configured':
            raise RuntimeError('Некорректный этап подготовки Donut')
        current = profile(state)
        if current['is_running'] or any(current.get(k) != v for k, v in PUBLIC.items() if k != 'dns_server') or current.get('dns_server') not in (None, ''):
            raise RuntimeError('Настройки профиля изменились или профиль запущен, исправьте вручную')
        result = request(state, 'POST', '/v1/profiles/bulk-edit', {'profile_ids': [state['profile_id']], 'patch': PATCH})
        if type(result.get('changed')) is not int or result['changed'] not in (0, 1):
            raise RuntimeError('Некорректное подтверждение настройки Donut')
        version = current.get('version')
        match = re.match(r'^(\d+)(?:\.|$)', version) if isinstance(version, str) else None
        if not match or int(match.group(1)) < 152:
            raise RuntimeError('Требуется установленный Wayfern 152 или новее. Обновите его в Donut')
        webrtc(state, current)
        result = request(state, 'GET', '/v1/profiles/' + identity(state['profile_id']) + '/fingerprint')
        if result.get('fingerprint') is None:
            raise RuntimeError('Профиль настроен, но Donut API не предоставил fingerprint. Автоматическая проверка UK недоступна, готовность не подтверждена. Проверьте профиль в интерфейсе Donut')
        try:
            fp = json.loads(result.get('fingerprint'))
        except (ValueError, TypeError):
            fp = None
        if not isinstance(fp, dict) or fp.get('timezone') != 'Europe/London' or fp.get('language') != 'en-GB' or not isinstance(fp.get('languages'), list) or 'en-GB' not in fp['languages']:
            raise RuntimeError('UK fingerprint не подтверждён. Используйте Match fingerprint to proxy в интерфейсе Donut и повторите prepare')
        print('Проверены UK fingerprint и прокси. Настройки Wayfern применены через API, но отдельно не проверены.')
        print('В Donut нажмите Play для профиля ' + state['name'] + '. Браузер работает вне sandbox CLI.')


def status():
    safe_directory()
    with open_private(LOCK) as lock:
        fcntl.flock(lock, fcntl.LOCK_SH)
        state = load()
        if state.get('pending'):
            print('Подготовка Donut требует ручной проверки после неопределённой операции.')
        elif 'profile_id' not in state:
            print('Профиль Donut ещё не подготовлен.')
        else:
            item = profile(state)
            print('Сохранённый профиль Donut: ' + ('запущен' if item['is_running'] else 'остановлен') + '. Готовность не проверялась.')
