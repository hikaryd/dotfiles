import assert from "node:assert/strict";
import test from "node:test";
import {
  createQuotaReader,
  formatReset,
  formatTokens,
  parseQuota,
} from "../extensions/ayu-bar/quota.mjs";

const reset = "2026-10-14T07:10:53Z";
const window = (usedPercent = 21, windowMinutes = 10080) => ({
  usedPercent,
  windowMinutes,
  resetsAt: reset,
});
const payload = (secondary = window()) =>
  JSON.stringify([{ provider: "codex", usage: { primary: null, secondary } }]);

test("uses the account-wide weekly window, not a model-specific limit", () => {
  const data = JSON.parse(payload());
  data[0].usage.additionalRateLimits = [{ secondary: window(99) }];
  data[0].usage.tertiary = window(88);
  assert.deepEqual(parseQuota(JSON.stringify(data)), {
    weekly: {
      usedPercent: 21,
      remainingPercent: 79,
      resetsAt: Date.parse(reset),
    },
  });
});

test("finds weekly primary when the windows are reversed", () => {
  const data = JSON.parse(payload(window(60, 300)));
  data[0].usage.primary = window(12);
  assert.equal(parseQuota(JSON.stringify(data)).weekly.remainingPercent, 88);
});

test("does not substitute a five-hour, code-review or per-model limit", () => {
  const data = JSON.parse(payload(window(5, 300)));
  data[0].usage.tertiary = window();
  data[0].usage.additionalRateLimits = [{ secondary: window() }];
  assert.throws(() => parseQuota(JSON.stringify(data)), /weekly/);
});

test("valid zero and exhausted quotas remain distinct from missing data", () => {
  assert.equal(parseQuota(payload(window(0))).weekly.remainingPercent, 100);
  assert.equal(parseQuota(payload(window(100))).weekly.remainingPercent, 0);
  for (const bad of [
    null,
    {},
    window(-1),
    window(101),
    window("21"),
    window(null),
    window(21, 300),
  ]) {
    assert.throws(() => parseQuota(payload(bad)));
  }
});

test("missing reset stays unknown", () => {
  const data = window();
  delete data.resetsAt;
  assert.equal(parseQuota(payload(data)).weekly.resetsAt, undefined);
});

test("malformed JSON, wrong providers, ambiguous accounts and command errors are rejected", () => {
  for (const data of [
    "bad",
    "null",
    "[]",
    '{"provider":"openai"}',
    JSON.stringify([...JSON.parse(payload()), ...JSON.parse(payload())]),
    '{"provider":"codex","error":{"message":"failed"}}',
  ]) {
    assert.throws(() => parseQuota(data));
  }
});

test("reset countdown never pretends an elapsed window was restored", () => {
  assert.equal(formatReset(undefined, 0), "");
  assert.equal(formatReset(0, 0), "resetting");
  assert.equal(formatReset(-1000, 0), "resetting");
  assert.equal(formatReset(2 * 86400000 + 3 * 3600000, 0), "2d 3h");
  assert.equal(formatReset(90 * 60000, 0), "1h 30m");
  assert.equal(formatReset(60000, 0), "1m");
});

test("compact context counts include 400k", () => {
  assert.equal(formatTokens(0), "0");
  assert.equal(formatTokens(12000), "12k");
  assert.equal(formatTokens(400000), "400k");
  assert.equal(formatTokens(1000000), "1M");
});

test("reader coalesces overlapping requests and throttles automatic refresh", async () => {
  let calls = 0;
  let resolve;
  let clock = 0;
  const reader = createQuotaReader(
    () => {
      calls++;
      return new Promise((done) => {
        resolve = done;
      });
    },
    () => {},
    () => clock,
  );
  const first = reader.refresh();
  const second = reader.refresh(true);
  assert.equal(first, second);
  await Promise.resolve();
  assert.equal(calls, 1);
  resolve({ code: 0, stdout: payload() });
  await first;
  await reader.refresh();
  assert.equal(calls, 1);
  assert.equal(reader.getState().quota.weekly.remainingPercent, 79);
  clock = 16000;
  const next = reader.refresh();
  await Promise.resolve();
  assert.equal(calls, 2);
  resolve({ code: 0, stdout: payload(window(30)) });
  await next;
  assert.equal(reader.getState().quota.weekly.remainingPercent, 70);
  reader.stop();
});

test("failed refresh retains the real reading and marks it stale", async () => {
  let fail = false;
  const reader = createQuotaReader(async () =>
    fail
      ? { code: 1, stdout: "secret error not displayed" }
      : { code: 0, stdout: payload() },
  );
  await reader.refresh();
  fail = true;
  await reader.refresh(true);
  assert.equal(reader.getState().quota.weekly.remainingPercent, 79);
  assert.equal(reader.getState().error, "unavailable");
  reader.stop();
});

test("first failure is unavailable, not a fabricated allowance", async () => {
  const reader = createQuotaReader(async () => ({
    code: 0,
    stdout: "not JSON",
  }));
  await reader.refresh();
  assert.equal(reader.getState().quota, undefined);
  assert.equal(reader.getState().error, "unavailable");
  reader.stop();
});

test("synchronous process failures do not wedge single-flight state", async () => {
  let calls = 0;
  const reader = createQuotaReader(() => {
    calls++;
    throw new Error("not installed");
  });
  await reader.refresh();
  await reader.refresh(true);
  assert.equal(calls, 2);
  assert.equal(reader.getState().error, "unavailable");
  reader.stop();
});

test("disposal aborts the child and ignores a late result", async () => {
  let resolve;
  let signal;
  let updates = 0;
  const reader = createQuotaReader(
    (active) => {
      signal = active;
      return new Promise((done) => {
        resolve = done;
      });
    },
    () => {
      updates++;
    },
  );
  const pending = reader.refresh();
  await Promise.resolve();
  reader.stop();
  assert.equal(signal.aborted, true);
  resolve({ code: 0, stdout: payload() });
  await pending;
  assert.equal(reader.getState().quota, undefined);
  assert.equal(updates, 1);
  await reader.refresh(true);
  assert.equal(updates, 1);
});
