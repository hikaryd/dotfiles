import importlib.machinery
import importlib.util
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

path = Path(__file__).resolve().parents[3] / 'scripts/agy-manager'
loader = importlib.machinery.SourceFileLoader('agy_manager', str(path))
spec = importlib.util.spec_from_loader(loader.name, loader)
agy = importlib.util.module_from_spec(spec)
loader.exec_module(agy)


class ManagerTest(unittest.TestCase):
    def test_config_ownership_permissions_and_symlink(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            config = root / 'gui_config.json'
            config.write_text('{"proxy": {"api_key": "fake"}}')
            config.chmod(0o644)
            self.assertEqual(agy.private_json(root, config.name)['proxy']['api_key'], 'fake')
            config.chmod(0o622)
            with self.assertRaises(ValueError):
                agy.private_json(root, config.name)
            config.chmod(0o600)
            root.chmod(0o755)
            with self.assertRaises(ValueError):
                agy.private_json(root, config.name)
            root.chmod(0o700)
            (root / 'link').symlink_to(config)
            with self.assertRaises(OSError):
                agy.private_json(root, 'link')

    def test_manager_config_and_accounts(self):
        config = {'proxy': {'port': 8045, 'allow_lan_access': False, 'auth_mode': 'strict', 'api_key': 'fake', 'upstream_proxy': {'enabled': True, 'url': 'socks5h://127.0.0.1:1080'}, 'proxy_pool': {'enabled': False}}}
        self.assertEqual(agy.manager_key(config), 'fake')
        for url in ('socks5h://', 'socks5h://invalid:99999', 'socks5h://localhost', 'socks5h://bad host:1080'):
            with self.assertRaises(ValueError):
                agy.manager_key({'proxy': dict(config['proxy'], upstream_proxy={'enabled': True, 'url': url})})
        for name, value in [('port', 8046), ('allow_lan_access', True), ('auth_mode', 'off'), ('api_key', '')]:
            invalid = {'proxy': dict(config['proxy'], **{name: value})}
            with self.assertRaises(ValueError):
                agy.manager_key(invalid)
        self.assertFalse(agy.has_account({'accounts': []}))
        self.assertFalse(agy.has_account({'accounts': [{'id': 'fake', 'disabled': True}]}))
        self.assertTrue(agy.has_account({'accounts': [{'id': 'fake'}]}))
        with self.assertRaises(ValueError):
            agy.has_account({'accounts': None})

    @unittest.skipUnless(sys.platform == 'darwin', 'macOS sandbox required')
    def test_sandbox_allows_only_selected_loopback_port(self):
        with socket.socket() as server:
            server.bind(('127.0.0.1', 0))
            server.listen()
            port = server.getsockname()[1]
            profile = agy.PROFILE.replace('localhost:8045', f'localhost:{port}')
            code = "import socket; s=socket.socket(); s.settimeout(2); s.connect(('127.0.0.1', int(__import__('sys').argv[1]))); s.close(); s=socket.socket(); s.settimeout(2); print(s.connect_ex(('192.0.2.1', 443)))"
            result = subprocess.run([agy.SANDBOX, '-p', profile, sys.executable, '-I', '-c', code, str(port)], capture_output=True, text=True, timeout=10, check=True)
            self.assertEqual(result.stdout.strip(), '1')

    def test_service_requires_config_account_and_free_port(self):
        home = Path('/synthetic-home')
        with patch.object(agy, 'load_state', return_value=None), patch.object(agy, 'listener') as check, patch.object(os, 'execve') as execute:
            agy.service(home)
            check.assert_not_called()
            execute.assert_not_called()
        with patch.object(agy, 'load_state', return_value='fake'), patch.object(agy, 'listener', return_value=agy.GUI), patch.object(os, 'execve') as execute:
            agy.service(home)
            execute.assert_not_called()
        with patch.object(agy, 'load_state', return_value='fake'), patch.object(agy, 'listener', return_value=None), patch.object(os, 'execve') as execute, patch.dict(os.environ, {'HOME': str(home), 'BASH_ENV': '/bad', 'ABV_API_KEY': 'bad'}, clear=True):
            agy.service(home)
            executable, args, env = execute.call_args.args
            self.assertEqual(executable, agy.SERVER)
            self.assertEqual(args[1:], ['--host', '127.0.0.1', '--port', '8045', '--no-web'])
            self.assertNotIn('BASH_ENV', env)
            self.assertNotIn('ABV_API_KEY', env)

    def test_login_never_stops_unowned_listener(self):
        home = Path('/synthetic-home')
        with patch.object(agy, 'listener', return_value=agy.SERVER), patch.object(agy, 'job', return_value=('gui/1', 'gui/1/job', Path('/fake'), False)), patch.object(agy.subprocess, 'run') as run:
            with self.assertRaises(ValueError):
                agy.show_login(home)
            run.assert_not_called()

    def test_empty_index_opens_gui_without_launching_cli(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp)
            (home / 'Library/LaunchAgents').mkdir(parents=True)
            directory = home / '.antigravity_tools'
            directory.mkdir(mode=0o700)
            (directory / 'accounts.json').write_text('{"accounts": []}')
            with patch.object(agy.Path, 'home', return_value=home), patch.object(agy.sys, 'argv', ['agy']), patch.object(agy, 'listener', return_value=None), patch.object(agy, 'job', return_value=('gui/test', 'test', home/'test', False)), patch.object(agy.subprocess, 'run') as run, patch.object(agy, 'launch') as launch:
                agy.main()
                run.assert_called_once_with(['/usr/bin/open', '-a', agy.APP], check=True, capture_output=True, timeout=10)
                launch.assert_not_called()
        with patch.object(agy, 'listener', side_effect=ValueError('foreign')), patch.object(agy.subprocess, 'run') as run:
            with self.assertRaises(ValueError):
                agy.show_login(home)
            run.assert_not_called()

    def test_launch_isolated_and_settings_cannot_be_overridden(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp)
            binary = home / '.local/bin/agy'
            binary.parent.mkdir(parents=True)
            binary.touch(mode=0o700)
            with patch.dict(os.environ, {'HOME': str(home), 'BASH_ENV': '/bad', 'GOOGLE_GEMINI_BASE_URL': 'https://bad', 'GEMINI_API_KEY': 'bad', 'HTTPS_PROXY': 'http://bad'}, clear=True), patch.object(os, 'execve') as execute:
                agy.launch(home, 'fake-key', ['-p', 'space $ value'])
                executable, args, env = execute.call_args.args
                self.assertEqual(executable, '/usr/bin/sandbox-exec')
                self.assertEqual(args[-2:], ['-p', 'space $ value'])
                self.assertIn('{"modelProvider":"gemini"}', args)
                self.assertIn('(deny network*)', args[2])
                self.assertEqual(env['GOOGLE_GEMINI_BASE_URL'], 'http://127.0.0.1:8045')
                self.assertEqual(env['GEMINI_API_KEY'], 'fake-key')
                self.assertEqual(env['HOME'], str(home))
                self.assertEqual(env['LC_ALL'], 'en_GB.UTF-8')
                self.assertNotIn('BASH_ENV', env)
                self.assertNotIn('HTTPS_PROXY', env)
                for flag in ('--settings', '--settings={}'):
                    with self.assertRaises(ValueError):
                        agy.launch(home, 'fake-key', [flag])


if __name__ == '__main__':
    unittest.main()
