import test from "node:test";
import assert from "node:assert/strict";
import { GroupNamer } from "../controller.js";

function fixture() {
  const groups = [
    { id: 10, windowId: 1, title: "", color: "purple", collapsed: true },
    { id: 11, windowId: 1, title: "Разные задачи", color: "red", collapsed: false },
    { id: 20, windowId: 2, title: "Другое окно", color: "blue", collapsed: true },
  ];
  const tabs = [
    { id: 100, windowId: 1, groupId: 10, index: 0, url: "https://kaiten.x5.ru/space/1/card/99", title: "Ускорить тесты | Kaiten" },
    { id: 101, windowId: 1, groupId: 10, index: 1, url: "https://scm.x5.ru/t/p/-/merge_requests/2", title: "ATS-1234 test: проверки" },
    { id: 102, windowId: 1, groupId: 11, index: 2, url: "https://scm.x5.ru/t/p/-/merge_requests/3", title: "ATS-5678" },
    { id: 103, windowId: 1, groupId: 11, index: 3, url: "https://scm.x5.ru/t/p/-/merge_requests/4", title: "ATS-9876" },
    { id: 200, windowId: 2, groupId: 20, index: 0, url: "https://scm.x5.ru/t/p/-/merge_requests/5", title: "ATS-3333 fix: исправить" },
  ];
  const session = {}; const local = {}; const updates = [];
  const storage = object => ({
    get: async () => ({ ...object }), set: async value => Object.assign(object, value),
    remove: async key => delete object[key],
  });
  const windows = new Set([1, 2]);
  const browser = {
    storage: { session: storage(session), local: storage(local) },
    runtime: { getURL: path => "chrome-extension://test/" + path },
    windows: {
      get: async id => { if (!windows.has(id)) throw Error("closed"); return { id, type: "normal" }; },
      getAll: async () => [...windows].map(id => ({ id, focused: id === 1, tabs: tabs.filter(tab => tab.windowId === id) })),
    },
    tabGroups: {
      get: async id => { const group = groups.find(g => g.id === id); if (!group) throw Error("gone"); return { ...group }; },
      query: async query => groups.filter(group => group.windowId === query.windowId).map(group => ({ ...group })),
      update: async (id, patch) => { updates.push({ id, patch }); Object.assign(groups.find(group => group.id === id), patch); },
    },
    tabs: { query: async query => tabs.filter(tab => tab.groupId === query.groupId).map(tab => ({ ...tab })) },
  };
  return { browser, groups, tabs, updates, windows };
}

test("no window selection means no mutations", async () => {
  const f = fixture(); const namer = new GroupNamer(f.browser);
  await namer.updateAutomatically();
  assert.deepEqual(f.updates, []);
});
test("only selected window names change, never tab order/color/collapse", async () => {
  const f = fixture(); const namer = new GroupNamer(f.browser); const before = structuredClone(f.tabs);
  await namer.selectWindow(1);
  assert.deepEqual(f.updates, [{ id: 10, patch: { title: "ATS-1234 · Ускорить тесты" } }]);
  assert.equal(f.groups[0].collapsed, true); assert.equal(f.groups[0].color, "purple");
  assert.deepEqual(f.tabs, before);
  assert.equal(f.groups[2].title, "Другое окно");
});
test("refresh is idempotent", async () => {
  const f = fixture(); const namer = new GroupNamer(f.browser);
  await namer.selectWindow(1); await namer.refresh(true);
  assert.equal(f.updates.length, 1);
});
test("locking protects a custom name, including a recreated controller", async () => {
  const f = fixture(); const namer = new GroupNamer(f.browser);
  await namer.selectWindow(1); await namer.setLock(10, "ATS-1234 · Моё название", true);
  f.tabs[0].title = "Другое название | Kaiten";
  const restored = new GroupNamer(f.browser); await restored.refresh(true);
  assert.equal(f.groups[0].title, "ATS-1234 · Моё название 🔒");
  await restored.setLock(10, f.groups[0].title, false); await restored.refresh(true);
  assert.equal(f.groups[0].title, "ATS-1234 · Другое название");
});
test("locks cannot mutate groups outside the selected window", async () => {
  const f = fixture(); const namer = new GroupNamer(f.browser);
  await namer.selectWindow(1);
  await assert.rejects(namer.setLock(20, "wrong", true), /не в выбранном/);
});
test("disabled automation waits for an explicit refresh", async () => {
  const f = fixture(); const namer = new GroupNamer(f.browser);
  await namer.selectWindow(1); await namer.setAuto(false);
  f.tabs[0].title = "Новое название | Kaiten";
  await namer.updateAutomatically();
  assert.equal(f.groups[0].title, "ATS-1234 · Ускорить тесты");
  await namer.refresh(true);
  assert.equal(f.groups[0].title, "ATS-1234 · Новое название");
});
test("closed window clears scope rather than selecting another window", async () => {
  const f = fixture(); const namer = new GroupNamer(f.browser);
  await namer.selectWindow(1); f.windows.delete(1);
  const view = await namer.refresh(true);
  assert.equal(view.windowId, null); assert.equal(f.groups[2].title, "Другое окно");
});
test("changing scope mid-refresh prevents stale writes", async () => {
  const f = fixture(); const namer = new GroupNamer(f.browser);
  await namer.selectWindow(1);
  f.groups[0].title = ""; f.updates.length = 0;
  const normal = f.browser.tabs.query;
  let release, entered;
  const gate = new Promise(resolve => release = resolve);
  const started = new Promise(resolve => entered = resolve);
  f.browser.tabs.query = async query => { if (query.groupId === 10) { entered(); await gate; } return normal(query); };
  const old = namer.refresh(true);
  await started;
  const switched = namer.selectWindow(2);
  await new Promise(resolve => setImmediate(resolve)); release();
  await Promise.all([old, switched]);
  assert.ok(f.updates.every(update => update.id === 20));
});
test("fallback card names are applied without changing membership", async () => {
  const f = fixture(); const before = structuredClone(f.tabs);
  f.tabs[1].title = "Проверки без номера";
  const namer = new GroupNamer(f.browser);
  await namer.selectWindow(1);
  assert.equal(f.groups[0].title, "Kaiten #99 · Ускорить тесты");
  assert.deepEqual(f.tabs, before.map(tab => tab.id === 101 ? { ...tab, title: "Проверки без номера" } : tab));
});
test("a generated URL name in a conflicting group becomes a warning, not a guessed task", async () => {
  const f = fixture();
  f.groups[1].title = "ATS-5678 · kaiten.x5.ru/space/1?card_id=9";
  const namer = new GroupNamer(f.browser);
  await namer.selectWindow(1);
  assert.equal(f.groups[1].title, "⚠ ATS-5678 / ATS-9876");
  assert.equal(f.groups[1].color, "red");
  assert.equal(f.groups[1].collapsed, false);
});
test("cached URL names are repaired while unrelated windows stay untouched", async () => {
  const f = fixture();
  f.groups[0].title = "ATS-1234 · kaiten.x5.ru/space/1/card/99";
  f.tabs[0].title = "kaiten.x5.ru/space/1/card/99";
  const namer = new GroupNamer(f.browser);
  await namer.selectWindow(1);
  assert.equal(f.groups[0].title, "ATS-1234 · проверки");
  assert.equal(f.groups[2].title, "Другое окно");
});
