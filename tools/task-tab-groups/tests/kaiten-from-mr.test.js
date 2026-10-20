import test from "node:test";
import assert from "node:assert/strict";
import { resourcesForTabs } from "../overview-model.js";
const mr = (id, title) => ({ id, index: id, title, url: `https://scm.x5.ru/team/app/-/merge_requests/${id}` });

test("MR-only group derives one Kaiten card from an explicit task number, not the MR iid", () => {
  const resources = resourcesForTabs([mr(42, "ATS-3804423 fix: Проверки"), mr(43, "3804423 исправление (!43)")]);
  const cards = resources.filter(r => r.kind === "kaiten");
  assert.equal(cards.length, 1);
  assert.equal(cards[0].id, "3804423");
  assert.deepEqual(cards[0].tabIds, []);
  assert.deepEqual(cards[0].sourceTabIds, [42, 43]);
});
test("explicit card URL takes precedence over task numbers in MR titles", () => {
  const resources = resourcesForTabs([mr(42, "ATS-3804423 fix"), {id: 100, index:0, url:"https://kaiten.x5.ru/space/1/card/99"}]);
  assert.deepEqual(resources.filter(r=>r.kind==='kaiten').map(r=>r.id), ['99']);
});
test("MR iid, unrelated host, group title, URL query and a loading tab are not evidence of a task number", () => {
  for (const tab of [mr(3804423, "fix: Проверки"), {...mr(42,"ATS-3804423"),url:'https://evil.test/team/app/-/merge_requests/42'}, {...mr(42,"fix"),url:mr(42,"fix").url+'?q=ATS-3804423'}, {...mr(42,"ATS-3804423"),status:'loading'}]) {
    assert.equal(resourcesForTabs([tab]).filter(r=>r.kind==='kaiten').length,0);
  }
});
test("different task keys in one MR-only group stay separate", () => {
  const cards=resourcesForTabs([mr(42,"ATS-3804423 fix"),mr(43,"ATS-3804424 fix")]).filter(r=>r.kind==='kaiten');
  assert.deepEqual(cards.map(r=>r.id),['3804423','3804424']);
});
