import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

ROOT = pathlib.Path(__file__).parent


def module(name):
    spec = importlib.util.spec_from_file_location(name, ROOT / (name + ".py"))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


class OwnerSSHTests(unittest.TestCase):
    def test_inventory_is_required_and_rejects_wildcards(self):
        supervisor = module("supervisor")
        valid = {"lan_ip": "192.0.2.116", "vps_host": "203.0.113.20"}
        self.assertEqual(supervisor.validate_inventory(valid), valid)
        for value in (
            {},
            None,
            {**valid, "extra": "unexpected"},
            {**valid, "lan_ip": "0.0.0.0"},
            {**valid, "lan_ip": "127.0.0.1"},
            {**valid, "lan_ip": "255.255.255.255"},
            {**valid, "vps_host": "-oProxyCommand=bad"},
            {**valid, "vps_host": "0.0.0.0"},
        ):
            with self.assertRaises(ValueError):
                supervisor.validate_inventory(value)
        with (
            tempfile.TemporaryDirectory() as tmp,
            patch.object(supervisor, "HOME", pathlib.Path(tmp)),
            self.assertRaises(FileNotFoundError),
        ):
            supervisor.commands()

    def test_vps_grant_requires_effective_restrictions(self):
        with patch.dict(sys.modules, {"install": module("install")}):
            vps = module("install_vps")
        common = {
            "authenticationmethods": "publickey",
            "passwordauthentication": "no",
            "kbdinteractiveauthentication": "no",
            "allowstreamlocalforwarding": "no",
            "gatewayports": "no",
            "permittty": "no",
            "x11forwarding": "no",
            "allowagentforwarding": "no",
            "permituserrc": "no",
            "maxsessions": "0",
        }
        for user, extra in (
            (
                "pixel-owner-link",
                {
                    "allowtcpforwarding": "remote",
                    "permitlisten": "127.0.0.1:19223",
                    "permitopen": "none",
                },
            ),
            (
                "pixel-owner-jump",
                {
                    "allowtcpforwarding": "local",
                    "permitlisten": "none",
                    "permitopen": "127.0.0.1:19223",
                },
            ),
        ):
            values = common | extra
            output = "\n".join(f"{key} {value}" for key, value in values.items())
            with patch.object(vps.subprocess, "check_output", return_value=output):
                vps.verify_effective(user, "192.0.2.1")
            for key in values:
                unsafe = values | {key: "any"}
                output = "\n".join(f"{name} {value}" for name, value in unsafe.items())
                with (
                    patch.object(vps.subprocess, "check_output", return_value=output),
                    self.assertRaises(RuntimeError),
                ):
                    vps.verify_effective(user, "192.0.2.1")

    def test_supervisor_real_child_failure_recovery_and_shutdown(self):
        with tempfile.TemporaryDirectory() as directory:
            home = pathlib.Path(directory)
            child = home / "child.py"
            child.write_text(
                "import pathlib,sys,time\n"
                "p=pathlib.Path(__file__).with_name('count')\n"
                "n=int(p.read_text())+1 if p.exists() else 1\n"
                "p.write_text(str(n))\n"
                "print('bounded diagnostic',file=sys.stderr,flush=True)\n"
                "if n==1: sys.exit(17)\n"
                "time.sleep(60)\n"
            )
            script = (
                "import importlib.util,pathlib,time\n"
                f"spec=importlib.util.spec_from_file_location('subject',{str(ROOT / 'supervisor.py')!r})\n"
                "s=importlib.util.module_from_spec(spec);spec.loader.exec_module(s)\n"
                f"s.HOME=pathlib.Path({directory!r})\n"
                f"s.commands=lambda:{{'sshd':[{sys.executable!r},{str(child)!r}],'tunnel':[{sys.executable!r},'-c','import time;time.sleep(60)']}}\n"
                "s.lan_available=lambda:(s.HOME/'wifi').exists()\n"
                "sleep=time.sleep;mono=time.monotonic\n"
                "s.time.sleep=lambda seconds:sleep(seconds*.01)\n"
                "s.time.monotonic=lambda:mono()*100\n"
                "s.main()\n"
            )
            process = subprocess.Popen([sys.executable, "-c", script])
            child_pid = None
            tunnel_pid = None
            try:
                deadline = time.monotonic() + 5
                while (
                    not (home / "status.json").exists() and time.monotonic() < deadline
                ):
                    time.sleep(0.02)
                state = json.loads((home / "status.json").read_text())
                self.assertIsNone(state["services"]["sshd"]["pid"])
                (home / "wifi").touch()
                while time.monotonic() < deadline:
                    state = json.loads((home / "status.json").read_text())["services"][
                        "sshd"
                    ]
                    if state["starts"] >= 2 and state["running"]:
                        break
                    time.sleep(0.02)
                self.assertEqual(state["last_exit"], 17)
                self.assertGreaterEqual(state["starts"], 2)
                self.assertTrue(state["running"])
                child_pid = state["pid"]
                state = json.loads((home / "status.json").read_text())
                self.assertEqual(state["services"]["tunnel"]["starts"], 0)
                (home / "tunnel-enabled").touch()
                while time.monotonic() < deadline:
                    state = json.loads((home / "status.json").read_text())
                    tunnel_pid = state["services"]["tunnel"]["pid"]
                    if tunnel_pid:
                        break
                    time.sleep(0.02)
                self.assertIsNotNone(tunnel_pid)
                self.assertIn("bounded diagnostic", (home / "sshd.log").read_text())
                self.assertEqual((home / "status.json").stat().st_mode & 0o777, 0o600)
            finally:
                process.terminate()
                process.wait(timeout=5)
            for pid in (child_pid, tunnel_pid):
                if pid:
                    with self.assertRaises(ProcessLookupError):
                        os.kill(pid, 0)

    def test_wifi_unavailable_then_available(self):
        supervisor = module("supervisor")
        with (
            patch.object(
                supervisor,
                "inventory",
                return_value={"lan_ip": "192.0.2.116", "vps_host": "203.0.113.20"},
            ),
            patch.object(supervisor.socket, "socket") as socket_factory,
        ):
            bind = socket_factory.return_value.__enter__.return_value.bind
            bind.side_effect = [OSError("no address"), None]
            self.assertFalse(supervisor.lan_available())
            self.assertTrue(supervisor.lan_available())

    def test_last_failure_survives_restart_counter(self):
        supervisor = module("supervisor")
        state = {"starts": 1}
        supervisor.note_exit(state, 255)
        state["starts"] += 1
        self.assertEqual(state["last_exit"], 255)
        self.assertIn("last_exit_utc", state)

    def test_no_wildcard_or_password(self):
        text = module("install").config("/private", "u0_a123", "192.0.2.116")
        self.assertEqual(text.count("ListenAddress "), 2)
        self.assertIn("ListenAddress 127.0.0.1\n", text)
        self.assertIn("ListenAddress 192.0.2.116\n", text)
        for line in (
            "PasswordAuthentication no",
            "KbdInteractiveAuthentication no",
            "DisableForwarding yes",
            "PermitUserRC no",
            "StrictModes yes",
        ):
            self.assertIn(line + "\n", text)

    def test_identity_fails_closed(self):
        install = module("install")
        for user, lan in (("root", "192.0.2.116"), ("u0_a123", "0.0.0.0")):
            with self.assertRaises(ValueError):
                install.config("/private", user, lan)

    def test_key_format(self):
        install = module("install")
        self.assertEqual(
            install.public_key("ssh-ed25519 AAAA comment"), "ssh-ed25519 AAAA"
        )
        for value in ("", "ssh-rsa AAAA", 'ssh-ed25519 AAAA"'):
            with self.assertRaises(ValueError):
                install.public_key(value)

    def test_tunnel_is_separate_and_pinned(self):
        supervisor = module("supervisor")
        with patch.object(
            supervisor,
            "inventory",
            return_value={"lan_ip": "192.0.2.116", "vps_host": "203.0.113.20"},
        ):
            cmd = supervisor.commands()["tunnel"]
        for expected in (
            "127.0.0.1:19223:127.0.0.1:8022",
            "StrictHostKeyChecking=yes",
            "IdentityAgent=none",
            "ForwardAgent=no",
            "ExitOnForwardFailure=yes",
        ):
            self.assertIn(expected, cmd)
        self.assertEqual(cmd[-1], "pixel-owner-link@203.0.113.20")
        self.assertNotIn("hermes-pixel@203.0.113.20", cmd)


if __name__ == "__main__":
    unittest.main()
