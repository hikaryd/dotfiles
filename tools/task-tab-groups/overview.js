import { STATUS_ORIGINS, REFRESH_MS, GROUP_COLORS, groupCategory, matchesSearch, isStale } from "./overview-model.js";

const $ = id => document.getElementById(id);
const filterNames = { all: "Все", attention: "Требуют внимания", active: "Не завершены", done: "Готово", unknown: "Нет полных данных" };
const pipelineNames = { success: "✓ CI пройден", failed: "✕ CI упал", canceled: "CI отменён", running: "◷ CI выполняется", pending: "CI в очереди", created: "CI создан", preparing: "CI готовится", waiting_for_resource: "CI ждёт ресурс", manual: "CI: ручной шаг", scheduled: "CI запланирован", skipped: "CI пропущен", none: "Нет pipeline" };
let view = null;
let filter = "all";
let busy = false;
let pending = false;

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}
function badge(text, kind = "") { return element("span", "badge " + kind, text); }
function time(timestamp) { return new Date(timestamp).toLocaleTimeString("ru", { hour: "2-digit", minute: "2-digit", second: "2-digit" }); }
function showError(message) { $("error").textContent = message; $("error").hidden = !message; }
async function call(message) {
  const response = await chrome.runtime.sendMessage(message);
  if (!response?.ok) throw new Error(response?.error || "Расширение не ответило. Попробуйте обновить страницу.");
  return response.view;
}
async function activate(groupId, tabId) {
  try { await call({ type: "overview-activate", groupId, ...(tabId === undefined ? {} : { tabId }) }); showError(""); }
  catch (error) { showError(error.message); }
}
function openButton(label, groupId, tabId, className, key) {
  const button = element("button", className, label);
  button.dataset.focusKey = key;
  button.addEventListener("click", () => activate(groupId, tabId));
  return button;
}
function renderResource(resource, groupId) {
  const block = element("div", "resource");
  const label = resource.kind === "kaiten" ? `Карточка #${resource.id} ↗` : `${resource.project} !${resource.id} ↗`;
  block.append(openButton(label, groupId, resource.tabIds[0], "resource-link", `${groupId}:${resource.key}`));
  const data = resource.data;
  if (data) {
    const badges = element("div", "badges");
    if (resource.kind === "kaiten") {
      badges.append(badge(data.column || { 1: "В очереди", 2: "В работе", 3: "Готово" }[data.state], data.state === 3 ? "good" : ""));
      if (data.blocked) badges.append(badge("Заблокирована", "bad"));
      if (data.archived) badges.append(badge("В архиве", "neutral"));
      if (data.blocked === null) badges.append(badge("Блокировка неизвестна", "neutral"));
      block.append(badges);
      if (data.board) block.append(element("p", "hint", data.board));
      if (data.blocked && data.blockReason) block.append(element("p", "hint", data.blockReason));
      block.append(element("p", "hint", data.assignees.length ? "Ответственный: " + data.assignees.join(", ") : "Ответственный не указан"));
      if (data.dueDate) block.append(element("p", "hint", "Срок: " + data.dueDate.slice(0, 10)));
    } else {
      const state = { opened: data.draft ? "Draft" : "Открыт", merged: "Merged", closed: "Закрыт без merge", locked: "Заблокирован" }[data.state];
      badges.append(badge(state, data.state === "merged" ? "good" : data.draft ? "neutral" : ""));
      if (data.hasConflicts) badges.append(badge("Конфликты", "bad"));
      if (data.discussionsResolved === false) badges.append(badge("Есть обсуждения", "warn"));
      if (data.mergeStatus === "not_approved") badges.append(badge("Нужны одобрения", "warn"));
      badges.append(badge(pipelineNames[data.pipeline] || (data.pipeline ? `CI: ${data.pipeline}` : "CI неизвестен"), data.pipeline === "success" ? "good" : ["failed", "canceled"].includes(data.pipeline) ? "bad" : "neutral"));
      block.append(badges);
      if (data.title) block.append(element("p", "hint", data.title));
      if (data.reviewers.length) block.append(element("p", "hint", "Ревью: " + data.reviewers.join(", ")));
    }
    block.append(element("p", "hint" + (isStale(resource) ? " stale" : ""), (isStale(resource) ? "Последние известные данные · " : "Проверено · ") + time(resource.checkedAt)));
  }
  if (resource.error) block.append(element("p", resource.error.code === "permission" ? "hint" : "resource-error", resource.error.message));
  return block;
}
function renderGroups() {
  if (!view) return;
  const focused = document.activeElement?.dataset.focusKey;
  const opened = new Set([...document.querySelectorAll(".tabs-list[open]")].map(node => node.dataset.groupId));
  const matching = view.groups.filter(group => matchesSearch(group, $("search").value));
  const counts = Object.fromEntries(Object.keys(filterNames).map(key => [key, key === "all" ? matching.length : matching.filter(g => groupCategory(g) === key).length]));
  for (const button of $("filters").children) {
    button.textContent = `${filterNames[button.dataset.filter]} · ${counts[button.dataset.filter]}`;
    button.setAttribute("aria-pressed", String(button.dataset.filter === filter));
  }
  const groups = matching.filter(group => filter === "all" || groupCategory(group) === filter);
  $("groups").replaceChildren(...groups.map(group => {
    const row = element("article", "task");
    row.tabIndex = 0; row.dataset.groupId = group.id; row.dataset.focusKey = `group:${group.id}`;
    row.setAttribute("aria-label", group.title || "Без названия");
    row.style.setProperty("--group-color", GROUP_COLORS[group.color] || GROUP_COLORS.grey);
    const info = element("div");
    info.append(element("span", "task-key", group.keys?.join(" / ") || "ГРУППА ВКЛАДОК"));
    info.append(openButton(group.title || "Без названия", group.id, undefined, "task-heading", `heading:${group.id}`));
    if (group.state === "conflict") info.append(element("p", "resource-error", "В группе разные задачи — показаны все найденные карточки и MR."));
    const details = element("details", "tabs-list"); details.dataset.groupId = group.id; details.open = opened.has(String(group.id));
    const summary = element("summary", "", `${group.tabs.length} вкладок в группе`); summary.dataset.focusKey = `tabs:${group.id}`;
    details.append(summary);
    for (const tab of group.tabs) details.append(openButton(tab.title + (tab.discarded ? " · выгружена" : ""), group.id, tab.id, "", `tab:${tab.id}`));
    info.append(details); row.append(info);
    for (const kind of ["kaiten", "gitlab"]) {
      const column = element("div");
      const resources = group.resources.filter(resource => resource.kind === kind);
      if (!resources.length) column.append(element("p", "hint", kind === "kaiten" ? "Нет вкладки с карточкой Kaiten" : "Нет вкладок с MR"));
      for (const resource of resources) column.append(renderResource(resource, group.id));
      row.append(column);
    }
    return row;
  }));
  if (focused) [...document.querySelectorAll("[data-focus-key]")].find(node => node.dataset.focusKey === focused)?.focus({ preventScroll: true });
  $("empty").hidden = groups.length > 0;
  $("empty").textContent = view.windowId == null ? "Выберите окно с задачами. До выбора группы не изменяются." : !view.groups.length ? "В выбранном окне нет групп вкладок." : "Нет задач, подходящих под поиск и фильтр.";
  $("status").textContent = busy ? "Обновляем статусы…" : `Показано ${groups.length} из ${view.groups.length} групп`;
  $("freshness").textContent = view.scannedAt ? `Обзор обновлён в ${time(view.scannedAt)}` : "";
}
function render() {
  const previous = Number($("window").value);
  $("window").replaceChildren(...view.windows.map(window => {
    const option = element("option", "", `Окно ${window.id} · ${window.count} вкладок${window.focused ? " · активное" : ""}`);
    option.value = window.id; return option;
  }));
  $("window").value = view.windowId ?? (view.windows.some(w => w.id === previous) ? previous : view.windows.find(w => w.focused)?.id ?? view.windows[0]?.id) ?? "";
  const granted = Object.values(view.permissions).filter(Boolean).length;
  $("enable").hidden = granted === STATUS_ORIGINS.length;
  $("disable").hidden = granted === 0;
  $("access-title").textContent = granted ? `Чтение статусов включено · ${granted} из 2 сайтов` : "Статусы подключаются отдельно";
  $("access-description").textContent = granted ? "Обновление только пока обзор виден. Используются существующие вкладки и ваш вход на сайт; выгруженные вкладки не перезагружаются." : "Только чтение Kaiten и GitLab через ваш вход в открытых вкладках. Без токенов, изменений карточек и отдельного сервера.";
  renderGroups();
}
async function refresh(force = false) {
  if (document.hidden) return;
  if (busy) { pending = true; return; }
  busy = true; $("refresh").disabled = true; $("status").textContent = "Обновляем статусы…";
  try { view = await call({ type: "overview", force }); showError(view.error || ""); render(); }
  catch (error) { showError(error.message); }
  finally {
    busy = false; $("refresh").disabled = false;
    if (view) renderGroups(); else $("status").textContent = "Обзор пока недоступен.";
    if (pending) { pending = false; scheduleRefresh(); }
  }
}
$("refresh").addEventListener("click", () => refresh(true));
$("select").addEventListener("click", async () => {
  if (!$("window").value) return;
  $("select").disabled = true;
  try { await call({ type: "select", windowId: Number($("window").value) }); await refresh(); }
  catch (error) { showError(error.message); }
  finally { $("select").disabled = false; }
});
$("enable").addEventListener("click", () => {
  // Call request directly from the gesture, before any await.
  chrome.permissions.request({ permissions: ["scripting"], origins: STATUS_ORIGINS }).then(granted => {
    if (granted) refresh(); else showError("Разрешение не выдано. Список групп и переходы продолжают работать.");
  }).catch(error => showError(error.message));
});
$("disable").addEventListener("click", async () => {
  try { await chrome.permissions.remove({ permissions: ["scripting"], origins: STATUS_ORIGINS }); await refresh(); }
  catch (error) { showError(error.message); }
});
$("search").addEventListener("input", renderGroups);
$("filters").addEventListener("click", event => {
  const button = event.target.closest("button[data-filter]");
  if (button) { filter = button.dataset.filter; renderGroups(); }
});
$("auto").addEventListener("change", () => chrome.storage.local.set({ overviewAuto: $("auto").checked }).catch(error => showError(error.message)));
chrome.storage.local.get("overviewAuto").then(saved => { $("auto").checked = saved.overviewAuto ?? true; }).catch(() => {});
document.addEventListener("keydown", event => {
  if (event.ctrlKey || event.metaKey || event.altKey || event.isComposing) return;
  const editing = event.target.matches("input, select, textarea") || event.target.isContentEditable;
  if (event.key === "Escape" && event.target === $("search")) { $("search").value = ""; renderGroups(); $("search").blur(); return; }
  if (editing) return;
  if (event.key === "/") { event.preventDefault(); $("search").focus(); return; }
  const rows = [...document.querySelectorAll(".task")];
  const row = event.target.closest(".task");
  if (["ArrowDown", "ArrowUp", "j", "k"].includes(event.key) && !event.target.matches("button, summary")) {
    event.preventDefault();
    const delta = ["ArrowDown", "j"].includes(event.key) ? 1 : -1;
    const index = row ? rows.indexOf(row) + delta : delta === 1 ? 0 : rows.length - 1;
    rows[Math.max(0, Math.min(rows.length - 1, index))]?.focus();
  }
  if (event.key === "Enter" && event.target === row) { event.preventDefault(); activate(Number(row.dataset.groupId)); }
});
let debounce;
function scheduleRefresh() {
  clearTimeout(debounce);
  if (!document.hidden) debounce = setTimeout(() => refresh(), 400);
}
for (const name of ["onCreated", "onUpdated", "onRemoved", "onMoved", "onAttached", "onDetached"]) chrome.tabs[name].addListener(scheduleRefresh);
for (const name of ["onCreated", "onUpdated", "onRemoved"]) chrome.tabGroups[name].addListener(scheduleRefresh);
chrome.permissions.onAdded.addListener(scheduleRefresh);
chrome.permissions.onRemoved.addListener(scheduleRefresh);
chrome.windows.onRemoved.addListener(scheduleRefresh);
document.addEventListener("visibilitychange", () => { if (!document.hidden) refresh(); });
setInterval(() => { if (!document.hidden && $("auto").checked) refresh(); }, REFRESH_MS);
// Age labels/filters even with automatic site requests disabled. This does not fetch.
setInterval(() => { if (!document.hidden && view) renderGroups(); }, 15_000);
refresh();
