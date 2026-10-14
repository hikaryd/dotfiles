# End-to-end rendering check in an isolated PTY/server; no live sessions touched.
# Run: uv run --with pyte python tests/test-herdr-ui.py scripts/herdr
import codecs
import fcntl
import os
import pty
import select
import struct
import subprocess
import sys
import tempfile
import termios
import time
import pyte

binary = os.path.abspath(sys.argv[1])
with tempfile.TemporaryDirectory(prefix="herdr-ui-check-") as tmp:
    env = dict(os.environ, XDG_CONFIG_HOME=tmp, XDG_STATE_HOME=tmp, TERM="xterm-256color")
    for key in list(env):
        if key.startswith("HERDR_") and key != "HERDR_DOTS_NO_PATCH":
            env.pop(key)
    config = os.path.join(tmp, "config.toml")
    with open(config, "w") as file:
        file.write('onboarding = false\n[terminal]\ndefault_shell = "/bin/sh"\nshell_mode = "non_login"\n[ui]\nsidebar_start_collapsed = true\nsidebar_collapsed_mode = "hidden"\nmouse_capture = false\n')
    env["HERDR_CONFIG_PATH"] = config
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 20, 100, 0, 0))
    proc = subprocess.Popen([binary], stdin=slave, stdout=slave, stderr=slave, env=env)
    os.close(slave)
    class Screen(pyte.Screen):
        def write_process_input(self, data):
            os.write(master, data.encode())
        def report_device_status(self, mode, private=False):
            if not private:
                super().report_device_status(mode)
    screen = Screen(100, 20)
    stream = pyte.Stream(screen)
    decoder = codecs.getincrementaldecoder("utf-8")("replace")
    try:
        until = time.monotonic() + 4
        while time.monotonic() < until and proc.poll() is None:
            if select.select([master], [], [], 0.1)[0]:
                stream.feed(decoder.decode(os.read(master, 65536)))
        row = screen.display[0]
        print("tab row:", repr(row))
        assert "1" in row, "client did not draw the test tab"
        assert "" in row and "" in row, "FAIL: tabs are not rounded"
        left, right = row.index(""), row.rindex("")
        assert abs((left + right) / 2 - 49.5) <= 1, f"FAIL: tabs not centered: {left}..{right}"
        print(f"PASS: rounded caps and centered tab ({left}..{right})")
    finally:
        subprocess.run([binary, "server", "stop"], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=5)
        if proc.poll() is None:
            proc.terminate()
        try:
            proc.wait(timeout=1)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()
        os.close(master)
