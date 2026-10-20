import test from "node:test";
import assert from "node:assert/strict";
import { readStatusInTab } from "../status-reader.js";
import { summarizeGroup, statusDiagnostics, groupCategory, STALE_MS } from "../overview-model.js";

const resource = { kind: "gitlab", origin: "https://scm.x5.ru", project: "team/sub/app", id: "42" };
const now = 1_000_000;
const approvedBy = [{ user: { id: 7, name: "Ревьюер", email: "must-not-leak" } }];
function fixture(t, approvals, { background = false, status = 200 } = {}) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "location");
  Object.defineProperty(globalThis, "location", { configurable: true, value: background
    ? { protocol: "chrome-extension:", origin: "chrome-extension://test" }
    : { origin: resource.origin } });
  t.after(() => { if (previous) Object.defineProperty(globalThis, "location", previous); else delete globalThis.location; });
  const requests = [];
  t.mock.method(globalThis, "fetch", async (url, options) => {
    requests.push({ url, options });
    return Response.json(url.endsWith("/approvals") ? approvals : {
      iid: 42, state: "opened", head_pipeline: { status: "success" },
      reviewers: [{ name: "Назначен, но не одобрил" }],
    }, { status: url.endsWith("/approvals") ? status : 200 });
  });
  return requests;
}
const group = (...reviews) => ({ resources: reviews.map((review, index) => ({
  ...resource, key: String(index), checkedAt: now,
  data: { state: "opened", pipeline: "success", review },
})) });

for (const background of [false, true]) test(`GitLab review uses real approvals via a read-only ${background ? "worker" : "tab"} request`, async t => {
  const requests = fixture(t, { iid: 42, approved: true, approvals_required: 1, approvals_left: 0, approved_by: approvedBy }, { background });
  const { data } = await readStatusInTab(resource, background);
  assert.equal(data.review?.status, "approved");
  assert.deepEqual(data.review.approvedBy, ["Ревьюер"]);
  assert.equal(data.review.count, 1);
  assert.equal(data.review.required, 1);
  assert.equal(data.review.remaining, 0);
  assert.equal(requests.length, 2);
  assert.equal(requests[1].url, (background ? resource.origin : "") + "/api/v4/projects/team%2Fsub%2Fapp/merge_requests/42/approvals");
  assert.equal(requests[1].options.method, "GET");
  assert.equal(requests[1].options.credentials, background ? "include" : "same-origin");
  assert.equal(JSON.stringify(data).includes("must-not-leak"), false);
});
test("approved:true with zero required and zero approvers does not mean a human reviewed it", async t => {
  fixture(t, { iid: 42, approved: true, approvals_required: 0, approvals_left: 0, approved_by: [] });
  const { data } = await readStatusInTab(resource);
  assert.equal(data.review?.status, "not-required");
  const summary = summarizeGroup(group(data.review), now).review;
  assert.equal(summary.label, "Нет одобрений");
  assert.notEqual(summary.tone, "good");
});
test("optional but actual approvals count as reviewed; assigned reviewers and CI do not", async t => {
  fixture(t, { iid: 42, approved: true, approvals_required: 0, approvals_left: 0, approved_by: approvedBy });
  assert.equal((await readStatusInTab(resource)).data.review?.status, "approved");
});
const automation = { user: { id: 99, username: "SRV.SAMEDI", name: "serviceAccount, SRV.SAMEDI" } };
for (const background of [false, true]) for (const required of [0, 1]) {
  test(`service-account approval alone is not a review (required=${required}, background=${background})`, async t => {
    fixture(t, { iid: 42, approved: true, approvals_required: required, approvals_left: 0, approved_by: [automation] }, { background });
    const review = (await readStatusInTab(resource, background)).data.review;
    assert.notEqual(review.status, "approved", "SRV.SAMEDI must not give a green review");
    assert.equal(review.status, required ? "pending" : "not-required");
    assert.equal(review.count, 0);
    assert.deepEqual(review.approvedBy, []);
    assert.equal(review.excludedCount, 1);
    assert.equal(review.remaining, required);
    assert.notEqual(summarizeGroup(group(review), now).review.tone, "good");
  });
}
test("service-account plus actual approval counts only the person", async t => {
  fixture(t, { approved: true, approvals_required: 1, approvals_left: 0, approved_by: [automation, ...approvedBy] });
  const review = (await readStatusInTab(resource)).data.review;
  assert.equal(review.status, "approved");
  assert.equal(review.count, 1);
  assert.equal(review.excludedCount, 1);
  assert.deepEqual(review.approvedBy, ["Ревьюер"]);
  assert.equal(summarizeGroup(group(review), now).review.tone, "good");
});
test("excluded account cannot fill a required approval slot", async t => {
  fixture(t, { approved: true, approvals_required: 2, approvals_left: 0, approved_by: [automation, ...approvedBy] });
  const review = (await readStatusInTab(resource)).data.review;
  assert.equal(review.status, "pending");
  assert.equal(review.count, 1);
  assert.equal(review.remaining, 1);
});
test("account exclusion is case-insensitive, exact, and not based on display name", async t => {
  fixture(t, { approved: true, approved_by: [
    { user: { ...automation.user, username: "srv.samedi" } },
    { user: { ...automation.user, username: "srv.samedi" } },
    { user: { id: 8, username: "srv.samedi.other", name: "serviceAccount, SRV.SAMEDI" } },
  ] });
  const review = (await readStatusInTab(resource)).data.review;
  assert.equal(review.status, "approved");
  assert.equal(review.count, 1);
  assert.equal(review.excludedCount, 1);
  assert.deepEqual(review.approvedBy, ["serviceAccount, SRV.SAMEDI"]);
});
test("a remaining required approval cannot be hidden by approvals from other people", async t => {
  fixture(t, { iid: 42, approved: false, approvals_required: 2, approvals_left: 1, approved_by: [...approvedBy, { user: { id: 8, name: "Второй" } }] });
  const { data } = await readStatusInTab(resource);
  assert.equal(data.review?.status, "pending");
  assert.equal(data.review.count, 2);
  assert.equal(data.review.remaining, 1);
  assert.equal(summarizeGroup(group(data.review), now).review.tone, "warn");
});
test("Community Edition approvals can omit iid and rule counters", async t => {
  fixture(t, { approved: true, approved_by: approvedBy });
  const review = (await readStatusInTab(resource)).data.review;
  assert.equal(review?.status, "approved");
  assert.equal(review.required, null);
});
test("older GitLab may expose rule counters without approved boolean", async t => {
  fixture(t, { iid: 42, approvals_required: 1, approvals_left: 0, approved_by: approvedBy });
  assert.equal((await readStatusInTab(resource)).data.review?.status, "approved");
});
test("approval endpoint failure keeps MR and CI but never reuses a successful review", async t => {
  fixture(t, {}, { status: 403 });
  const { data, error } = await readStatusInTab(resource);
  assert.equal(error, undefined);
  assert.equal(data.state, "opened");
  assert.equal(data.pipeline, "success");
  assert.equal(data.review?.status, "unknown");
  assert.equal(data.review.error.code, "forbidden");
  const summary = summarizeGroup(group(data.review), now);
  assert.notEqual(summary.review.tone, "good");
  assert.equal(summary.ci.tone, "good");
});
for (const [name, payload] of [
  ["wrong iid", { iid: 43, approved: true, approved_by: approvedBy }],
  ["missing approvers", { approved: true }],
  ["missing required approvers", { approved: true, approvals_required: 1, approvals_left: 0, approved_by: [] }],
  ["missing state", { approved_by: approvedBy }],
  ["malformed approver", { approved: true, approved_by: [null] }],
  ["malformed username", { approved: true, approved_by: [{ user: { id: 7, username: 123 } }] }],
  ["contradictory requirements", { approved: true, approvals_left: 1, approved_by: approvedBy }],
]) test(`malformed approvals are unknown (${name})`, async t => {
  fixture(t, payload);
  const review = (await readStatusInTab(resource)).data.review;
  assert.equal(review?.status, "unknown");
  assert.equal(review.error.code, "schema");
});
test("review summaries aggregate every MR and never turn missing/stale/no-approval data green", () => {
  const approved = { status: "approved", count: 1, approvedBy: ["Ревьюер"] };
  assert.equal(summarizeGroup(group(approved, approved), now).review.tone, "good");
  for (const other of [undefined, { status: "pending" }, { status: "not-required" }, { status: "unknown" }]) {
    assert.notEqual(summarizeGroup(group(approved, other), now).review.tone, "good");
  }
  const stale = group(approved);
  stale.resources[0].checkedAt = now - STALE_MS;
  assert.notEqual(summarizeGroup(stale, now).review.tone, "good");
  const report = statusDiagnostics({ groups: [group(approved, { status: "unknown", error: { code: "forbidden", message: "private text" } })] }, "test", now);
  assert.deepEqual(report.review, { approved: 1, unknown: 1 });
  assert.deepEqual(report.reviewErrors, { forbidden: 1 });
  assert.equal(JSON.stringify(report).includes("Ревьюер"), false);
  assert.equal(JSON.stringify(report).includes("private text"), false);
});
test("only required pending reviews, not an assigned reviewer or optional zero approvals, require attention", () => {
  assert.equal(groupCategory(group({ status: "pending" }), now), "attention");
  assert.equal(groupCategory(group({ status: "not-required" }), now), "active");
  assert.equal(groupCategory(group({ status: "unknown", error: { code: "forbidden" } }), now), "attention");
});
