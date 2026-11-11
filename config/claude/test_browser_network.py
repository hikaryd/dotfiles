import os
import pathlib
import subprocess
import time
import unittest
import uuid


IMAGE = os.environ.get("CLAUDE_GUARD_TEST_IMAGE", "moby/buildkit:buildx-stable-1")
FIREWALL = pathlib.Path(__file__).with_name("firewall.sh").resolve()


def docker(*args, check=True, timeout=20):
    return subprocess.run(
        ["docker", *args], text=True, stdout=subprocess.PIPE,
        stderr=subprocess.PIPE, check=check, timeout=timeout,
    )


@unittest.skipUnless(os.environ.get("CLAUDE_GUARD_DOCKER_TESTS") == "1", "set CLAUDE_GUARD_DOCKER_TESTS=1 for real Docker network tests")
class FirewallNetworkTest(unittest.TestCase):
    def test_private_network(self):
        prefix = "jcode-fw-test-" + uuid.uuid4().hex[:16]
        network, proxy, other, guard = (prefix + suffix for suffix in ("-net", "-proxy", "-other", "-guard"))
        containers = []
        network_created = False
        try:
            docker("network", "create", "--internal", network)
            network_created = True
            gateway = docker("network", "inspect", "-f", "{{(index .IPAM.Config 0).Gateway}}", network).stdout.strip()
            for name in (proxy, other):
                docker("run", "--pull", "never", "-d", "--name", name,
                       "--network", network, "--cap-drop", "ALL",
                       "--security-opt", "no-new-privileges", "--entrypoint", "/bin/sh",
                       IMAGE, "-c", "while true; do nc -l -p 8080 -e /bin/cat; done")
                containers.append(name)
            ip = docker("inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", proxy).stdout.strip()
            other_ip = docker("inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", other).stdout.strip()
            docker("run", "--pull", "never", "-d", "--name", guard,
                   "--network", network, "--cap-drop", "ALL", "--cap-add", "NET_ADMIN",
                   "--security-opt", "no-new-privileges", "--read-only", "--memory", "128m",
                   "--pids-limit", "64", "--sysctl", "net.ipv6.conf.all.disable_ipv6=1",
                   "--sysctl", "net.ipv6.conf.default.disable_ipv6=1", "-v", str(FIREWALL) + ":/firewall.sh:ro",
                   "--entrypoint", "/bin/sh", IMAGE, "/firewall.sh", ip, "8080", gateway)
            containers.append(guard)
            for _ in range(30):
                if "FIREWALL_READY" in docker("logs", guard, check=False).stdout:
                    break
                time.sleep(0.1)
            else:
                self.fail("guard never reported readiness")
            workload = prefix + "-workload"
            docker("run", "--pull", "never", "-d", "--name", workload,
                   "--network", "container:" + guard, "--cap-drop", "ALL", "--user", "1000:1000",
                   "--read-only", "--security-opt", "no-new-privileges", "--entrypoint", "/bin/sh",
                   IMAGE, "-c", "exec sleep infinity")
            containers.append(workload)
            docker("exec", "-d", workload, "/bin/sh", "-c", "while true; do nc -l -p 5900 -e /bin/cat; done")
            guard_ip = docker("inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", guard).stdout.strip()
            self.assertNotEqual(docker("exec", other, "nc", "-z", "-w", "2", guard_ip, "5900", check=False).returncode, 0,
                                "peer containers must not reach the VNC return channel")
            self.assertNotEqual(docker("exec", workload, "iptables", "-P", "OUTPUT", "ACCEPT", check=False).returncode, 0,
                                "untrusted workload must not be able to alter firewall")
            def attempt(host, port):
                return docker("exec", workload, "nc", "-z", "-w", "2", host, str(port), check=False).returncode
            docker("exec", "-d", proxy, "/bin/sh", "-c", "while true; do nc -l -p 8081 -e /bin/cat; done")
            time.sleep(0.2)
            self.assertEqual(docker("exec", other, "nc", "-z", "-w", "2", ip, "8081", check=False).returncode, 0,
                             "control must reach other proxy port")
            self.assertEqual(docker("exec", proxy, "nc", "-z", "-w", "2", other_ip, "8080", check=False).returncode, 0,
                             "control must reach other IP")
            self.assertEqual(attempt(ip, 8080), 0, "exact proxy must be reachable")
            self.assertNotEqual(attempt(ip, 8081), 0, "another port must be blocked")
            self.assertNotEqual(attempt(other_ip, 8080), 0, "another IP must be blocked")
            self.assertEqual(docker("exec", proxy, "nslookup", other, "127.0.0.11", check=False).returncode, 0,
                             "control must resolve through Docker DNS")
            self.assertNotEqual(docker("exec", workload, "nslookup", other, "127.0.0.11", check=False).returncode, 0,
                                "guard must not reach Docker DNS")
            docker("exec", "-d", other, "/bin/sh", "-c", "nc -u -l -p 5353 > /tmp/udp-received")
            time.sleep(0.2)
            docker("exec", workload, "/bin/sh", "-c", 'printf forbidden | nc -u -w 1 "$1" 5353', "sh", other_ip, check=False)
            docker("exec", proxy, "/bin/sh", "-c", 'printf control | nc -u -w 1 "$1" 5353', "sh", other_ip, check=False)
            time.sleep(0.2)
            udp_received = docker("exec", other, "cat", "/tmp/udp-received").stdout
            self.assertIn("control", udp_received, "control UDP must reach listener after guard attempt")
            self.assertNotIn("forbidden", udp_received, "guard UDP must not reach listener")
            self.assertEqual(docker("exec", guard, "cat", "/proc/sys/net/ipv6/conf/all/disable_ipv6").stdout.strip(), "1")
            self.assertEqual(docker("exec", proxy, "ping6", "-c", "1", "-W", "1", "::1", check=False).returncode, 0,
                             "control must have usable IPv6 loopback")
            self.assertNotEqual(docker("exec", guard, "ping6", "-c", "1", "-W", "1", "::1", check=False).returncode, 0,
                                "IPv6 must not connect")
            self.assertIn("-P OUTPUT DROP", docker("exec", guard, "ip6tables", "-S").stdout)
            docker("stop", "-t", "0", proxy)
            self.assertNotEqual(attempt(ip, 8080), 0, "dead proxy must not connect")
            self.assertNotEqual(attempt(other_ip, 8080), 0, "proxy death must not enable bypass")
        finally:
            for container in reversed(containers):
                docker("rm", "-f", "-v", container, check=False)
            if network_created:
                docker("network", "rm", network, check=False)


if __name__ == "__main__":
    unittest.main(verbosity=2)
