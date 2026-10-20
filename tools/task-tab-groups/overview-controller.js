import { STATUS_ORIGINS, REFRESH_MS, resourceForTab, resourcesForTabs, kaitenCardUrl } from "./overview-model.js";
import { readStatusInTab } from "./status-reader.js";

const CACHE_KEY = "overviewStatusCache";
const error = (code, message) => ({ code, message });

export class GroupOverview {
  constructor(browser, namer, now = Date.now) {
    this.browser = browser;
    this.namer = namer;
    this.now = now;
    this.cache = {};
    this.running = null;
    this.tabReads = new Map();
    this.resourceOpens = new Map();
    this.readInBackground = (resource, signal) => readStatusInTab(resource, true, signal);
    this.ready = browser.storage.session.get(CACHE_KEY).then(saved => { this.cache = saved[CACHE_KEY] || {}; });
  }

  async permissions() {
    const entries = await Promise.all(STATUS_ORIGINS.map(async origin => [origin.slice(0, -2), await this.browser.permissions.contains({ permissions: ["scripting"], origins: [origin] })]));
    return Object.fromEntries(entries);
  }

  async snapshot(force = false, readStatuses = true) {
    // Inventory never waits behind slow site APIs; publish groups before their statuses.
    if (!readStatuses) return this.collect(false, false);
    // Multiple overview tabs share one bounded refresh, including a manual refresh cooldown.
    if (this.running) return this.running;
    const operation = this.collect(force);
    this.running = operation;
    try { return await operation; }
    finally { if (this.running === operation) this.running = null; }
  }

  async collect(force, readStatuses = true) {
    await this.ready;
    const view = await this.namer.refresh(false);
    const generation = this.namer.generation;
    const granted = await this.permissions();
    const tabs = view.windowId == null ? [] : await this.browser.tabs.query({ windowId: view.windowId });
    const groups = view.groups.map(group => {
      const members = tabs.filter(tab => tab.groupId === group.id).sort((a, b) => a.index - b.index);
      return { ...group, index: members[0]?.index ?? Infinity,
        tabs: members.map(tab => ({ id: tab.id, title: tab.title || "Без названия", discarded: Boolean(tab.discarded) })),
        resources: resourcesForTabs(members),
      };
    }).filter(group => group.tabs.length).sort((a, b) => a.index - b.index);
    const unique = new Map();
    for (const group of groups) for (const resource of group.resources) {
      const previous = unique.get(resource.key);
      if (previous) {
        previous.tabIds.push(...resource.tabIds);
        previous.sourceTabIds.push(...(resource.sourceTabIds || []));
      } else unique.set(resource.key, { ...resource, tabIds: [...resource.tabIds], sourceTabIds: [...(resource.sourceTabIds || [])] });
    }
    // Session-only cache, bounded to resources of the chosen window; no descriptions or tokens.
    for (const key of Object.keys(this.cache)) {
      const resource = unique.get(key);
      if (!resource || !granted[resource.origin]) delete this.cache[key];
    }
    const pending = readStatuses ? [...unique.values()].filter(resource => granted[resource.origin]) : [];
    let cursor = 0;
    const consume = async () => {
      while (cursor < pending.length) {
        const resource = pending[cursor++];
        if (generation !== this.namer.generation) return;
        const cached = this.cache[resource.key];
        if (cached && this.now() - cached.attemptedAt < (force ? 5_000 : REFRESH_MS)) continue;
        const result = await this.readResource(resource, view.windowId, new Set(groups.map(g => g.id)));
        if (generation !== this.namer.generation) return;
        const checkedAt = this.now();
        this.cache[resource.key] = result.data
          ? { data: result.data, checkedAt, attemptedAt: checkedAt, error: null }
          : { data: cached?.data ?? null, checkedAt: cached?.checkedAt ?? null, attemptedAt: checkedAt, error: result.error };
      }
    };
    await Promise.all(Array.from({ length: Math.min(3, pending.length) }, consume));
    // Do not show or persist an old scope's response, or data after a permission revocation.
    if (generation !== this.namer.generation) throw new Error("Выбранное окно изменилось. Обновите обзор.");
    const currentPermissions = await this.permissions();
    for (const [key, resource] of unique) if (!currentPermissions[resource.origin]) delete this.cache[key];
    await this.browser.storage.session.set({ [CACHE_KEY]: this.cache });
    return { ...view, permissions: currentPermissions, scannedAt: this.now(), groups: groups.map(group => ({ ...group,
      resources: group.resources.map(resource => ({ ...resource, ...(currentPermissions[resource.origin]
        ? this.cache[resource.key] || { error: error("pending", "Проверяем статус…") }
        : { error: error("permission", "Чтение статусов отключено."), data: null }) })),
    })) };
  }

  async readResource(resource, windowId, groupIds) {
    // Timers inside a suspended renderer do not run. Bound the WHOLE read here,
    // including injection, JSON parsing, and the optional background transport.
    let timer;
    const controller = new AbortController();
    try {
      return await Promise.race([
        this.readResourceFromTabs(resource, windowId, groupIds, controller.signal),
        new Promise(resolve => { timer = setTimeout(() => resolve({ error: error("timeout", "Вкладка не ответила за 20 секунд. Откройте её и повторите чтение.") }), 20_000); }),
      ]);
    } finally { clearTimeout(timer); controller.abort(); }
  }

  async readResourceFromTabs(resource, windowId, groupIds, signal) {
    const generation = this.namer.generation;
    let canReadInBackground = false;
    let lastError = error("tab", "Вкладка закрыта или выгружена. Откройте её и обновите обзор.");
    for (const tabId of resource.tabIds) {
      if (signal?.aborted) break;
      try {
        const tab = await this.browser.tabs.get(tabId);
        if (tab.windowId !== windowId || !groupIds.has(tab.groupId) || resourceForTab(tab)?.key !== resource.key) continue;
        canReadInBackground = true;
        if (tab.discarded || tab.status === "loading") continue;
        if (tab.frozen) {
          lastError = error("frozen", "Вкладка заморожена браузером. Откройте её, затем обновите статусы.");
          continue;
        }
        // A deadline cannot cancel executeScript. Reuse an outstanding injection
        // instead of queuing another one on every refresh in the same stuck tab.
        let read = this.tabReads.get(tabId);
        if (read && read.key !== resource.key) continue;
        if (signal?.aborted) break;
        if (!read) {
          const promise = this.browser.scripting.executeScript({ target: { tabId }, func: readStatusInTab, args: [resource] });
          read = { key: resource.key, promise };
          this.tabReads.set(tabId, read);
          const clear = () => { if (this.tabReads.get(tabId) === read) this.tabReads.delete(tabId); };
          promise.then(clear, clear);
        }
        const results = await read.promise;
        const result = results[0]?.result;
        if (result?.data || result?.error) return result;
      } catch {
        lastError = error("tab", "Нет доступа к вкладке. Проверьте разрешения расширения и откройте сайт.");
      }
    }
    // MR-derived cards have no Kaiten tab. Revalidate the task number in the
    // current source tab, its group and window before authorizing any request.
    for (const tabId of resource.sourceTabIds || []) {
      if (signal?.aborted) break;
      try {
        const tab = await this.browser.tabs.get(tabId);
        if (tab.windowId === windowId && groupIds.has(tab.groupId) && resourcesForTabs([tab]).some(r => r.key === resource.key && r.sourceTabIds?.includes(tabId))) canReadInBackground = true;
      } catch { /* A closed source cannot authorize a derived card read. */ }
    }
    // A sleeping tab still provides the exact resource identity. Use the browser's
    // existing session without opening/reloading that tab, or exposing cookies.
    if (canReadInBackground && !signal?.aborted && generation === this.namer.generation) {
      const granted = await this.browser.permissions.contains({ permissions: ["scripting"], origins: [resource.origin + "/*"] });
      if (!granted) return { error: error("permission", "Чтение статусов отключено.") };
      if (!signal?.aborted && generation === this.namer.generation) return this.readInBackground(resource, signal);
    }
    return { error: lastError };
  }

  async activate(groupId, tabId) {
    await this.namer.ready;
    const windowId = this.namer.state.windowId;
    if (windowId == null) throw new Error("Сначала выберите окно с задачами.");
    const generation = this.namer.generation;
    const group = await this.browser.tabGroups.get(groupId);
    if (group.windowId !== windowId) throw new Error("Группа уже не в выбранном окне. Обновите обзор.");
    let tab;
    if (tabId !== undefined) tab = await this.browser.tabs.get(tabId);
    else {
      const tabs = (await this.browser.tabs.query({ groupId })).sort((a, b) => a.index - b.index);
      tab = tabs.find(t => resourceForTab(t)?.kind === "kaiten") || tabs.find(t => resourceForTab(t)?.kind === "gitlab") || tabs[0];
    }
    if (!tab || tab.windowId !== windowId || tab.groupId !== groupId || generation !== this.namer.generation) throw new Error("Вкладка перемещена или закрыта. Обновите обзор.");
    // Expanding and focusing are only allowed after an explicit user navigation action.
    if (group.collapsed) await this.browser.tabGroups.update(groupId, { collapsed: false });
    await this.browser.tabs.update(tab.id, { active: true });
    await this.browser.windows.update(windowId, { focused: true });
  }

  async activateResource(groupId, resourceKey) {
    // Coalesce double clicks so a missing card produces exactly one new tab.
    if (this.resourceOpens.has(resourceKey)) return this.resourceOpens.get(resourceKey);
    const operation = this.openResource(groupId, resourceKey);
    this.resourceOpens.set(resourceKey, operation);
    try { return await operation; }
    finally { if (this.resourceOpens.get(resourceKey) === operation) this.resourceOpens.delete(resourceKey); }
  }

  async openResource(groupId, resourceKey) {
    const view = await this.snapshot(false, false);
    const generation = this.namer.generation;
    const resource = view.groups.find(g => g.id === groupId)?.resources.find(r => r.key === resourceKey);
    if (!resource) throw new Error("Карточка больше не связана с этой группой. Обновите обзор.");
    if (resource.tabIds.length) return this.activate(groupId, resource.tabIds[0]);
    if (resource.kind !== "kaiten" || !resource.sourceTabIds?.length) throw new Error("Нет адреса карточки.");
    const tabs = await this.browser.tabs.query({});
    const existing = tabs.filter(t => resourceForTab(t)?.key === resource.key)
      .sort((a, b) => Number(b.windowId === view.windowId) - Number(a.windowId === view.windowId))[0];
    if (generation !== this.namer.generation) throw new Error("Выбранное окно изменилось. Обновите обзор.");
    if (existing) {
      const tab = await this.browser.tabs.get(existing.id);
      if (resourceForTab(tab)?.key !== resource.key) throw new Error("Вкладка перешла на другую страницу.");
      if (tab.groupId >= 0) {
        const group = await this.browser.tabGroups.get(tab.groupId);
        if (group.collapsed) await this.browser.tabGroups.update(group.id, { collapsed: false });
      }
      await this.browser.tabs.update(tab.id, { active: true });
      await this.browser.windows.update(tab.windowId, { focused: true });
    } else {
      const tab = await this.browser.tabs.create({ windowId: view.windowId, url: kaitenCardUrl(resource.id), active: true });
      // Only a newly created card tab is grouped, and only after an explicit click.
      await this.browser.tabs.group({ groupId, tabIds: [tab.id] });
      const group = await this.browser.tabGroups.get(groupId);
      if (group.collapsed) await this.browser.tabGroups.update(groupId, { collapsed: false });
      await this.browser.windows.update(view.windowId, { focused: true });
    }
  }

  async open() {
    await this.namer.ready;
    const url = this.browser.runtime.getURL("overview.html");
    const existing = (await this.browser.tabs.query({ url }))[0];
    if (existing) {
      await this.browser.tabs.update(existing.id, { active: true });
      await this.browser.windows.update(existing.windowId, { focused: true });
    } else {
      const windowId = this.namer.state.windowId;
      await this.browser.tabs.create({ url, ...(windowId == null ? {} : { windowId }) });
    }
  }
}
