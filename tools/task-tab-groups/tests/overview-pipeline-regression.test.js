import test from "node:test";
import assert from "node:assert/strict";
import { readStatusInTab } from "../status-reader.js";
import { summarizeGroup } from "../overview-model.js";

// The normal MR API -> normalizer -> group summary path, not a UI-only fixture.
async function summaryFor(t, statuses) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "location");
  Object.defineProperty(globalThis, "location", { configurable: true, value: { origin: "https://scm.x5.ru" } });
  t.after(() => { if (previous) Object.defineProperty(globalThis, "location", previous); else delete globalThis.location; });
  t.mock.method(globalThis, "fetch", async path => {
    const iid = Number(path.split("/").at(-1));
    return new Response(JSON.stringify({ iid, state: "opened", head_pipeline: statuses[iid] === null ? null : { status: statuses[iid] } }), { headers: { "Content-Type": "application/json" } });
  });
  const now = Date.now();
  const resources = await Promise.all(statuses.map(async (_, iid) => ({
    kind: "gitlab", checkedAt: now,
    ...await readStatusInTab({ kind: "gitlab", origin: "https://scm.x5.ru", project: "team/app", id: String(iid) }),
  })));
  return summarizeGroup({ resources }, now).ci;
}

test("successful pipeline is green when another MR has no pipeline", async t => {
  const ci = await summaryFor(t, ["success", null]);
  assert.equal(ci.label, "✓ Пройден");
  assert.equal(ci.tone, "good");
  assert.match(ci.detail, /1.*без CI/);
});
test("all absent pipelines remain neutral, never a fictitious CI pass", async t => {
  const ci = await summaryFor(t, [null, null]);
  assert.equal(ci.label, "Нет pipeline");
  assert.equal(ci.tone, "neutral");
});
test("a genuinely skipped pipeline is named explicitly, not conflated with absence", async t => {
  const ci = await summaryFor(t, ["success", "skipped", null]);
  assert.equal(ci.label, "CI пропущен · 1");
  assert.notEqual(ci.tone, "good");
  assert.match(ci.detail, /1.*без CI/);
});
