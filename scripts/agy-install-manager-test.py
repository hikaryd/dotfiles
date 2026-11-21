import importlib.util
import os
from pathlib import Path
import plistlib
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
root = Path(__file__).resolve().parent.parent
spec = importlib.util.spec_from_file_location('updater', root / 'scripts/agy-install-manager-update.py')
u = importlib.util.module_from_spec(spec)
spec.loader.exec_module(u)


class ManagerTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir=root)
        self.addCleanup(self.tmp.cleanup)
        self.home = Path(self.tmp.name)
        self.plist = self.home / 'Library/LaunchAgents' / (u.LABEL + '.plist')
        self.plist.parent.mkdir(parents=True)
        self.plist.write_bytes(plistlib.dumps(u.EXPECTED))
        self.calls = []
        self.loaded = True
        self.outdated = True
        self.gui = False
        self.active = False
        self.upgrade_fails = False
        self.interrupt = False
        self.foreign = False
        self.patches = [patch.dict(os.environ, HOME=str(self.home)), patch.object(u, 'run', self.run_cmd),
                        patch.object(u, 'process_path', lambda _: '/foreign' if self.foreign else u.SERVER),
                        patch.object(u.subprocess, 'run', self.upgrade)]
        for p in self.patches:
            p.start()
            self.addCleanup(p.stop)

    def result(self, code=0, out='', err=''):
        return subprocess.CompletedProcess([], code, out, err)

    def run_cmd(self, *args):
        self.calls.append(args)
        if args[:2] == ('brew', 'outdated'):
            return self.result(out='antigravity-tools\n' if self.outdated else '')
        if args[0] == 'brew':
            return self.result()
        if args[0] == 'pgrep':
            return self.result(0 if self.gui else 1)
        if args[0] == 'ps':
            return self.result(out=str(os.getuid()))
        if args[0] == 'lsof':
            return self.result(0 if self.active else 1)
        if args[:2] == ('launchctl', 'bootout'):
            self.loaded = False
            return self.result()
        if args[:2] == ('launchctl', 'bootstrap'):
            self.loaded = True
            return self.result()
        if args[:2] == ('launchctl', 'print'):
            if not self.loaded:
                return self.result(1)
            return self.result(out=f'path = {self.plist}\nprogram = /bin/sh\n{u.EXPECTED["ProgramArguments"][2]}\npid = 123\n')
        raise AssertionError(args)

    def upgrade(self, args, **kwargs):
        self.calls.append(tuple(args))
        if self.interrupt:
            raise KeyboardInterrupt
        return self.result(1 if self.upgrade_fails else 0)

    def test_idle_upgrade_and_restore(self):
        u.manager_update()
        self.assertTrue(self.loaded)
        self.assertEqual([x[1] for x in self.calls if x[0] == 'launchctl'], ['print', 'bootout', 'print', 'bootstrap'])
        self.assertIn(('brew', 'upgrade', '--cask', u.CASK), self.calls)

    def test_upgrade_failure_restores(self):
        self.upgrade_fails = True
        with self.assertRaises(ValueError):
            u.manager_update()
        self.assertTrue(self.loaded)
        self.assertTrue(any(x[:2] == ('launchctl', 'bootstrap') for x in self.calls))

    def test_interrupted_upgrade_restores(self):
        self.interrupt = True
        with self.assertRaises(KeyboardInterrupt):
            u.manager_update()
        self.assertTrue(self.loaded)
        self.assertTrue(any(x[:2] == ('launchctl', 'bootstrap') for x in self.calls))

    def test_safety_deferrals(self):
        for attr in ('gui', 'active', 'foreign'):
            with self.subTest(attr=attr):
                self.calls.clear()
                setattr(self, attr, True)
                if attr == 'foreign':
                    with self.assertRaises(ValueError):
                        u.manager_update()
                else:
                    u.manager_update()
                self.assertFalse(any(x[:2] in (('brew', 'upgrade'), ('launchctl', 'bootout')) for x in self.calls))
                setattr(self, attr, False)

    def test_not_outdated_never_stops(self):
        self.outdated = False
        u.manager_update()
        self.assertFalse(any(x[0] == 'launchctl' for x in self.calls))

    def test_foreign_plist_never_stops(self):
        self.plist.write_bytes(plistlib.dumps({'Label': u.LABEL}))
        with self.assertRaises(ValueError):
            u.manager_update()
        self.assertFalse(any(x[:2] == ('launchctl', 'bootout') for x in self.calls))


if __name__ == '__main__':
    unittest.main()
