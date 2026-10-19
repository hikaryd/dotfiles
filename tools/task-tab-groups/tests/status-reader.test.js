import test from "node:test";
import assert from "node:assert/strict";
import { readStatusInTab } from "../status-reader.js";

const kaiten = { kind: "kaiten", origin: "https://kaiten.x5.ru", id: "99" };
const gitlab = { kind: "gitlab", origin: "https://scm.x5.ru", project: "team/sub/project", id: "42" };
function fixture(t, resource, responses) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, "location");
  Object.defineProperty(globalThis, "location", { configurable: true, value: { origin: resource.origin } });
  t.after(() => { if (previous) Object.defineProperty(globalThis, "location", previous); else delete globalThis.location; });
  const requests = [];
  t.mock.method(globalThis, "fetch", async (path, options) => {
    requests.push({ path, options });
    const next = responses.shift();
    if (next instanceof Error) throw next;
    return new Response(typeof next.body === "string" ? next.body : JSON.stringify(next.body), { status: next.status || 200, headers: { "content-type": next.type || "application/json" } });
  });
  return requests;
}
test("Kaiten returns only overview fields, with responsible members distinct from owner", async t => {
  const requests = fixture(t, kaiten, [{ body: { id: 99, state: 2, title: "Карточка", blocked: true, block_reason: "Ждём", column: { title: "Разработка" }, board: { title: "Команда" }, members: [{ type: 1, full_name: "Участник" }, { type: 2, full_name: "Исполнитель" }], owner: { full_name: "Автор" }, description: "must not leave the tab", secret: "not returned" } }]);
  const { data } = await readStatusInTab(kaiten);
  assert.equal(data.column, "Разработка"); assert.equal(data.blocked, true);
  assert.deepEqual(data.assignees, ["Исполнитель"]); assert.equal(data.owner, "Автор");
  assert.equal("description" in data, false); assert.equal("secret" in data, false);
  assert.equal(requests[0].path, "/api/v1/cards/99");
  assert.equal(requests[0].options.method, "GET");
  assert.equal(requests[0].options.credentials, "same-origin");
  assert.equal(requests[0].options.redirect, "error");
  assert.equal("Authorization" in requests[0].options.headers, false);
});
test("documented Kaiten endpoint is a bounded fallback, not a search for guessed IDs", async t => {
  const requests = fixture(t, kaiten, [{ status: 404, body: {} }, { body: { id: 99, state: 3 } }]);
  assert.equal((await readStatusInTab(kaiten)).data.state, 3);
  assert.deepEqual(requests.map(r => r.path), ["/api/v1/cards/99", "/api/latest/cards/99"]);
});
test("GitLab project namespace is encoded, with MR/CI/review statuses retained", async t => {
  const requests = fixture(t, gitlab, [{ body: { iid: 42, state: "opened", draft: true, head_pipeline: { status: "failed" }, has_conflicts: true, blocking_discussions_resolved: false, detailed_merge_status: "not_approved", reviewers: [{ name: "Ревьюер" }], description: "not returned" } }]);
  const { data } = await readStatusInTab(gitlab);
  assert.equal(requests[0].path, "/api/v4/projects/team%2Fsub%2Fproject/merge_requests/42");
  assert.equal(data.pipeline, "failed"); assert.equal(data.draft, true);
  assert.equal(data.hasConflicts, true); assert.equal(data.discussionsResolved, false);
  assert.deepEqual(data.reviewers, ["Ревьюер"]); assert.equal("description" in data, false);
});
test("no pipeline and an unavailable pipeline field remain distinct", async t => {
  fixture(t, gitlab, [{ body: { iid: 42, state: "merged", head_pipeline: null } }, { body: { iid: 42, state: "merged" } }]);
  assert.equal((await readStatusInTab(gitlab)).data.pipeline, "none");
  assert.equal((await readStatusInTab(gitlab)).data.pipeline, null);
});
test("API auth failures and HTML login responses are explicit, not empty successful states", async t => {
  fixture(t, gitlab, [{ status: 401, body: {} }, { status: 403, body: {} }, { type: "text/html", body: "<html>Sign in</html>" }]);
  assert.equal((await readStatusInTab(gitlab)).error.code, "auth");
  assert.equal((await readStatusInTab(gitlab)).error.code, "forbidden");
  assert.equal((await readStatusInTab(gitlab)).error.code, "auth");
});
test("rate limits do not trigger fallback/retry storms", async t => {
  const requests = fixture(t, kaiten, [{ status: 429, body: {} }]);
  assert.equal((await readStatusInTab(kaiten)).error.code, "rate-limit");
  assert.equal(requests.length, 1);
});
test("wrong IDs, unexpected schemas and invalid JSON cannot be assigned to the task", async t => {
  fixture(t, gitlab, [{ body: { iid: 43, state: "opened" } }, { body: { iid: 42, state: "other" } }, { body: "{" }]);
  for (let i = 0; i < 3; i++) assert.equal((await readStatusInTab(gitlab)).error.code, "schema");
});
test("timeouts and redirects/network errors have bounded, non-sensitive messages", async t => {
  fixture(t, gitlab, [new DOMException("secret URL", "TimeoutError"), new TypeError("secret URL")]);
  assert.equal((await readStatusInTab(gitlab)).error.code, "timeout");
  const result = await readStatusInTab(gitlab);
  assert.equal(result.error.code, "network"); assert.ok(!result.error.message.includes("secret"));
});
test("navigation to a different origin refuses to issue a request", async t => {
  const requests = fixture(t, kaiten, []);
  assert.equal((await readStatusInTab(gitlab)).error.code, "navigation");
  assert.equal((await readStatusInTab({ ...kaiten, origin: "https://evil.test" })).error.code, "navigation");
  assert.equal(requests.length, 0);
});
