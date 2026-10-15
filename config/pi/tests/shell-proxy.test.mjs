import assert from "node:assert/strict";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
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
    [
      "-c",
      `${prefix}\n"$DOTS_TEST_NODE" -e 'console.log(JSON.stringify(process.env))'`,
    ],
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

test("clearing proxy in child shells does not mutate their parent environment", () => {
  shellEnvironment(settings.shellCommandPrefix);
  const unmodified = shellEnvironment();
  for (const key of proxyKeys) {
    assert.equal(environment[key], proxy);
    assert.equal(unmodified[key], proxy);
  }
});

test("Pi settings route model and MCP requests through the managed proxy", () => {
  assert.equal(settings.httpProxy, proxy);
});

for (const shell of ["/bin/bash", "/bin/zsh"]) {
  test(`${shell}: Pi ensures a non-RU proxy and preserves args, stdin and parent env`, (t) => {
    const fixture = mkdtempSync(join(tmpdir(), "dots-pi-proxy-"));
    t.after(() => rmSync(fixture, { recursive: true, force: true }));
    const readback = `const keys=${JSON.stringify([...proxyKeys, "NO_PROXY", "no_proxy", "KUBECONFIG", "GITLAB_HOST"])}; const env=Object.fromEntries(keys.filter(key=>process.env[key]!==undefined).map(key=>[key,process.env[key]])); console.log(JSON.stringify({args:process.argv.slice(1), stdin:require("node:fs").readFileSync(0,"utf8"), env})); process.exit(7);`;
    writeFileSync(
      join(fixture, "pi"),
      `#!/bin/sh\nexec "$DOTS_TEST_NODE" -e '${readback}' "$@"\n`,
      { mode: 0o700 },
    );
    writeFileSync(
      join(fixture, "dots-tools"),
      '#!/bin/sh\n[ "$*" = "proxy pi --ensure" ] || exit 98\n[ -z "$(read ignored; printf "%s" "$ignored")" ] || exit 97\nexit "${DOTS_TEST_ENSURE_STATUS:-0}"\n',
      { mode: 0o700 },
    );
    for (const binary of ["lsof", "xray"]) {
      writeFileSync(
        join(fixture, binary),
        '#!/bin/sh\nprintf "unexpected proxy dependency\\n" >&2\nexit 99\n',
        { mode: 0o700 },
      );
    }
    const source = fileURLToPath(
      new URL("../../shell/pi-proxy.sh", import.meta.url),
    );
    const args = ["exec", "argument with spaces", "$(not-a-command)"];
    const result = spawnSync(
      shell,
      [
        "-c",
        `source "$DOTS_TEST_SOURCE"\npi "$@"\nstatus_code=$?\n"$DOTS_TEST_NODE" -e 'console.log(JSON.stringify(Object.fromEntries(${JSON.stringify(proxyKeys)}.map(key=>[key,process.env[key]]))))'\nexit "$status_code"`,
        "fixture",
        ...args,
      ],
      {
        env: {
          ...environment,
          HOME: fixture,
          PATH: `${fixture}:${process.env.PATH}`,
          DOTS_TEST_SOURCE: source,
        },
        input: "fixture stdin\n",
        encoding: "utf8",
        timeout: 5000,
      },
    );
    assert.ifError(result.error);
    assert.equal(result.status, 7, result.stderr);
    const [child, parent] = result.stdout
      .trim()
      .split("\n")
      .map((line) => JSON.parse(line));
    assert.deepEqual(child.args, args);
    assert.equal(child.stdin, "fixture stdin\n");
    for (const key of proxyKeys) {
      assert.equal(child.env[key], proxy, `${key} missing in Pi`);
      assert.equal(parent[key], proxy, `${key} changed in the parent shell`);
    }
    for (const key of ["KUBECONFIG", "GITLAB_HOST"])
      assert.equal(child.env[key], environment[key]);
    assert.match(child.env.NO_PROXY, /nebula-staging\.x5\.ru/);
    assert.equal(child.env.no_proxy, child.env.NO_PROXY);

    const failed = spawnSync(
      shell,
      ["-c", 'source "$DOTS_TEST_SOURCE"; pi -p test'],
      {
        env: {
          ...environment,
          HOME: fixture,
          PATH: `${fixture}:${process.env.PATH}`,
          DOTS_TEST_SOURCE: source,
          DOTS_TEST_ENSURE_STATUS: "1",
        },
        encoding: "utf8",
        timeout: 5000,
      },
    );
    assert.ifError(failed.error);
    assert.equal(failed.status, 1);
    assert.equal(
      failed.stdout,
      "",
      "Pi must not start without a verified non-RU proxy",
    );
  });
}
