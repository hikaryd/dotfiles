import { inferGroupName, lockedTitle } from "./naming.js";

export class GroupNamer {
  constructor(browser) {
    this.browser = browser;
    this.generation = 0;
    this.running = null;
    this.state = { windowId: null, auto: true, groups: [], error: null };
    this.ready = this.restore();
  }

  async restore() {
    const [session, local] = await Promise.all([
      this.browser.storage.session.get(["windowId"]),
      this.browser.storage.local.get(["auto"]),
    ]);
    this.state.windowId = session.windowId ?? null;
    this.state.auto = local.auto ?? true;
  }

  async selectWindow(windowId) {
    await this.ready;
    const window = await this.browser.windows.get(windowId);
    if (window.type !== "normal") throw new Error("Выберите обычное окно браузера.");
    this.generation++;
    this.state.windowId = windowId;
    this.state.groups = [];
    this.state.error = null;
    await this.browser.storage.session.set({ windowId });
    return this.refresh(true);
  }

  async setAuto(auto) {
    await this.ready;
    this.state.auto = Boolean(auto);
    await this.browser.storage.local.set({ auto: this.state.auto });
    return this.refresh(this.state.auto);
  }

  async setLock(groupId, title, locked) {
    await this.ready;
    if (this.state.windowId == null) throw new Error("Сначала выберите окно.");
    const group = await this.browser.tabGroups.get(groupId);
    if (group.windowId !== this.state.windowId) throw new Error("Группа не в выбранном окне.");
    if (typeof title !== "string" || !title.trim()) throw new Error("Имя группы не должно быть пустым.");
    // The native marker persists across browser restarts and changing group IDs.
    await this.browser.tabGroups.update(groupId, { title: lockedTitle(title.trim(), locked) });
    return this.refresh(false);
  }

  async refresh(apply = false) {
    await this.ready;
    // Serialise refreshes; the generation guard prevents stale writes after scope changes.
    const previous = this.running;
    const operation = (async () => {
      if (previous) await previous.catch(() => {});
      return this.performRefresh(apply);
    })();
    this.running = operation;
    try { return await operation; }
    finally { if (this.running === operation) this.running = null; }
  }

  async performRefresh(apply) {
    const windowId = this.state.windowId;
    const generation = this.generation;
    if (windowId == null) return this.getView();
    try {
      await this.browser.windows.get(windowId);
    } catch {
      if (generation === this.generation) {
        this.state.windowId = null;
        this.state.groups = [];
        this.state.error = "Выбранное окно закрыто. Выберите окно заново.";
        await this.browser.storage.session.remove("windowId");
      }
      return this.getView();
    }
    const groups = await this.browser.tabGroups.query({ windowId });
    const rows = [];
    for (const group of groups) {
      if (generation !== this.generation) break;
      try {
        const tabs = await this.browser.tabs.query({ groupId: group.id });
        if (!tabs.length || tabs.some(tab => tab.windowId !== windowId)) continue;
        const fresh = await this.browser.tabGroups.get(group.id);
        if (fresh.windowId !== windowId) continue;
        const decision = inferGroupName({ title: fresh.title, tabs });
        let title = fresh.title;
        if (apply && (["ready", "fallback"].includes(decision.state) || decision.repair) && decision.suggested && title !== decision.suggested && generation === this.generation) {
          // Update only the title: never move/reload tabs, recolour or expand groups.
          await this.browser.tabGroups.update(group.id, { title: decision.suggested });
          title = decision.suggested;
        }
        rows.push({ id: group.id, title, color: fresh.color, count: tabs.length, ...decision });
      } catch {
        // A tab/group may disappear while the user is working. Retry on the next event.
      }
    }
    if (generation === this.generation) {
      this.state.groups = rows;
      this.state.error = null;
    }
    return this.getView();
  }

  async getView() {
    await this.ready;
    const windows = await this.browser.windows.getAll({ populate: true, windowTypes: ["normal"] });
    const choices = windows.filter(window => !window.tabs?.every(tab => tab.url?.startsWith(this.browser.runtime.getURL(""))))
      .map(window => ({ id: window.id, count: window.tabs?.length ?? 0, focused: window.focused }));
    return { ...this.state, windows: choices };
  }

  async forgetWindow(windowId) {
    await this.ready;
    if (windowId !== this.state.windowId) return;
    this.generation++;
    this.state.windowId = null;
    this.state.groups = [];
    await this.browser.storage.session.remove("windowId");
  }

  async dispatch(message) {
    switch (message.type) {
      case "view": return this.refresh(false);
      case "select": return this.selectWindow(message.windowId);
      case "refresh": return this.refresh(true);
      case "auto": return this.setAuto(message.enabled);
      case "lock": return this.setLock(message.groupId, message.title, message.locked);
      default: throw new Error("Неизвестная команда.");
    }
  }

  async updateAutomatically() {
    await this.ready;
    if (this.state.auto) return this.refresh(true);
  }
}
