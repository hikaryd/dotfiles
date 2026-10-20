import { STATUS_ORIGINS, REFRESH_MS, GROUP_COLORS, groupCategory, matchesSearch, isStale, summarizeGroup, statusDiagnostics } from "./overview-model.js";

const $ = id => document.getElementById(id);
const filterNames = { all: "Все", attention: "Требуют внимания", active: "В работе", done: "Готово", unknown: "Нет полных данных" };
const pipelineNames = { success: "✓ CI пройден", failed: "✕ CI упал", canceled: "CI отменён", running: "◷ CI выполняется", pending: "CI в очереди", created: "CI создан", preparing: "CI готовится", waiting_for_resource: "CI ждёт ресурс", manual: "CI: ручной шаг", scheduled: "CI запланирован", skipped: "CI пропущен", none: "Нет pipeline" };
let view = null;
let filter = "all";
let busy = false;
let pending = false;
let pendingForce = false;
let pendingStatuses = false;
let revision = 0;
let selecting = false;
let changingWindowId = null;
let renderedScope;

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
async function activateResource(groupId, resourceKey) {
  try { await call({ type: "overview-activate-resource", groupId, resourceKey }); showError(""); }
  catch (error) { showError(error.message); }
}
function openButton(label, groupId, tabId, className, key, resourceKey) {
  const button = element("button", className, label);
  button.dataset.focusKey = key;
  button.addEventListener("click", () => resourceKey ? activateResource(groupId, resourceKey) : activate(groupId, tabId));
  return button;
}
function renderResource(resource, groupId) {
  const block = element("div", "resource");
  const label = resource.kind === "kaiten" ? `Карточка #${resource.id} ↗` : `${resource.project} !${resource.id} ↗`;
  block.append(openButton(label, groupId, resource.tabIds[0], "resource-link", `${groupId}:${resource.key}`, resource.key));
  if (resource.sourceTabIds?.length && !resource.tabIds.length) block.append(element("p", "hint", "Найдена по номеру задачи в MR"));
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
      const review = summarizeGroup({ resources: [resource] }).review;
      badges.append(badge(review.label, review.tone));
      badges.append(badge(pipelineNames[data.pipeline] || (data.pipeline ? `CI: ${data.pipeline}` : "CI неизвестен"), data.pipeline === "success" ? "good" : ["failed", "canceled"].includes(data.pipeline) ? "bad" : "neutral"));
      block.append(badges);
      if (data.title) block.append(element("p", "hint", data.title));
      if (data.review?.approvedBy?.length) block.append(element("p", "hint", "Одобрили: " + data.review.approvedBy.join(", ")));
      if (data.review?.remaining > 0) block.append(element("p", "hint", `Нужно ещё одобрений: ${data.review.remaining}`));
      if (data.review?.excludedCount > 0) block.append(element("p", "hint", "Одобрение @SRV.SAMEDI не учитывается."));
      if (data.review?.status === "not-required") block.append(element("p", "hint", "Нет учитываемых одобрений." + (data.review.required === 0 ? " Обязательные одобрения не настроены." : "")));
      if (data.review?.error) block.append(element("p", "resource-error", "Ревью: " + data.review.error.message));
      if (!data.review) block.append(element("p", "hint", "Обновите статусы, чтобы проверить одобрения."));
      if (data.reviewers.length) block.append(element("p", "hint", "Назначены на ревью: " + data.reviewers.join(", ")));
    }
    block.append(element("p", "hint" + (isStale(resource) ? " stale" : ""), (isStale(resource) ? "Последние известные данные · " : "Проверено · ") + time(resource.checkedAt)));
  }
  if (resource.error) block.append(element("p", ["permission", "pending"].includes(resource.error.code) ? "hint" : "resource-error", resource.error.code === "pending" && !busy ? "Нажмите «Обновить», чтобы проверить статус." : resource.error.message));
  return block;
}
function summaryButton(summary, className, label, key, action, resources) {
  const button = element("button", `summary-button ${className}`);
  button.dataset.focusKey = key; button.dataset.label = label;
  const checking = busy && resources.length && resources.every(r => !r.data && r.error?.code === "pending");
  button.append(element("span", "summary-line " + summary.tone, checking ? "Чтение…" : summary.label));
  if (summary.detail && !checking) button.append(element("span", "summary-sub", summary.detail));
  button.title = [summary.label, summary.detail].filter(Boolean).join(" · ");
  button.addEventListener("click", action);
  return button;
}
function renderGroups() {
  if (!view) return;
  const focused = document.activeElement?.dataset.focusKey;
  const opened = new Set([...document.querySelectorAll("details[open][data-focus-key]")].map(node => node.dataset.focusKey));
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
    const line = element("div", "task-row");
    const info = element("div", "task-identity");
    info.append(element("span", "group-dot"));
    const name = element("div", "task-name");
    name.append(element("div", "task-key", group.keys?.join(" / ") || "ГРУППА"));
    let title = group.title || "Без названия";
    if (group.keys?.length === 1 && title.startsWith(group.keys[0])) title = title.slice(group.keys[0].length).replace(/^[\s·:—-]+/, "") || title;
    const heading = openButton(title, group.id, undefined, "task-heading", `heading:${group.id}`);
    heading.title = group.title || title;
    name.append(heading); info.append(name); line.append(info);
    const details = element("details", "task-details");
    details.dataset.focusKey = `details:${group.id}`; details.open = opened.has(details.dataset.focusKey);
    const toggle = element("summary"); toggle.dataset.focusKey = `toggle:${group.id}`;
    toggle.append(element("span", "sr-only", `Подробности: ${group.title}`));
    toggle.title = "Карточки, MR и вкладки"; details.append(toggle);
    const expand = () => { details.open = !details.open; };
    const summary = summarizeGroup(group);
    const cards = group.resources.filter(r => r.kind === "kaiten");
    const mrs = group.resources.filter(r => r.kind === "gitlab");
    line.append(summaryButton(summary.kaiten, "kaiten-summary", "Kaiten", `card-summary:${group.id}`, () => cards.length === 1 ? activateResource(group.id, cards[0].key) : expand(), cards));
    line.append(summaryButton(summary.mr, "mr-summary", "Merge requests", `mr-summary:${group.id}`, expand, mrs));
    line.append(summaryButton(summary.review, "review-summary", "Ревью", `review-summary:${group.id}`, expand, mrs));
    line.append(summaryButton(summary.ci, "ci-summary", "CI", `ci-summary:${group.id}`, expand, mrs));
    const content = element("div", "detail-content");
    if (group.state === "conflict") content.append(element("p", "resource-error", "В группе разные задачи — показаны все найденные карточки и MR."));
    for (const [kind, label] of [["kaiten", "Kaiten"], ["gitlab", "Merge requests / ревью / CI"]]) {
      const column = element("div"); column.append(element("h2", "detail-heading", label));
      const resources = group.resources.filter(r => r.kind === kind);
      if (!resources.length) column.append(element("p", "hint", kind === "kaiten" ? "Нет карточки и номера задачи в MR" : "Нет вкладок с MR"));
      for (const resource of resources) column.append(renderResource(resource, group.id));
      content.append(column);
    }
    const tabs = element("details", "tabs-list"); tabs.dataset.focusKey = `tab-list:${group.id}`; tabs.open = opened.has(tabs.dataset.focusKey);
    const tabsToggle = element("summary", "", `${group.tabs.length} вкладок в группе`); tabsToggle.dataset.focusKey = `tabs:${group.id}`;
    tabs.append(tabsToggle);
    for (const tab of group.tabs) tabs.append(openButton(tab.title + (tab.discarded ? " · выгружена" : ""), group.id, tab.id, "", `tab:${tab.id}`));
    content.append(tabs); details.append(content); row.append(line, details);
    return row;
  }));
  if (focused) [...document.querySelectorAll("[data-focus-key]")].find(node => node.dataset.focusKey === focused)?.focus({ preventScroll: true });
  const hasScope = view.windowId != null;
  $("column-head").hidden = !groups.length;
  $("empty").hidden = !hasScope || groups.length > 0;
  $("empty-message").textContent = !view.groups.length ? "В этом окне нет групп вкладок. Выберите другое окно в списке «Источник»." : "Нет задач, подходящих под поиск и фильтр.";
  $("clear-filters").hidden = !view.groups.length;
  $("status").textContent = !hasScope ? "Окно с группами не выбрано" : `Показано ${groups.length} из ${view.groups.length} групп${busy ? " · проверяем статусы…" : ""}`;
  const checked = view.groups.flatMap(group => group.resources).map(r => r.checkedAt).filter(Boolean);
  $("freshness").textContent = busy ? "Читаем статусы…" : checked.length ? `Данные · ${time(Math.min(...checked))}` : "";
  const resources = view.groups.flatMap(group => group.resources);
  const fresh = resources.filter(r => r.data && !isStale(r)).length;
  const failures = resources.filter(r => r.error && !["permission", "pending"].includes(r.error.code)).length;
  const reviewFailures = resources.filter(r => r.kind === "gitlab" && r.data && (!r.data.review || r.data.review.status === "unknown")).length;
  $("data-health").hidden = !hasScope;
  $("data-health").textContent = resources.length ? `Свежие данные: ${fresh} из ${resources.length}${failures ? ` · ошибок: ${failures}` : ""}${reviewFailures ? ` · ревью без данных: ${reviewFailures}` : ""}` : "Карточки и MR не найдены";
  $("data-health").classList.toggle("stale", failures + reviewFailures > 0);
}
function render() {
  const hasScope = view.windowId != null;
  if (renderedScope !== view.windowId) { filter = "all"; $("search").value = ""; renderedScope = view.windowId; }
  const placeholder = element("option", "", "Выберите окно с группами…"); placeholder.value = ""; placeholder.disabled = true;
  const windowLabel = (window, index) => `Окно ${index + 1} · групп: ${window.groupCount ?? "?"} · вкладок: ${window.count}`;
  $("window").replaceChildren(placeholder, ...view.windows.map((window, index) => {
    const option = element("option", "", windowLabel(window, index));
    option.value = window.id; option.title = window.sample || `ID ${window.id}`; return option;
  }));
  $("window").value = changingWindowId ?? view.windowId ?? "";
  $("onboarding").hidden = hasScope;
  for (const id of ["access", "search-line", "filters"]) $(id).hidden = !hasScope;
  $("refresh").disabled = busy || !hasScope;
  $("auto").disabled = !hasScope;
  $("window-choices").replaceChildren(...view.windows.map((window, index) => {
    const choice = element("button", "window-choice");
    choice.append(element("strong", "", windowLabel(window, index)), element("span", "hint", window.sample || "Нет групп вкладок"), element("span", "choice-action", "Показать группы →"));
    choice.addEventListener("click", () => selectWindow(window.id));
    return choice;
  }));
  if (!view.windows.length) $("window-choices").append(element("p", "hint", "Откройте обычное окно браузера с группами задач."));
  const granted = Object.values(view.permissions).filter(Boolean).length;
  $("enable").hidden = granted === STATUS_ORIGINS.length;
  $("disable").hidden = granted === 0;
  $("access-title").textContent = granted ? `Доступ к статусам · ${granted} из 2 сайтов` : "Подключить статусы Kaiten и GitLab";
  $("access-description").textContent = granted ? "Разрешение выдано, но успешное чтение зависит от входа на сайт и доступности API. Выгруженные вкладки нужно открыть самостоятельно." : "Список и переходы уже работают. Для статусов разрешите только чтение через ваш вход в открытых вкладках. Без копирования токенов.";
  renderGroups();
}
async function refresh(force = false, readStatuses = $("auto").checked) {
  if (document.hidden) return;
  if (busy) { pending = true; pendingForce ||= force; pendingStatuses ||= readStatuses; return; }
  const startedRevision = revision;
  let progressTimer;
  let reading = false;
  const progress = async () => {
    if (!reading || document.hidden || startedRevision !== revision) return;
    try {
      const inventory = await call({ type: "overview", readStatuses: false });
      if (reading && startedRevision === revision) { view = inventory; renderGroups(); }
    } catch { /* The final request reports errors; progress is best effort. */ }
    if (reading) progressTimer = setTimeout(progress, 700);
  };
  busy = true; $("refresh").disabled = true; $("status").textContent = "Обновляем статусы…";
  try {
    const inventory = await call({ type: "overview", readStatuses: false });
    if (startedRevision !== revision) return;
    view = inventory; showError(view.error || ""); render();
    if (readStatuses && view.windowId != null && Object.values(view.permissions).some(Boolean) && !document.hidden) {
      reading = true; progressTimer = setTimeout(progress, 700);
      const statuses = await call({ type: "overview", force });
      if (startedRevision !== revision) return;
      view = statuses; showError(view.error || ""); render();
    }
  }
  catch (error) { if (startedRevision === revision) showError(error.message); }
  finally {
    reading = false; clearTimeout(progressTimer);
    busy = false; $("refresh").disabled = view?.windowId == null;
    if (view) renderGroups(); else $("status").textContent = "Обзор пока недоступен.";
    if (pending) {
      const forceNext = pendingForce, statusesNext = pendingStatuses;
      pending = pendingForce = pendingStatuses = false;
      queueMicrotask(() => refresh(forceNext, statusesNext));
    }
  }
}
$("refresh").addEventListener("click", () => refresh(true, true));
async function selectWindow(windowId) {
  if (selecting) return;
  selecting = true; revision++; changingWindowId = windowId; $("window").disabled = true;
  try {
    await call({ type: "select", windowId: Number(windowId) });
    filter = "all"; $("search").value = "";
    view = await call({ type: "overview", readStatuses: false });
    showError(view.error || ""); render();
    await refresh(false, true);
  } catch (error) { showError(error.message); }
  finally {
    selecting = false; changingWindowId = null; $("window").disabled = false;
    $("window").value = view?.windowId ?? "";
  }
}
$("window").addEventListener("change", () => selectWindow($("window").value));
$("clear-filters").addEventListener("click", () => { filter = "all"; $("search").value = ""; renderGroups(); });
$("enable").addEventListener("click", () => {
  // Call request directly from the gesture, before any await.
  chrome.permissions.request({ permissions: ["scripting"], origins: STATUS_ORIGINS }).then(granted => {
    if (granted) refresh(false, true); else showError("Разрешение не выдано. Список групп и переходы продолжают работать.");
  }).catch(error => showError(error.message));
});
$("disable").addEventListener("click", async () => {
  try { await chrome.permissions.remove({ permissions: ["scripting"], origins: STATUS_ORIGINS }); await refresh(); }
  catch (error) { showError(error.message); }
});
$("diagnose").addEventListener("click", async () => {
  if (!view) return;
  const report = JSON.stringify(statusDiagnostics({ ...view, reading: busy }, chrome.runtime.getManifest().version), null, 2);
  $("diagnostics").textContent = report; $("diagnostics").hidden = false;
  try { await navigator.clipboard.writeText(report); $("diagnose").textContent = "Диагностика скопирована"; }
  catch { $("diagnose").textContent = "Выделите и скопируйте отчёт ниже"; }
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
chrome.storage.onChanged.addListener((changes, area) => {
  if (area === "session" && changes.windowId && !selecting) { revision++; scheduleRefresh(); }
});
document.addEventListener("visibilitychange", () => { if (!document.hidden) refresh(); });
setInterval(() => { if (!document.hidden && $("auto").checked) refresh(); }, REFRESH_MS);
// Age labels/filters even with automatic site requests disabled. This does not fetch.
setInterval(() => { if (!document.hidden && view) renderGroups(); }, 15_000);
refresh(false, true);
