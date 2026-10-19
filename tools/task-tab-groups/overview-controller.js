import { STATUS_ORIGINS, REFRESH_MS, resourceForTab, resourcesForTabs } from "./overview-model.js";
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
    this.ready = browser.storage.session.get(CACHE_KEY).then(saved => { this.cache = saved[CACHE_KEY] || {}; });
  }

  async permissions() {
    const entries = await Promise.all(STATUS_ORIGINS.map(async origin => [origin.slice(0, -2), await this.browser.permissions.contains({ permissions: ["scripting"], origins: [origin] })]));
    return Object.fromEntries(entries);
  }

  async snapshot(force = false) {
    // Multiple overview tabs share one bounded refresh, including a manual refresh cooldown.
    if (this.running) return this.running;
    const operation = this.collect(force);
    this.running = operation;
    try { return await operation; }
    finally { if (this.running === operation) this.running = null; }
  }

  async collect(force) {
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
      if (previous) previous.tabIds.push(...resource.tabIds);
      else unique.set(resource.key, { ...resource, tabIds: [...resource.tabIds] });
    }
    // Session-only cache, bounded to resources of the chosen window; no descriptions or tokens.
    for (const key of Object.keys(this.cache)) {
      const resource = unique.get(key);
      if (!resource || !granted[resource.origin]) delete this.cache[key];
    }
    const pending = [...unique.values()].filter(resource => granted[resource.origin]);
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
        ? this.cache[resource.key] || { error: error("unavailable", "Статус ещё не получен.") }
        : { error: error("permission", "Чтение статусов отключено."), data: null }) })),
    })) };
  }

  async readResource(resource, windowId, groupIds) {
    let lastError = error("tab", "Вкладка закрыта или выгружена. Откройте её и обновите обзор.");
    for (const tabId of resource.tabIds) {
      try {
        const tab = await this.browser.tabs.get(tabId);
        if (tab.windowId !== windowId || !groupIds.has(tab.groupId) || tab.discarded || tab.status === "loading" || resourceForTab(tab)?.key !== resource.key) continue;
        const results = await this.browser.scripting.executeScript({ target: { tabId }, func: readStatusInTab, args: [resource] });
        const result = results[0]?.result;
        if (result?.data || result?.error) return result;
      } catch {
        lastError = error("tab", "Нет доступа к вкладке. Проверьте разрешения расширения и откройте сайт.");
      }
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
