import assert from "node:assert/strict";
import test from "node:test";
import {
  createQuotaReader,
  fetchCodexQuota,
  formatQuota,
  parseQuota,
} from "../extensions/ayu-bar/quota.mjs";

const now = Date.parse("2026-10-14T00:00:00Z");
const window = (used_percent = 21, limit_window_seconds = 604800) => ({
  used_percent,
  limit_window_seconds,
  reset_at: (now + 86400000) / 1000,
});
const payload = (secondary_window = window()) => ({
  rate_limit: { primary_window: window(5, 18000), secondary_window },
});
const jwt = (claims) =>
  `header.${Buffer.from(JSON.stringify(claims)).toString("base64url")}.signature`;
const token = jwt({
  "https://api.openai.com/auth": { chatgpt_account_id: "fixture-account" },
});

test("reads only the account-wide weekly allowance, not model/review limits", () => {
  const data = payload();
  data.additional_rate_limits = [
    { rate_limit: { secondary_window: window(99) } },
  ];
  data.code_review_rate_limit = { secondary_window: window(88) };
  assert.deepEqual(parseQuota(data), {
    weekly: { usedPercent: 21, remainingPercent: 79, resetsAt: now + 86400000 },
  });
  assert.throws(() => parseQuota({ ...data, rate_limit: undefined }));
  assert.throws(() => parseQuota(payload(window(5, 18000))));
});

test("weekly window can be primary; malformed and absent data never becomes 100%", () => {
  assert.equal(
    parseQuota({ rate_limit: { primary_window: window(12) } }).weekly
      .remainingPercent,
    88,
  );
  assert.equal(parseQuota(payload(window(0))).weekly.remainingPercent, 100);
  assert.equal(parseQuota(payload(window(100))).weekly.remainingPercent, 0);
  for (const value of [
    null,
    {},
    window(-1),
    window(101),
    window("21"),
    window(NaN),
  ])
    assert.throws(() => parseQuota(payload(value)));
  assert.equal(
    parseQuota(payload({ ...window(), reset_at: null })).weekly.resetsAt,
    undefined,
  );
  for (const data of [null, [], {}, { rate_limit: {} }])
    assert.throws(() => parseQuota(data));
});

test("fetch uses refreshed Pi legacy auth with fixed host, timeout, and no redirects", async () => {
  const controller = new AbortController();
  let calls = 0;
  const quota = await fetchCodexQuota(
    async () => ({ source: "OAuth", auth: { apiKey: token } }),
    controller.signal,
    async (url, options) => {
      calls++;
      assert.equal(url, "https://chatgpt.com/backend-api/wham/usage");
      assert.equal(options.headers.Authorization, `Bearer ${token}`);
      assert.equal(options.headers["ChatGPT-Account-Id"], "fixture-account");
      assert.equal(options.redirect, "error");
      assert.ok(options.signal instanceof AbortSignal);
      return { ok: true, json: async () => payload() };
    },
  );
  assert.equal(calls, 1);
  assert.equal(quota.weekly.remainingPercent, 79);
});

test("missing login and API keys never cause network calls or credential disclosure", async () => {
  for (const auth of [
    undefined,
    { source: "OPENAI_API_KEY", auth: { apiKey: "fixture-secret" } },
  ]) {
    await assert.rejects(
      fetchCodexQuota(
        async () => auth,
        new AbortController().signal,
        () => assert.fail("must not fetch"),
      ),
      { message: "login_required" },
    );
  }
  await assert.rejects(
    fetchCodexQuota(
      async () => ({ source: "OAuth", auth: { apiKey: "fixture-secret" } }),
      new AbortController().signal,
      () => assert.fail("must not fetch"),
    ),
    { message: "unavailable" },
  );
});

test("HTTP errors and cancellation fail closed", async () => {
  const auth = async () => ({ source: "OAuth", auth: { apiKey: token } });
  await assert.rejects(
    fetchCodexQuota(auth, new AbortController().signal, async () => ({
      ok: false,
      status: 401,
    })),
    { message: "unavailable" },
  );
  const controller = new AbortController();
  controller.abort();
  await assert.rejects(
    fetchCodexQuota(auth, controller.signal, () =>
      assert.fail("must not fetch"),
    ),
  );
});

test("reader deduplicates refresh, throttles polling and keeps stale real readings", async () => {
  let clock = now;
  let calls = 0;
  let release;
  const reader = createQuotaReader(
    async () => {
      calls++;
      if (calls > 1) throw new Error("fixture-secret");
      await new Promise((resolve) => {
        release = resolve;
      });
      return parseQuota(payload());
    },
    () => {},
    () => clock,
  );
  const pending = reader.refresh();
  assert.equal(reader.refresh(true), pending);
  await Promise.resolve();
  release();
  await pending;
  await reader.refresh();
  assert.equal(calls, 1);
  clock += 60000;
  await reader.refresh();
  assert.equal(reader.getState().quota.weekly.remainingPercent, 79);
  assert.equal(reader.getState().error, "unavailable");
  assert.match(formatQuota(reader.getState(), clock), /79% left.*stale/);
  reader.stop();
  await reader.refresh(true);
  assert.equal(calls, 2);
});

test("stop cancels pending work and ignores late completions", async () => {
  let signal;
  let release;
  let changes = 0;
  const reader = createQuotaReader(
    async (s) => {
      signal = s;
      await new Promise((resolve) => {
        release = resolve;
      });
      return parseQuota(payload());
    },
    () => changes++,
  );
  const pending = reader.refresh();
  await Promise.resolve();
  reader.stop();
  assert.ok(signal.aborted);
  release();
  await pending;
  assert.equal(changes, 1);
  assert.equal(reader.getState().quota, undefined);
});

test("footer text distinguishes loading, login, unavailable, stale and resetting", () => {
  assert.match(formatQuota({ loading: true }, now), /Codex 7d.*…/);
  assert.match(
    formatQuota({ error: "login_required" }, now),
    /\/login.*OpenAI Codex/,
  );
  assert.match(formatQuota({}, now), /unavailable/);
  const state = { quota: parseQuota(payload()), updatedAt: now };
  assert.match(formatQuota(state, now), /79% left.*1d 0h/);
  assert.match(formatQuota(state, now + 6 * 60000), /stale/);
  assert.match(formatQuota(state, now + 86400000), /resetting/);
  assert.doesNotMatch(formatQuota(state, now + 86400000), /100%/);
});
