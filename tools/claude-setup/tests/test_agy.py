import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'src'))
import launcher


class AgyTest(unittest.TestCase):
    def test_guarded_launch(self):
        with tempfile.TemporaryDirectory() as tmp:
            home = Path(tmp).resolve()
            binary = home / '.local/bin/agy'
            binary.parent.mkdir(parents=True)
            binary.touch(mode=0o700)
            config = {'proxy_url': 'http://192.0.2.1:8000', 'direct_https_hosts': ['example.org']}
            args = ['--project', 'space and $dollar']
            with (patch.object(launcher, 'REAL_HOME', home),
                  patch.object(launcher, 'relay', return_value=4321) as relay,
                  patch.dict(os.environ, {'HOME': str(home), 'NO_PROXY': '*'}),
                  patch.object(os, 'execve') as execute):
                launcher.run_agy(args, config)
                relay.assert_called_once_with(config['proxy_url'])
                sandbox, argv, env = execute.call_args.args
                self.assertEqual(sandbox, '/usr/bin/sandbox-exec')
                self.assertEqual(argv[-3:], [str(binary), *args])
                self.assertIn('(deny network*)', argv[2])
                self.assertEqual(env['HTTPS_PROXY'], 'http://127.0.0.1:4321')
                self.assertNotIn('*', env['NO_PROXY'])
                self.assertEqual((env['HOME'], env['TZ'], env['LC_ALL']), (str(home), 'Europe/London', 'en_GB.UTF-8'))
                execute.reset_mock()
                relay.side_effect = RuntimeError('offline')
                with self.assertRaisesRegex(RuntimeError, 'offline'):
                    launcher.run_agy(args, config)
                execute.assert_not_called()
