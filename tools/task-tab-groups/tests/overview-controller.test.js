import test from "node:test";
import assert from "node:assert/strict";
import { GroupNamer } from "../controller.js";
import { GroupOverview } from "../overview-controller.js";
import { STATUS_ORIGINS, REFRESH_MS } from "../overview-model.js";

function fixture() {
  const groups = [{ id: 10, windowId: 1, title: "ATS-123 · Проверки", color: "purple", collapsed: true }, { id: 20, windowId: 2, title: "Другое окно", color: "blue", collapsed: true }];
  const tabs = [
    { id: 100, windowId: 1, groupId: 10, index: 0, title: "Проверки | Kaiten", url: "https://kaiten.x5.ru/space/1/card/99", status: "complete" },
    { id: 101, windowId: 1, groupId: 10, index: 1, title: "ATS-123 fix: проверки", url: "https://scm.x5.ru/team/app/-/merge_requests/42", status: "complete" },
    { id: 200, windowId: 2, groupId: 20, index: 0, url: "https://scm.x5.ru/team/other/-/merge_requests/42", status: "complete" },
  ];
  const session = { windowId: 1 }; const local = {}; const calls = []; const mutations = [];
  const granted = new Set(STATUS_ORIGINS);
  const storage = object => ({
    get: async () => structuredClone(object), set: async value => Object.assign(object, structuredClone(value)), remove: async key => { delete object[key]; },
  });
  const getTab = async id => { const tab = tabs.find(t => t.id === id); if (!tab) throw Error("gone"); return { ...tab }; };
  const browser = {
    storage: { session: storage(session), local: storage(local) },
    runtime: { getURL: path => "chrome-extension://test/" + path },
    permissions: { contains: async ({ origins }) => origins.every(origin => granted.has(origin)) },
    scripting: { executeScript: async options => {
      calls.push(options);
      const resource = options.args[0];
      return [{ result: { data: resource.kind === "kaiten" ? { state: 2, assignees: [] } : { state: "opened", pipeline: "success", reviewers: [] } } }];
    } },
    tabs: {
      get: getTab,
      query: async query => tabs.filter(t => Object.entries(query).every(([key, value]) => t[key] === value)).map(t => ({ ...t })),
      update: async (id, patch) => { await getTab(id); mutations.push({ kind: "tab", id, patch }); },
      create: async properties => { mutations.push({ kind: "create", properties }); },
    },
    tabGroups: {
      query: async ({ windowId }) => groups.filter(g => g.windowId === windowId).map(g => ({ ...g })),
      get: async id => { const group = groups.find(g => g.id === id); if (!group) throw Error("gone"); return { ...group }; },
      update: async (id, patch) => { mutations.push({ kind: "group", id, patch }); Object.assign(groups.find(g => g.id === id), patch); },
    },
    windows: {
      get: async id => ({ id, type: "normal" }),
      getAll: async () => [1, 2].map(id => ({ id, tabs: tabs.filter(t => t.windowId === id) })),
      update: async (id, patch) => { mutations.push({ kind: "window", id, patch }); },
    },
  };
  let now = 1_000_000;
  const namer = new GroupNamer(browser);
  const overview = new GroupOverview(browser, namer, () => now);
  const backgroundCalls = [];
  overview.readInBackground = async resource => {
    backgroundCalls.push(resource);
    return { data: resource.kind === "kaiten" ? { state: 2, assignees: [] } : { state: "opened", pipeline: "success", reviewers: [] } };
  };
  return { browser, namer, overview, groups, tabs, session, granted, calls, backgroundCalls, mutations, advance: ms => { now += ms; } };
}
test("overview is selected-window-only, read-only, and keeps browser tab order", async () => {
  const f = fixture();
  const view = await f.overview.snapshot();
  assert.equal(view.groups.length, 1); assert.equal(view.groups[0].id, 10);
  assert.deepEqual(view.groups[0].tabs.map(t => t.id), [100, 101]);
  assert.equal(f.calls.length, 2); assert.deepEqual(f.mutations, []);
  assert.equal(view.groups[0].resources[0].data.state, 2);
});
test("MR-only cards read through the worker without creating or waking a tab", async () => {
  const f = fixture(); f.tabs.shift();
  const view = await f.overview.snapshot();
  const card = view.groups[0].resources.find(r => r.kind === "kaiten");
  assert.equal(card.id, "123"); assert.equal(card.data.state, 2);
  assert.deepEqual(card.tabIds, []); assert.deepEqual(card.sourceTabIds, [101]);
  assert.equal(f.calls.length, 1); assert.equal(f.backgroundCalls.length, 1);
  assert.deepEqual(f.mutations, []);
});
test("derived card reads revalidate source identity, window, group and host permission", async () => {
  for (const reason of ["number", "window", "group", "permission", "closed"]) {
    const f = fixture(); f.tabs.shift();
    const get = f.browser.tabs.get;
    f.browser.tabs.get = async id => {
      const tab = await get(id);
      if (id === 101) {
        if (reason === "closed") throw Error("gone");
        if (reason === "permission") f.granted.delete(STATUS_ORIGINS[0]);
        if (reason === "number") tab.title = "ATS-124 other";
        if (reason === "window") tab.windowId = 2;
        if (reason === "group") tab.groupId = 20;
      }
      return tab;
    };
    await f.overview.snapshot();
    assert.equal(f.backgroundCalls.length, 0, reason); assert.deepEqual(f.mutations, []);
  }
});
test("derived card navigation reuses an existing card in another window without moving it", async () => {
  const f = fixture(); f.tabs.shift();
  f.tabs.push({id:201,windowId:2,groupId:20,index:1,url:"https://kaiten.x5.ru/space/1/card/123"});
  await f.overview.activateResource(10, "https://kaiten.x5.ru/cards/123");
  assert.deepEqual(f.mutations, [
    {kind:"group",id:20,patch:{collapsed:false}},
    {kind:"tab",id:201,patch:{active:true}},
    {kind:"window",id:2,patch:{focused:true}},
  ]);
  assert.equal(f.groups[0].collapsed,true);
  assert.equal(f.backgroundCalls.length,0);
});
test("explicit card click creates and groups just one tab, even on double click", async () => {
  const f = fixture(); f.tabs.shift();
  f.browser.tabs.create = async properties => {
    f.mutations.push({kind:"create",properties});
    const tab={id:300,groupId:-1,...properties}; f.tabs.push(tab); return {...tab};
  };
  f.browser.tabs.group = async ({tabIds,groupId}) => {
    assert.deepEqual(tabIds,[300]); assert.equal(groupId,10);
    f.tabs.find(t=>t.id===300).groupId=groupId;
  };
  const key="https://kaiten.x5.ru/cards/123";
  await Promise.all([f.overview.activateResource(10,key), f.overview.activateResource(10,key)]);
  assert.equal(f.mutations.filter(m=>m.kind==="create").length,1);
  assert.deepEqual(f.mutations[0].properties,{windowId:1,url:"https://kaiten.x5.ru/space/18571/boards/card/123",active:true});
  assert.equal(f.tabs.find(t=>t.id===300).groupId,10);
  await f.overview.activateResource(10,key);
  assert.equal(f.mutations.filter(m=>m.kind==="create").length,1);
  await assert.rejects(f.overview.activateResource(10,"https://kaiten.x5.ru/cards/999"));
});
test("no chosen window means no site reads and no fallback to another window", async () => {
  const f = fixture(); await f.namer.ready; f.namer.state.windowId = null;
  assert.equal((await f.overview.snapshot()).groups.length, 0);
  assert.equal(f.calls.length, 0); assert.deepEqual(f.mutations, []);
});
test("optional access can be absent or granted for only one site", async () => {
  const f = fixture(); f.granted.clear();
  const view = await f.overview.snapshot();
  assert.equal(f.calls.length, 0);
  assert.ok(view.groups[0].resources.every(r => r.error.code === "permission"));
  f.granted.add(STATUS_ORIGINS[1]);
  const partial = await f.overview.snapshot();
  assert.equal(f.calls.length, 1); assert.equal(partial.groups[0].resources[1].data.state, "opened");
});
test("TTL and manual cooldown bound requests; session cache survives service-worker recreation", async () => {
  const f = fixture(); await f.overview.snapshot(); await f.overview.snapshot(true);
  assert.equal(f.calls.length, 2);
  const restored = new GroupOverview(f.browser, f.namer, () => 1_000_001);
  await restored.snapshot(); assert.equal(f.calls.length, 2);
  f.advance(REFRESH_MS); await f.overview.snapshot(); assert.equal(f.calls.length, 4);
});
test("a failed refresh keeps last success separately from the error and check timestamp", async () => {
  const f = fixture(); const first = await f.overview.snapshot(); f.advance(REFRESH_MS);
  f.browser.scripting.executeScript = async () => [{ result: { error: { code: "auth", message: "Нужен вход" } } }];
  const resource = (await f.overview.snapshot()).groups[0].resources[0];
  assert.equal(resource.data.state, 2); assert.equal(resource.error.code, "auth");
  assert.equal(resource.checkedAt, first.groups[0].resources[0].checkedAt);
  assert.ok(resource.attemptedAt > resource.checkedAt);
});
test("permission removal drops cached data instead of continuing to show private statuses", async () => {
  const f = fixture(); await f.overview.snapshot(); f.granted.clear();
  const view = await f.overview.snapshot();
  assert.ok(view.groups[0].resources.every(r => r.data === null && r.error.code === "permission"));
  assert.deepEqual(f.session.overviewStatusCache, {});
});
test("duplicate resource tabs share a read, distinct repositories never share a result", async () => {
  const f = fixture();
  f.tabs.push({ ...f.tabs[1], id: 102, index: 2 }, { ...f.tabs[1], id: 103, index: 3, url: "https://scm.x5.ru/team/docs/-/merge_requests/42" });
  const resources = (await f.overview.snapshot()).groups[0].resources;
  assert.equal(resources.length, 3); assert.equal(f.calls.length, 3);
  assert.deepEqual(resources[1].tabIds, [101, 102]);
});
test("discarded or navigated tabs are not reloaded and cannot supply another resource's data", async () => {
  const f = fixture(); f.tabs[0].discarded = true;
  const original = f.browser.tabs.get;
  f.browser.tabs.get = async id => id === 101 ? { ...(await original(id)), url: "https://scm.x5.ru/team/other/-/merge_requests/42" } : original(id);
  const view = await f.overview.snapshot();
  assert.equal(f.calls.length, 0); assert.deepEqual(f.mutations, []);
  assert.equal(view.groups[0].resources[0].data.state, 2);
  assert.equal(view.groups[0].resources[1].error.code, "tab");
  assert.equal(f.backgroundCalls.length, 1, "navigated MR must not authorize background fetch");
});
test("discarded Kaiten card is read in background without waking or injecting into its tab", async () => {
  const f = fixture(); f.tabs[0].discarded = true;
  const fallback = [];
  f.overview.readInBackground = async resource => { fallback.push(resource); return { data: { state: 2, column: "Разработка" } }; };
  const view = await f.overview.snapshot();
  assert.equal(view.groups[0].resources[0].data?.column, "Разработка");
  assert.equal(fallback.length, 1);
  assert.equal(fallback[0].id, "99");
  assert.equal(f.calls.length, 1, "only GitLab should use injection");
  assert.deepEqual(f.mutations, []);
});
test("sleeping tabs cannot bypass revoked permissions or a moved/closed resource", async () => {
  for (const reason of ["permission", "moved", "closed"]) {
    const f = fixture(); f.tabs[0].discarded = true;
    const get = f.browser.tabs.get;
    f.browser.tabs.get = async id => {
      if (id !== 100) return get(id);
      if (reason === "closed") throw Error("gone");
      if (reason === "permission") f.granted.clear();
      return { ...await get(id), ...(reason === "moved" ? { windowId: 2 } : {}) };
    };
    await f.overview.snapshot();
    assert.equal(f.backgroundCalls.length, 0, reason);
    assert.deepEqual(f.mutations, []);
  }
});
test("navigation selects the existing Kaiten tab and expands only the requested group", async () => {
  const f = fixture(); await f.overview.activate(10);
  assert.deepEqual(f.mutations, [
    { kind: "group", id: 10, patch: { collapsed: false } },
    { kind: "tab", id: 100, patch: { active: true } },
    { kind: "window", id: 1, patch: { focused: true } },
  ]);
});
test("navigation refuses moved, closed or other-window tabs without changing groups", async () => {
  const f = fixture();
  await assert.rejects(f.overview.activate(20, 200), /не в выбранном/);
  await assert.rejects(f.overview.activate(10, 200), /перемещена/);
  await assert.rejects(f.overview.activate(10, 999), /gone/);
  assert.deepEqual(f.mutations, []);
});
test("overview opening reuses its existing tab rather than creating duplicates", async () => {
  const f = fixture(); await f.overview.open();
  assert.equal(f.mutations[0].kind, "create");
  f.tabs.push({ id: 300, windowId: 2, url: f.browser.runtime.getURL("overview.html") }); f.mutations.length = 0;
  await f.overview.open();
  assert.deepEqual(f.mutations.map(m => m.kind), ["tab", "window"]);
  assert.equal(f.mutations[0].id, 300);
});
test("concurrent dashboard readers coalesce and limit in-flight site requests to three", async () => {
  const f = fixture();
  for (let i = 0; i < 8; i++) f.tabs.push({ ...f.tabs[1], id: 300 + i, index: 2 + i, url: `https://scm.x5.ru/team/app/-/merge_requests/${100 + i}` });
  let active = 0; let maximum = 0; let calls = 0;
  f.browser.scripting.executeScript = async () => {
    active++; maximum = Math.max(maximum, active); calls++;
    await new Promise(resolve => setImmediate(resolve)); active--;
    return [{ result: { data: { state: 2 } } }];
  };
  await Promise.all([f.overview.snapshot(), f.overview.snapshot()]);
  assert.equal(maximum, 3); assert.equal(calls, 10);
});
test("inventory shows groups without site reads, even while status API calls are blocked", async () => {
  const f = fixture(); let release; let entered;
  const started = new Promise(resolve => { entered = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  f.browser.scripting.executeScript = async () => { entered(); await gate; return [{ result: { data: { state: 2 } } }]; };
  const inventory = await f.overview.snapshot(false, false);
  assert.equal(inventory.groups.length, 1);
  assert.ok(inventory.groups[0].resources.every(r => r.error.code === "pending"));
  assert.equal(f.calls.length, 0);
  assert.deepEqual(inventory.windows.map(w => w.groupCount), [1, 1]);
  const loading = f.overview.snapshot(); await started;
  try {
    const quick = await f.overview.snapshot(false, false);
    assert.equal(quick.groups[0].id, 10);
    assert.deepEqual(f.mutations, []);
  } finally { release(); }
  await loading;
  const cached = await f.overview.snapshot(false, false);
  assert.equal(cached.groups[0].resources[0].data.state, 2);
});
test("a suspended renderer cannot leave every resource pending forever", async t => {
  const f = fixture();
  t.mock.timers.enable({ apis: ["setTimeout"] });
  let entered;
  const started = new Promise(resolve => { entered = resolve; });
  f.browser.scripting.executeScript = async () => { entered(); return new Promise(() => {}); };
  let result;
  const reading = f.overview.snapshot().then(view => { result = view; });
  await started;
  t.mock.timers.tick(20_000);
  await new Promise(resolve => setImmediate(resolve));
  assert.ok(result, "an unresponsive tab left the entire overview stuck on Проверяем статус…");
  await reading;
  assert.ok(result.groups[0].resources.every(r => r.error.code === "timeout"));
  assert.equal(f.overview.running, null);
});
test("timed-out injections are reused, and late replies cannot silently overwrite cache", async t => {
  const f = fixture();
  t.mock.timers.enable({ apis: ["setTimeout"] });
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  let calls = 0;
  f.browser.scripting.executeScript = async () => { calls++; await gate; return [{ result: { data: { state: 3 } } }]; };
  for (let attempt = 0; attempt < 2; attempt++) {
    const reading = f.overview.snapshot();
    await new Promise(resolve => setImmediate(resolve));
    t.mock.timers.tick(20_000);
    const view = await reading;
    assert.ok(view.groups[0].resources.every(r => r.error.code === "timeout"));
    f.advance(REFRESH_MS);
  }
  assert.equal(calls, 2, "retry must not inject again into a still-paused tab");
  release(); await new Promise(resolve => setImmediate(resolve));
  const inventory = await f.overview.snapshot(false, false);
  assert.ok(inventory.groups[0].resources.every(r => r.error.code === "timeout" && !r.data));
  const retried = await f.overview.snapshot();
  assert.ok(retried.groups[0].resources.every(r => r.data.state === 3));
  assert.equal(calls, 4);
});
test("an injection failing after its deadline cannot start a late background request", async t => {
  const f = fixture();
  let reject;
  f.browser.scripting.executeScript = () => new Promise((_, fail) => { reject = fail; });
  t.mock.timers.enable({ apis: ["setTimeout"] });
  const resource = { kind: "kaiten", origin: "https://kaiten.x5.ru", key: "https://kaiten.x5.ru/cards/99", id: "99", tabIds: [100] };
  const result = f.overview.readResource(resource, 1, new Set([10]));
  await Promise.resolve();
  t.mock.timers.tick(20_000);
  assert.equal((await result).error.code, "timeout");
  reject(Error("renderer finally failed"));
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(f.backgroundCalls.length, 0);
});
test("a frozen tab is read through the worker without injecting into a paused renderer", async () => {
  const f = fixture(); f.tabs[0].frozen = true;
  const view = await f.overview.snapshot();
  assert.equal(view.groups[0].resources[0].data.state, 2);
  assert.equal(f.backgroundCalls.length, 1);
  assert.equal(view.groups[0].resources[1].data.state, "opened");
  assert.equal(f.calls.length, 1);
  assert.deepEqual(f.mutations, []);
});
test("changing selected window during a site read cannot publish old results", async () => {
  const f = fixture(); let release; let entered;
  const started = new Promise(resolve => { entered = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  f.browser.scripting.executeScript = async () => { entered(); await gate; return [{ result: { data: { state: 3 } } }]; };
  const running = f.overview.snapshot();
  await started; f.namer.generation++; f.namer.state.windowId = 2; release();
  await assert.rejects(running, /окно изменилось/);
  assert.equal(f.session.overviewStatusCache, undefined);
});
test("revocation while requests are in flight drops their results", async () => {
  const f = fixture(); let release; let entered;
  const started = new Promise(resolve => { entered = resolve; });
  const gate = new Promise(resolve => { release = resolve; });
  f.browser.scripting.executeScript = async () => { entered(); await gate; return [{ result: { data: { state: 3 } } }]; };
  const running = f.overview.snapshot(); await started; f.granted.clear(); release();
  const view = await running;
  assert.ok(view.groups[0].resources.every(r => r.data === null));
  assert.deepEqual(f.session.overviewStatusCache, {});
});
