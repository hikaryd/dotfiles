// Runs in an ISOLATED world of an existing site tab. Keep this function self-contained:
// executeScript serializes it without imports/closures. No DOM, cookies or token extraction.
export async function readStatusInTab(resource) {
  const fail = (code, message) => ({ error: { code, message } });
  const allowed = { kaiten: "https://kaiten.x5.ru", gitlab: "https://scm.x5.ru" };
  if (allowed[resource.kind] !== resource.origin || location.origin !== resource.origin || !/^\d+$/.test(resource.id)) {
    return fail("navigation", "Вкладка перешла на другую страницу. Обновите обзор.");
  }
  if (resource.kind === "gitlab" && (typeof resource.project !== "string" || resource.project.split("/").some(p => !p || p === "." || p === ".."))) {
    return fail("schema", "Не удалось определить проект MR.");
  }
  const request = async path => {
    try {
      const response = await fetch(path, {
        method: "GET", credentials: "same-origin", redirect: "error", cache: "no-store",
        headers: { Accept: "application/json" }, signal: AbortSignal.timeout(8_000),
      });
      if (response.status === 401) return fail("auth", "Нужен вход на сайт. Откройте вкладку и войдите.");
      if (response.status === 403) return fail("forbidden", "Нет доступа к API. Проверьте вход и права на сайте.");
      if (response.status === 404) return fail("missing", "Карточка/MR или endpoint API недоступны (404).");
      if (response.status === 429) return fail("rate-limit", "Сайт ограничил запросы. Повторим позже.");
      if (!response.ok) return fail("http", `Сайт ответил HTTP ${response.status}.`);
      if (!response.headers.get("content-type")?.includes("json")) return fail("auth", "Вместо данных получена страница. Проверьте вход на сайт.");
      try { return { value: await response.json() }; }
      catch { return fail("schema", "API вернул некорректный JSON."); }
    } catch (error) {
      return fail(error.name === "TimeoutError" || error.name === "AbortError" ? "timeout" : "network", "Не удалось прочитать статус: сеть, VPN, вход или таймаут.");
    }
  };
  const names = people => Array.isArray(people) ? [...new Set(people.map(p => p.full_name || p.name || p.username).filter(v => typeof v === "string"))] : [];
  const text = value => typeof value === "string" ? value : null;
  if (resource.kind === "kaiten") {
    // v1 is the web-client endpoint; latest is the documented public API. Both GET-only.
    let result = await request(`/api/v1/cards/${resource.id}`);
    if (result.error && ["missing", "auth", "forbidden"].includes(result.error.code)) result = await request(`/api/latest/cards/${resource.id}`);
    if (result.error) return result;
    const card = result.value;
    if (!card || String(card.id) !== resource.id || ![1, 2, 3].includes(card.state)) return fail("schema", "Неизвестный формат карточки Kaiten.");
    const members = Array.isArray(card.members) ? card.members.filter(member => member.type === 2) : [];
    return { data: {
      title: text(card.title), state: card.state, column: text(card.column?.title), board: text(card.board?.title),
      assignees: names(members), owner: text(card.owner?.full_name),
      blocked: typeof card.blocked === "boolean" ? card.blocked : null, blockReason: text(card.block_reason),
      archived: card.archived === true || card.condition === 2, dueDate: text(card.due_date),
    } };
  }
  const result = await request(`/api/v4/projects/${encodeURIComponent(resource.project)}/merge_requests/${resource.id}`);
  if (result.error) return result;
  const mr = result.value;
  if (!mr || String(mr.iid) !== resource.id || !["opened", "merged", "closed", "locked"].includes(mr.state)) return fail("schema", "Неизвестный формат MR GitLab.");
  return { data: {
    title: text(mr.title), state: mr.state, draft: mr.draft === true || mr.work_in_progress === true,
    assignees: names(mr.assignees), reviewers: names(mr.reviewers),
    hasConflicts: typeof mr.has_conflicts === "boolean" ? mr.has_conflicts : null,
    discussionsResolved: typeof mr.blocking_discussions_resolved === "boolean" ? mr.blocking_discussions_resolved : null,
    mergeStatus: text(mr.detailed_merge_status),
    pipeline: mr.head_pipeline === null ? "none" : text(mr.head_pipeline?.status),
  } };
}
