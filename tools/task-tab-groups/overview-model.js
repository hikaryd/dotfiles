export const STATUS_ORIGINS = ["https://kaiten.x5.ru/*", "https://scm.x5.ru/*"];
export const REFRESH_MS = 60_000;
export const STALE_MS = 120_000;
export const GROUP_COLORS = { grey: "#9aa0a6", blue: "#8ab4f8", red: "#f28b82", yellow: "#fdd663", green: "#81c995", pink: "#ff8bcb", purple: "#c58af9", cyan: "#78d9ec", orange: "#fcad70" };

// Identity comes from actual card/MR URLs, never from an ATS number or group name.
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
  return [...resources.values()];
}

export function isStale(resource, now = Date.now()) {
  return Boolean(resource.data) && (Boolean(resource.error) || !resource.checkedAt || now - resource.checkedAt >= STALE_MS);
}

export function groupCategory(group, now = Date.now()) {
  const resources = group.resources;
  if (group.state === "conflict" || resources.some(r => r.error && r.error.code !== "permission")) return "attention";
  if (resources.some(r => r.data && (r.data.blocked || r.data.hasConflicts || r.data.discussionsResolved === false || ["failed", "canceled"].includes(r.data.pipeline)))) return "attention";
  if (!resources.length || resources.some(r => !r.data || isStale(r, now))) return "unknown";
  const cards = resources.filter(r => r.kind === "kaiten");
  const mrs = resources.filter(r => r.kind === "gitlab");
  // A merged MR is not proof that its Kaiten card is done. An archived card is not either.
  if (cards.length && cards.every(r => r.data.state === 3) && mrs.every(r => ["merged", "closed"].includes(r.data.state))) return "done";
  if (cards.some(r => [1, 2].includes(r.data.state)) || mrs.some(r => r.data.state === "opened")) return "active";
  return "unknown";
}

export function matchesSearch(group, query) {
  const words = query.toLocaleLowerCase("ru").trim().split(/\s+/).filter(Boolean);
  const text = [group.title, ...(group.keys || []), ...group.resources.flatMap(r => [r.project, r.id, r.data?.title, r.data?.column, ...(r.data?.assignees || [])])].join(" ").toLocaleLowerCase("ru");
  return words.every(word => text.includes(word));
}
