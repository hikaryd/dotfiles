import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { homedir } from "node:os";
import { join } from "node:path";
import test from "node:test";

// Exercise the real TypeScript extension with the same jiti/TUI as installed Pi.
const install = join(homedir(), ".pi/agent/install");
const versionFile = join(install, "current-version");
const runtime =
  process.env.PI_TEST_RUNTIME ??
  (existsSync(versionFile)
    ? join(install, "releases", readFileSync(versionFile, "utf8").trim())
    : undefined);
const options = { skip: !runtime && "Pi managed runtime is not installed" };
let extension;
let visibleWidth;
if (runtime) {
  const require = createRequire(join(runtime, "package.json"));
  const { createJiti } = require("jiti");
  const tuiPath = require.resolve("@earendil-works/pi-tui");
  const jiti = createJiti(import.meta.url, {
    moduleCache: false,
    alias: { "@earendil-works/pi-tui": tuiPath },
  });
  extension = await jiti.import(
    new URL("../extensions/ayu-bar/index.ts", import.meta.url).pathname,
  );
  ({ visibleWidth } = await import(tuiPath));
}
const theme = { fg: (_, text) => text, style: (text) => text };
const footer = {
  getGitBranch: () => "main",
  getExtensionStatuses: () => new Map(),
  onBranchChange: () => () => {},
};
const ctx = () => ({
  mode: "tui",
  cwd: "/fixture/dots",
  model: { provider: "openai", id: "gpt-fixture", contextWindow: 400000 },
  getContextUsage: () => ({
    percent: 10,
    tokens: 40000,
    contextWindow: 400000,
  }),
  sessionManager: { getLeafId: () => "fixture", getBranch: () => [] },
});

test(
  "actual footer retains weekly Codex quota, context and local session stats",
  options,
  () => {
    const now = Date.now();
    const state = {
      quota: {
        weekly: {
          usedPercent: 21,
          remainingPercent: 79,
          resetsAt: now + 86400000,
        },
      },
      updatedAt: now,
    };
    const pi = { getThinkingLevel: () => "high" };
    const render = (width) =>
      extension.renderStatusBar(
        ctx(),
        pi,
        theme,
        footer,
        width,
        undefined,
        { input: 100, output: 20, cost: 0 },
        state,
        now,
      );
    const text = render(100).join("\n");
    assert.match(text, /Codex 7d.*79% left/);
    assert.match(text, /↑100 ↓20/);
    assert.match(text, /ctx 10% 40k\/400k/);
    for (const width of [0, 1, 20, 60, 100])
      for (const row of render(width)) assert.ok(visibleWidth(row) <= width);
  },
);

test(
  "factory does not poll; terminal lifecycle uses Pi auth and exposes missing-login guidance",
  options,
  async () => {
    const handlers = new Map();
    const commands = new Map();
    let reads = 0;
    let renders = 0;
    let bar;
    const notifications = [];
    const pi = {
      on: (name, handler) => handlers.set(name, handler),
      registerCommand: (name, command) => commands.set(name, command),
      getThinkingLevel: () => "high",
    };
    extension.default(pi);
    assert.equal(reads, 0);
    const context = {
      ...ctx(),
      modelRegistry: {
        getProviderAuth: async (provider) => {
          assert.equal(provider, "openai-codex");
          reads++;
          return undefined;
        },
      },
      ui: {
        setFooter: (factory) => {
          bar = factory({ requestRender: () => renders++ }, theme, footer);
        },
        notify: (message) => notifications.push(message),
      },
    };
    handlers.get("session_start")({}, { ...context, mode: "rpc" });
    assert.equal(reads, 0);
    assert.equal(bar, undefined);
    try {
      handlers.get("session_start")({}, context);
      await commands.get("quota").handler("", context);
      assert.equal(
        reads,
        1,
        "initial refresh and /quota share the pending read",
      );
      assert.match(bar.render(100).join("\n"), /Codex 7d.*\/login/);
      assert.match(notifications[0], /OpenAI Codex \(legacy\)/);
      assert.ok(renders > 0);
      bar.dispose();
      handlers.get("agent_end")({}, context);
      assert.equal(reads, 1);
    } finally {
      handlers.get("session_shutdown")({}, context);
    }
  },
);
