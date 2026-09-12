"""Host-side guardrail checks; not an Android API or VM runtime test."""

import os
import subprocess
import unittest
from pathlib import Path

SOURCE = Path(__file__).with_name("AvfProbe.java").read_text()
RELAY = Path(__file__).with_name("SshRelay.java").read_text()


class ProbeContract(unittest.TestCase):
    def test_preflight_returns_before_boot(self):
        main = SOURCE.split("public static void main(", 1)[1]
        self.assertLess(
            main.index('args[0].equals("preflight")'),
            main.index("boot(context, config)"),
        )
        self.assertIn(
            "System.exit(0);\n        }\n        boot(context, config);", main
        )
        prefix = SOURCE.split("private static void boot(", 1)[0]
        self.assertNotIn('"VirtualMachineManager"', prefix)
        self.assertNotIn('"create", new Class<?>[]', prefix)

    def test_fixed_authenticated_guest_relay(self):
        self.assertIn('InetAddress.getByName("127.0.0.1")', RELAY)
        self.assertIn("LOCAL_PORT = 27682", RELAY)
        self.assertIn("GUEST_PORT = 22022L", RELAY)
        self.assertIn("new Semaphore(2)", RELAY)
        self.assertIn("TIMEOUT_MS = 10_000", RELAY)
        self.assertIn('getMethod("connectVsock", long.class)', RELAY)
        self.assertNotIn("setReuseAddress", RELAY)
        stop = SOURCE.split("private static boolean stop()", 1)[1]
        self.assertLess(
            stop.index("currentRelay.close()"), stop.index('get(current, "stop")')
        )

    def test_android_runtime_initialization(self):
        context = SOURCE.split("private static Context context()", 1)[1]
        self.assertLess(
            context.index("Looper.prepareMainLooper()"),
            context.index('getMethod("systemMain")'),
        )
        main = SOURCE.split("public static void main(", 1)[1]
        self.assertLess(
            main.index("setDefaultUncaughtExceptionHandler"), main.index("context()")
        )
        self.assertIn("error.printStackTrace(System.err)", main)
        self.assertIn("System.exit(1)", main)

    def test_no_network_by_default(self):
        self.assertIn("boolean network = args.length == 2;", SOURCE)
        self.assertIn('args[1].equals("--network")', SOURCE)
        self.assertIn('set(custom, "useNetwork", boolean.class, network)', SOURCE)

    def test_fixed_budget_and_deadline(self):
        self.assertIn("512L * 1024 * 1024", SOURCE)
        self.assertIn('getField("CPU_TOPOLOGY_ONE_CPU")', SOURCE)
        self.assertIn("STOP_AFTER_MS = 115_000", SOURCE)
        self.assertIn("DEADLINE_MS = 120_000", SOURCE)
        self.assertIn("Runtime.getRuntime().halt(124)", SOURCE)
        self.assertIn("finally {\n            require(stop(),", SOURCE)

    def test_no_secret_share_or_command_api(self):
        for forbidden in (
            "addSharedPath",
            "ProcessBuilder",
            "Runtime.getRuntime().exec",
            '"setDebugLevel", int.class, 1',
            '"getOrCreate"',
            '"delete"',
        ):
            self.assertNotIn(forbidden, SOURCE)
        self.assertIn('getSharedPaths")) == 0', SOURCE)
        self.assertIn('getField("DEBUG_LEVEL_NONE")', SOURCE)
        self.assertIn("require(stat.st_uid == 2000", SOURCE)
        self.assertIn("require((stat.st_mode & 0077) == 0", SOURCE)
        self.assertIn("file.getCanonicalPath().equals(path)", SOURCE)

    @unittest.skipUnless(
        os.environ.get("JAVA_HOME"), "JDK needed for CLI rejection test"
    )
    def test_invalid_cli_rejected_before_android_calls(self):
        java = Path(os.environ["JAVA_HOME"]) / "bin/java"
        sdk = Path(
            os.environ.get("ANDROID_HOME", str(Path.home() / "Library/Android/sdk"))
        )
        classes = Path(
            os.environ.get("AVF_PROBE_CLASSES", "/tmp/pixel-avf-probe-build/classes")
        )
        for args in (
            [],
            ["other"],
            ["boot", "--shell"],
            ["boot", "--network", "extra"],
        ):
            with self.subTest(args=args):
                result = subprocess.run(
                    [
                        str(java),
                        "-cp",
                        f"{classes}:{sdk}/platforms/android-35/android.jar",
                        "AvfProbe",
                        *args,
                    ],
                    text=True,
                    capture_output=True,
                    check=False,
                )
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("IllegalStateException", result.stderr)
                self.assertNotIn("Stub!", result.stderr)
                self.assertNotIn("CONFIG_OK", result.stdout)


if __name__ == "__main__":
    unittest.main()
