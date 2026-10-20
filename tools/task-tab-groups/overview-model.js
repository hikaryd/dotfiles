import { inferGroupName } from "./naming.js";

export const STATUS_ORIGINS = ["https://kaiten.x5.ru/*", "https://scm.x5.ru/*"];
export const REFRESH_MS = 60_000;
export const STALE_MS = 120_000;
export const GROUP_COLORS = { grey: "#9aa0a6", blue: "#8ab4f8", red: "#f28b82", yellow: "#fdd663", green: "#81c995", pink: "#ff8bcb", purple: "#c58af9", cyan: "#78d9ec", orange: "#fcad70" };

// Parse a tab's explicit resource URL. MR task-number inference is a separate
// deployment-specific step in resourcesForTabs, never based on a manual group name.
export function resourceForTab(tab) {
  let url;
  try { url = new URL(tab.url); } catch { return null; }
  if (url.protocol !== "https:" || url.username || url.password || url.port) return null;
  if (/(?:^|\/)(?:login|signin|sign_in|auth|oauth2?|sso)(?:\/|$)/i.test(url.pathname)) return null;
  if (url.hostname === "kaiten.x5.ru") {
    const fragment = new URLSearchParams(url.hash.replace(/^#\??/, ""));
    const candidates = [url.searchParams.get("card_id"), url.pathname.match(/\/cards?\/(\d+)(?:\/|$)/)?.[1], fragment.get("card_id")];
    const ids = [...new Set(candidates.filter(id => /^\d+$/.test(id || "")))];
    if (ids.length !== 1) return null;
    return { kind: "kaiten", origin: url.origin, id: ids[0], key: `${url.origin}/cards/${ids[0]}` };
  }
  if (url.hostname === "scm.x5.ru") {
    const match = url.pathname.match(/^\/(.+)\/-\/merge_requests\/(\d+)(?:\/|$)/);
    if (!match) return null;
    let project;
    try { project = decodeURIComponent(match[1]); } catch { return null; }
    if (project.split("/").some(part => !part || part === "." || part === "..")) return null;
    return { kind: "gitlab", origin: url.origin, project, id: match[2], key: `${url.origin}/${encodeURIComponent(project)}/merge_requests/${match[2]}` };
  }
  return null;
}

export function resourcesForTabs(tabs) {
  const resources = new Map();
  for (const tab of [...tabs].sort((a, b) => a.index - b.index)) {
    const resource = resourceForTab(tab);
    if (!resource) continue;
    if (!resources.has(resource.key)) resources.set(resource.key, { ...resource, tabIds: [] });
    resources.get(resource.key).tabIds.push(tab.id);
  }
  // Deployment-specific convention confirmed by the user: ATS-N is Kaiten
  // card N. Infer only from actual GitLab MR metadata, never a manual group name
  // or an MR iid; an explicit Kaiten tab remains the authoritative source.
  if (![...resources.values()].some(r => r.kind === "kaiten")) {
    for (const tab of tabs) {
      if (tab.status === "loading" || resourceForTab(tab)?.kind !== "gitlab") continue;
      for (const key of inferGroupName({ tabs: [tab] }).keys) {
        if (!/^ATS-\d+$/.test(key)) continue;
        const resource = resourceForTab({ url: kaitenCardUrl(key.slice(4)) });
        if (!resources.has(resource.key)) resources.set(resource.key, { ...resource, tabIds: [], sourceTabIds: [] });
        resources.get(resource.key).sourceTabIds.push(tab.id);
      }
    }
  }
  return [...resources.values()];
}

export function kaitenCardUrl(id) {
  if (!/^\d+$/.test(id)) throw new Error("Некорректный номер карточки.");
  // ATS workspace of the supported Kaiten deployment; not a universal URL rule.
  return `https://kaiten.x5.ru/space/18571/boards/card/${id}`;
}

export function isStale(resource, now = Date.now()) {
  return Boolean(resource.data) && (Boolean(resource.error) || !resource.checkedAt || now - resource.checkedAt >= STALE_MS);
}

export function groupCategory(group, now = Date.now()) {
  const resources = group.resources;
  if (group.state === "conflict" || resources.some(r => r.error && !["permission", "pending"].includes(r.error.code))) return "attention";
  if (resources.some(r => r.data && (r.data.blocked || r.data.hasConflicts || r.data.discussionsResolved === false || ["failed", "canceled"].includes(r.data.pipeline) || r.data.review?.error || (r.data.state === "opened" && !r.data.draft && r.data.review?.status === "pending")))) return "attention";
  if (!resources.length || resources.some(r => !r.data || isStale(r, now))) return "unknown";
  const cards = resources.filter(r => r.kind === "kaiten");
  const mrs = resources.filter(r => r.kind === "gitlab");
  // A merged MR is not proof that its Kaiten card is done. An archived card is not either.
  if (cards.length && cards.every(r => r.data.state === 3) && mrs.every(r => ["merged", "closed"].includes(r.data.state))) return "done";
  if (cards.some(r => [1, 2].includes(r.data.state)) || mrs.some(r => r.data.state === "opened")) return "active";
  return "unknown";
}

// Compact summaries never turn missing/stale statuses into a green aggregate.
export function summarizeGroup(group, now = Date.now()) {
  const cards = group.resources.filter(r => r.kind === "kaiten");
  const mrs = group.resources.filter(r => r.kind === "gitlab");
  const missing = resources => resources.filter(r => !r.data).length;
  const stale = resources => resources.some(r => isStale(r, now));
  const unavailable = resources => {
    const codes = resources.map(r => r.error?.code);
    if (!resources.length) return { label: "—", detail: "Нет вкладки", tone: "neutral" };
    const labels = { auth: "API: 401", forbidden: "Нет доступа", schema: "Ошибка формата", frozen: "Вкладка спит", tab: "Откройте вкладку", timeout: "Нет ответа", network: "Ошибка сети", missing: "API: 404", "rate-limit": "Лимит запросов", permission: "Статусы отключены", pending: "Не проверено" };
    const code = codes.find(code => code && !["pending", "permission"].includes(code)) || codes[0];
    return { label: labels[code] || "Нет данных", detail: resources.find(r => r.error?.code === code)?.error?.message || "", tone: ["pending", "permission"].includes(code) ? "neutral" : "warn" };
  };
  const qualify = (summary, resources) => {
    if (stale(resources)) return { ...summary, tone: "warn", detail: "Последние известные данные" + (resources.some(r => r.error) ? ` · ${unavailable(resources).label}` : "") };
    if (missing(resources)) return { ...summary, tone: "warn", detail: `${missing(resources)} без данных` };
    return summary;
  };
  const card = cards.find(r => r.data)?.data;
  const blocked = cards.some(r => r.data?.blocked);
  const archived = cards.some(r => r.data?.archived);
  const kaiten = card ? qualify({
    label: cards.length > 1 ? `${cards.length} карточки` : card.column || { 1: "В очереди", 2: "В работе", 3: "Готово" }[card.state],
    detail: blocked ? "Заблокирована" : archived ? "В архиве" : cards.length > 1 ? "Откройте подробности" : card.assignees?.join(", ") || "Ответственный не указан",
    tone: blocked ? "bad" : archived ? "neutral" : cards.every(r => r.data?.state === 3) ? "good" : "",
  }, cards) : unavailable(cards);
  const counts = { opened: 0, draft: 0, merged: 0, closed: 0, locked: 0 };
  for (const { data } of mrs) if (data) counts[data.state === "opened" && data.draft ? "draft" : data.state]++;
  const parts = [];
  if (counts.opened) parts.push(`${counts.opened} ${counts.opened === 1 ? "открыт" : "открыты"}`);
  if (counts.draft) parts.push(`${counts.draft} Draft`);
  if (counts.merged) parts.push(`${counts.merged} Merged`);
  if (counts.closed) parts.push(`${counts.closed} закрыт${counts.closed > 1 ? "ы" : ""}`);
  if (counts.locked) parts.push(`${counts.locked} locked`);
  const conflict = mrs.some(r => r.data?.hasConflicts);
  const discussions = mrs.some(r => r.data?.discussionsResolved === false);
  const approvals = mrs.some(r => r.data?.mergeStatus === "not_approved");
  const mr = parts.length ? qualify({ label: parts.join(" · "),
    detail: conflict ? "Конфликты" : discussions ? "Есть обсуждения" : approvals ? "Нужны одобрения" : "",
    tone: conflict ? "bad" : discussions || approvals ? "warn" : counts.merged === mrs.length ? "good" : "",
  }, mrs) : unavailable(mrs);
  const pipelines = mrs.map(r => r.data?.pipeline);
  // MR count is not pipeline count: a documentation MR may have no CI at all.
  const withoutCI = pipelines.filter(p => p === "none").length;
  const withCI = pipelines.filter(p => p !== "none");
  const skipped = pipelines.filter(p => p === "skipped").length;
  const passed = pipelines.filter(p => p === "success").length;
  const ciDetail = `${passed} из ${withCI.length}${withoutCI ? ` · ${withoutCI} MR без CI` : ""}`;
  let ci = unavailable(mrs);
  const failed = pipelines.filter(p => ["failed", "canceled"].includes(p)).length;
  if (failed) ci = { label: `✕ CI упал · ${failed}`, detail: "", tone: "bad" };
  else if (pipelines.includes("running")) ci = { label: "◷ Выполняется", detail: "", tone: "" };
  else if (pipelines.includes("manual")) ci = { label: "Ручной шаг", detail: "", tone: "warn" };
  else if (pipelines.some(p => ["pending", "created", "preparing", "waiting_for_resource", "scheduled"].includes(p))) ci = { label: "В очереди", detail: "", tone: "" };
  else if (withCI.length && withCI.every(p => p === "success")) ci = { label: "✓ Пройден", detail: ciDetail, tone: "good" };
  else if (mrs.length && pipelines.every(p => p === "none")) ci = { label: "Нет pipeline", detail: "", tone: "neutral" };
  else if (skipped && pipelines.every(p => ["success", "none", "skipped"].includes(p))) ci = { label: `CI пропущен · ${skipped}`, detail: ciDetail, tone: "neutral" };
  else if (parts.length) ci = { label: "CI неизвестен", detail: "", tone: "neutral" };
  const reviews = mrs.map(r => r.data?.review);
  const approved = reviews.filter(r => r?.status === "approved").length;
  const waiting = reviews.filter(r => r?.status === "pending").length;
  const noApprovals = reviews.filter(r => r?.status === "not-required").length;
  const unknown = mrs.length - approved - waiting - noApprovals;
  let review = unavailable(mrs);
  if (mrs.some(r => r.data)) {
    const progress = `${approved} из ${mrs.length} MR одобрены${unknown ? ` · нет данных: ${unknown}` : ""}`;
    if (waiting) {
      const remaining = reviews[0]?.remaining;
      review = { label: mrs.length === 1 ? "Ждёт одобрений" : `Ждут одобрений · ${waiting}`,
        detail: mrs.length === 1 && remaining > 0 ? `Нужно ещё: ${remaining}` : progress, tone: "warn" };
    } else if (unknown) review = { label: "Ревью неизвестно", detail: progress, tone: "warn" };
    else if (approved === mrs.length) review = { label: mrs.length === 1 ? "✓ Одобрено" : `✓ Одобрены · ${approved}`,
      detail: mrs.length === 1 ? `Одобрений: ${reviews[0].count}` : progress, tone: "good" };
    else if (noApprovals === mrs.length) review = { label: "Нет одобрений",
      detail: reviews.some(r => r?.excludedCount > 0) ? "Автоодобрение не учитывается" : "Не обязательны по правилам", tone: "neutral" };
    else review = { label: `${approved} из ${mrs.length} одобрены`, detail: `${noApprovals} без одобрений`, tone: "neutral" };
  }
  return { kaiten, mr, review: qualify(review, mrs), ci: qualify(ci, mrs) };
}

// Safe to share: no titles, URLs, identifiers, people, raw API bodies or credentials.
export function statusDiagnostics(view, version, now = Date.now()) {
  const resources = [...new Map(view.groups.flatMap(g => g.resources).map(r => [r.key, r])).values()];
  const ci = {}, review = {}, reviewErrors = {};
  const statuses = ["success", "failed", "canceled", "running", "manual", "pending", "created", "preparing", "waiting_for_resource", "scheduled", "skipped", "none"];
  for (const item of resources.filter(r => r.kind === "gitlab")) {
    const status = !item.data ? "unavailable" : isStale(item, now) ? "stale" : statuses.includes(item.data.pipeline) ? item.data.pipeline : "unknown";
    ci[status] = (ci[status] || 0) + 1;
    const state = !item.data ? "unavailable" : isStale(item, now) ? "stale"
      : ["approved", "pending", "not-required"].includes(item.data.review?.status) ? item.data.review.status : "unknown";
    review[state] = (review[state] || 0) + 1;
    if (item.data?.review?.error) {
      const raw = item.data.review.error.code;
      const code = ["auth", "forbidden", "schema", "timeout", "network", "missing", "rate-limit", "http"].includes(raw) ? raw : "unknown";
      reviewErrors[code] = (reviewErrors[code] || 0) + 1;
    }
  }
  return { version, reading: Boolean(view.reading), groups: view.groups.length, ci, review, reviewErrors,
    sources: Object.fromEntries(["kaiten", "gitlab"].map(kind => {
      const items = resources.filter(r => r.kind === kind);
      const errors = {};
      for (const item of items) if (item.error) errors[item.error.code] = (errors[item.error.code] || 0) + 1;
      return [kind, { total: items.length, fresh: items.filter(r => r.data && !isStale(r, now)).length, errors }];
    })),
  };
}

export function matchesSearch(group, query) {
  const words = query.toLocaleLowerCase("ru").trim().split(/\s+/).filter(Boolean);
  const text = [group.title, ...(group.keys || []), ...group.resources.flatMap(r => [r.project, r.id, r.data?.title, r.data?.column, ...(r.data?.assignees || [])])].join(" ").toLocaleLowerCase("ru");
  return words.every(word => text.includes(word));
}
