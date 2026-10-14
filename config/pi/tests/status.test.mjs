import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { formatTokens, sessionUsage } from "../extensions/ayu-bar/status.mjs";

test("compact context counts include 400k", () => {
  assert.equal(formatTokens(0), "0");
  assert.equal(formatTokens(12000), "12k");
  assert.equal(formatTokens(400000), "400k");
  assert.equal(formatTokens(1000000), "1M");
});

const assistant = (usage) => ({
  type: "message",
  message: { role: "assistant", usage },
});

test("status aggregates only assistant usage from the supplied active branch", () => {
  const branch = [
    assistant({ input: 100, output: 20, cost: { total: 0.01 } }),
    assistant({ input: 200, output: 30, cost: { total: 0.02 } }),
    { type: "message", message: { role: "user", usage: { input: 999 } } },
    { type: "custom", data: { usage: { input: 999 } } },
  ];
  assert.deepEqual(sessionUsage(branch), {
    input: 300,
    output: 50,
    cost: 0.03,
  });
  assert.deepEqual(sessionUsage(branch.slice(0, 1)), {
    input: 100,
    output: 20,
    cost: 0.01,
  });
  assert.deepEqual(sessionUsage([]), { input: 0, output: 0, cost: 0 });
});

test("missing and malformed usage does not poison session stats", () => {
  assert.deepEqual(
    sessionUsage([
      assistant(),
      assistant({ input: -1, output: NaN, cost: { total: Infinity } }),
      assistant({ input: "secret", output: 1 }),
    ]),
    { input: 0, output: 1, cost: 0 },
  );
});

test("Ayu footer is local-only: no external commands or account polling", () => {
  const source = readFileSync(
    new URL("../extensions/ayu-bar/index.ts", import.meta.url),
    "utf8",
  );
  assert.doesNotMatch(source, /pi\.exec|setInterval|fetch\(|quota|codexbar/i);
  assert.match(source, /sessionUsage\(branch\)/);
});
