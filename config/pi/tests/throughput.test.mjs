import assert from "node:assert/strict";
import test from "node:test";
import {
  latestThroughput,
  responseThroughput,
} from "../extensions/ayu-bar/throughput.mjs";

const response = (overrides = {}) => ({
  role: "assistant",
  provider: "openai",
  model: "gpt-6.1-sol",
  stopReason: "stop",
  usage: { output: 240, reasoning: 100, input: 400000 },
  durationMs: 2000,
  ...overrides,
});
const entry = (message) => ({ type: "message", message });

test("TPS uses reported output tokens and the response duration, not input tokens", () => {
  assert.deepEqual(responseThroughput(response()), {
    provider: "openai",
    model: "gpt-6.1-sol",
    tps: 120,
  });
});

test("reasoning tokens are already included in output, not counted twice", () => {
  assert.equal(
    responseThroughput(
      response({ usage: { output: 100, reasoning: 80 }, durationMs: 1000 }),
    ).tps,
    100,
  );
});

test("valid empty responses have zero throughput", () => {
  assert.equal(responseThroughput(response({ usage: { output: 0 } })).tps, 0);
});

test("old, malformed, pending, deferred and failed responses have unknown TPS", () => {
  for (const durationMs of [undefined, null, 0, -1, NaN, Infinity, "2000"]) {
    assert.equal(responseThroughput(response({ durationMs })), undefined);
  }
  for (const output of [undefined, null, -1, NaN, Infinity, "240"]) {
    assert.equal(
      responseThroughput(response({ usage: { output } })),
      undefined,
    );
  }
  for (const stopReason of ["pending", "error", "aborted", "deferred"]) {
    assert.equal(responseThroughput(response({ stopReason })), undefined);
  }
  assert.equal(
    responseThroughput({ role: "toolResult", durationMs: 1000 }),
    undefined,
  );
  assert.equal(responseThroughput(null), undefined);
});

test("tool-call responses count, but tool execution and later failures do not alter TPS", () => {
  const messages = [
    entry(response({ stopReason: "toolUse" })),
    entry({ role: "toolResult", durationMs: 60000 }),
    entry(response({ stopReason: "aborted", durationMs: 1000 })),
  ];
  assert.equal(latestThroughput(messages).tps, 120);
});

test("latest completed response wins and retains its model identity", () => {
  assert.deepEqual(
    latestThroughput([
      entry(response()),
      { type: "custom", data: "ignored" },
      entry(
        response({ provider: "anthropic", model: "other", durationMs: 4000 }),
      ),
    ]),
    { provider: "anthropic", model: "other", tps: 60 },
  );
});

test("missing timing on the latest response is not filled with an older TPS", () => {
  assert.equal(
    latestThroughput([
      entry(response()),
      entry(response({ durationMs: undefined })),
    ]),
    undefined,
  );
  assert.equal(latestThroughput([]), undefined);
});
