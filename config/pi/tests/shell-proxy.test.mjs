import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import test from "node:test";

const settings = JSON.parse(
  readFileSync(new URL("../settings.json", import.meta.url), "utf8"),
);
const proxyKeys = [
  "HTTP_PROXY",
  "HTTPS_PROXY",
  "ALL_PROXY",
  "http_proxy",
  "https_proxy",
  "all_proxy",
  "WS_PROXY",
  "WSS_PROXY",
  "ws_proxy",
  "wss_proxy",
];
const proxy = "http://127.0.0.1:10810";
const environment = {
  ...process.env,
  ...Object.fromEntries(proxyKeys.map((key) => [key, proxy])),
  DOTS_TEST_NODE: process.execPath,
  KUBECONFIG: "/fixture/kubeconfig",
  GITLAB_HOST: "gitlab.fixture.invalid",
  NO_PROXY: "localhost,.fixture.invalid",
  no_proxy: "localhost,.fixture.invalid",
};

// Pi joins shellCommandPrefix and each bash/! command with a newline, then
// invokes a non-interactive shell. Exercise that boundary with a real process.
function shellEnvironment(prefix = "") {
  const result = spawnSync(
    settings.shellPath ?? "/bin/bash",
    ["-c", `${prefix}\n"$DOTS_TEST_NODE" -e 'console.log(JSON.stringify(process.env))'`],
    { env: environment, encoding: "utf8", timeout: 5000 },
  );
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout);
}

test("Pi shell commands do not inherit the agent's HTTP or WebSocket proxy", () => {
  const child = shellEnvironment(settings.shellCommandPrefix);
  for (const key of proxyKeys) {
    assert.equal(child[key], undefined, `${key} leaked into a shell command`);
  }
});

test("shell setup preserves Kubernetes, GitLab and proxy bypass configuration", () => {
  const child = shellEnvironment(settings.shellCommandPrefix);
  for (const key of ["KUBECONFIG", "GITLAB_HOST", "NO_PROXY", "no_proxy"]) {
    assert.equal(child[key], environment[key], `${key} was changed`);
  }
});

test("the parent Pi environment retains its proxy for model and MCP requests", () => {
  shellEnvironment(settings.shellCommandPrefix);
  const unmodified = shellEnvironment();
  for (const key of proxyKeys) {
    assert.equal(environment[key], proxy);
    assert.equal(unmodified[key], proxy);
  }
  assert.equal(settings.httpProxy, proxy);
});
