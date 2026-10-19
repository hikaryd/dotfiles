import test from "node:test";
import assert from "node:assert/strict";
import { resourceForTab, resourcesForTabs, groupCategory, isStale, matchesSearch, STALE_MS } from "../overview-model.js";

const card = path => ({ url: "https://kaiten.x5.ru" + path });
const mr = (project, id = 42) => ({ url: `https://scm.x5.ru/${project}/-/merge_requests/${id}` });

test("card identity comes from path/query/fragment, never the ATS title", () => {
  for (const path of ["/space/1/card/99", "/cards/99", "/space/1?card_id=99", "/space/1#card_id=99", "/space/1#?card_id=99"]) {
    assert.equal(resourceForTab({ ...card(path), title: "ATS-1234567" }).id, "99");
  }
  assert.equal(resourceForTab({ ...card("/space/1"), title: "ATS-1234567" }), null);
  assert.equal(resourceForTab(card("/card/99?card_id=100")), null);
});
test("only exact HTTPS hosts, usable URLs and card/MR routes are accepted", () => {
  for (const url of ["not a url", "https://kaiten.x5.ru.evil.org/card/99", "http://kaiten.x5.ru/card/99", "https://user:secret@kaiten.x5.ru/card/99", "https://kaiten.x5.ru:444/card/99", "https://kaiten.x5.ru/auth?card_id=99", "https://scm.x5.ru/users/sign_in", "https://scm.x5.ru/t/p/-/pipelines/42"]) assert.equal(resourceForTab({ url }), null);
  assert.equal(resourceForTab(mr("team/sub/project")).project, "team/sub/project");
});
test("deduplicate MR tabs but never same iid in different repositories", () => {
  const resources = resourcesForTabs([{ ...mr("t/a"), id: 1, index: 4 }, { ...mr("t/a"), id: 2, index: 1 }, { ...mr("t/b"), id: 3, index: 2 }]);
  assert.equal(resources.length, 2);
  assert.deepEqual(resources[0].tabIds, [2, 1]);
  assert.notEqual(resources[0].key, resources[1].key);
});
const now = 1_000_000;
const resource = (kind, data, extra = {}) => ({ kind, data, checkedAt: now, ...extra });
const group = (...resources) => ({ title: "ATS-123 · Проверки", keys: ["ATS-123"], resources });
test("done requires actual current card/MR states, not a merged MR alone", () => {
  assert.equal(groupCategory(group(resource("kaiten", { state: 2 }), resource("gitlab", { state: "merged" })), now), "active");
  assert.equal(groupCategory(group(resource("kaiten", { state: 3 }), resource("gitlab", { state: "merged" })), now), "done");
  assert.equal(groupCategory(group(resource("kaiten", { state: 3 }), resource("gitlab", { state: "opened" })), now), "active");
  assert.equal(groupCategory(group(resource("kaiten", { state: 1, archived: true })), now), "active");
  assert.equal(groupCategory(group(resource("gitlab", { state: "closed" })), now), "unknown");
  assert.equal(groupCategory(group(resource("gitlab", { state: "merged" })), now), "unknown");
});
test("missing, expired and failed reads cannot masquerade as a completed task", () => {
  assert.equal(groupCategory(group(), now), "unknown");
  assert.equal(groupCategory(group(resource("kaiten", null, { error: { code: "permission" } })), now), "unknown");
  const stale = resource("kaiten", { state: 3 }, { checkedAt: now - STALE_MS });
  assert.equal(isStale(stale, now), true);
  assert.equal(groupCategory(group(stale), now), "unknown");
  assert.equal(groupCategory(group(resource("kaiten", { state: 3 }, { error: { code: "auth" } })), now), "attention");
});
test("conflicts, blocked cards, failed CI and unresolved discussions draw attention", () => {
  for (const data of [{ blocked: true }, { hasConflicts: true }, { pipeline: "failed" }, { discussionsResolved: false }]) assert.equal(groupCategory(group(resource("gitlab", data)), now), "attention");
  assert.equal(groupCategory({ ...group(), state: "conflict" }, now), "attention");
});
test("search combines task, project, API title and responsible person without HTML", () => {
  const g = group(resource("gitlab", { title: "Контракт", assignees: ["Тестовый Автор"] }, { project: "team/docs" }));
  assert.equal(matchesSearch(g, "ATS-123 контракт docs автор"), true);
  assert.equal(matchesSearch(g, "другая"), false);
});
