// Self-contained for executeScript in an ISOLATED site tab. Also usable from the
// extension worker for sleeping tabs, with normal cookie-based GETs and host permission.
// No DOM, cookies API, credential storage or token extraction.
export async function readStatusInTab(resource, background = false, signal = null) {
  const fail = (code, message) => ({ error: { code, message } });
  const allowed = { kaiten: "https://kaiten.x5.ru", gitlab: "https://scm.x5.ru" };
  const wrongContext = background ? location.protocol !== "chrome-extension:" : location.origin !== resource.origin;
  if (allowed[resource.kind] !== resource.origin || wrongContext || !/^\d+$/.test(resource.id)) {
    return fail("navigation", "Вкладка перешла на другую страницу. Обновите обзор.");
  }
  if (resource.kind === "gitlab" && (typeof resource.project !== "string" || resource.project.split("/").some(p => !p || p === "." || p === ".."))) {
    return fail("schema", "Не удалось определить проект MR.");
  }
  const request = async path => {
    try {
      const response = await fetch(background ? resource.origin + path : path, {
        method: "GET", credentials: background ? "include" : "same-origin", redirect: "error", cache: "no-store",
        headers: { Accept: "application/json" }, signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(8_000)]) : AbortSignal.timeout(8_000),
      });
      if (response.status === 401) return fail("auth", "API не принял авторизацию (HTTP 401).");
      if (response.status === 403) return fail("forbidden", "Нет доступа к API. Проверьте вход и права на сайте.");
      if (response.status === 404) return fail("missing", "Карточка/MR или endpoint API недоступны (404).");
      if (response.status === 429) return fail("rate-limit", "Сайт ограничил запросы. Повторим позже.");
      if (!response.ok) return fail("http", `Сайт ответил HTTP ${response.status}.`);
      if (!response.headers.get("content-type")?.includes("json")) return fail("schema", "API вернул не JSON. Ответ не соответствует формату данных.");
      try { return { value: await response.json() }; }
      catch { return fail("schema", "API вернул некорректный JSON."); }
    } catch (error) {
      return fail(error.name === "TimeoutError" || error.name === "AbortError" ? "timeout" : "network", "Не удалось прочитать статус: сеть, VPN, вход или таймаут.");
    }
  };
  const names = people => Array.isArray(people) ? [...new Set(people.map(p => p.full_name || p.name || p.username).filter(v => typeof v === "string"))] : [];
  const text = value => typeof value === "string" ? value : null;
  if (resource.kind === "kaiten") {
    // Observed in this deployment's web client and verified with its browser
    // session. The versioned public API uses a different auth contract.
    const result = await request(`/api/cards/${resource.id}`);
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
  const path = `/api/v4/projects/${encodeURIComponent(resource.project)}/merge_requests/${resource.id}`;
  const result = await request(path);
  if (result.error) return result;
  const mr = result.value;
  if (!mr || String(mr.iid) !== resource.id || !["opened", "merged", "closed", "locked"].includes(mr.state)) return fail("schema", "Неизвестный формат MR GitLab.");
  // A successful MR/CI read survives an unavailable approvals endpoint. Never
  // infer a review from reviewers, mergeability, CI, or approved:true alone:
  // GitLab EE also returns approved:true when no approval rules are configured.
  const approvalResult = await request(path + "/approvals");
  const parseReview = () => {
    const unknown = error => ({ status: "unknown", error });
    if (approvalResult.error) return unknown(approvalResult.error);
    const value = approvalResult.value;
    const schema = () => unknown(fail("schema", "Неизвестный формат одобрений GitLab.").error);
    const number = n => Number.isInteger(n) && n >= 0 ? n : null;
    if (!value || (value.iid != null && String(value.iid) !== resource.id) || !Array.isArray(value.approved_by)) return schema();
    if (value.approved_by.some(p => !p?.user || (p.user.username != null && typeof p.user.username !== "string") || (typeof p.user.id !== "number" && !p.user.name && !p.user.username))) return schema();
    const required = number(value.approvals_required), remaining = number(value.approvals_left);
    if ((value.approvals_required != null && required === null) || (value.approvals_left != null && remaining === null)) return schema();
    const approved = typeof value.approved === "boolean" ? value.approved
      : required !== null && remaining !== null ? remaining === 0 : null;
    if (approved === null || (approved && remaining > 0)) return schema();
    const approvers = [...new Map(value.approved_by.map(p => [p.user.id ?? p.user.username ?? p.user.name, p.user])).values()];
    if (approved && !approvers.length && required > 0) return schema();
    // This deployment's automated reviewer is explicitly excluded by the team.
    // Match the exact GitLab username, never a mutable display name or a prefix.
    // Keep the policy inside this self-contained function for executeScript.
    const people = approvers.filter(p => p.username?.toLowerCase() !== "srv.samedi");
    const excludedCount = approvers.length - people.length;
    const effectiveRemaining = excludedCount && required !== null
      ? Math.max(remaining ?? 0, required - people.length) : remaining;
    return { status: !approved || effectiveRemaining > 0 ? "pending" : people.length ? "approved" : "not-required",
      count: people.length, required, remaining: effectiveRemaining, approvedBy: names(people), excludedCount, error: null };
  };
  return { data: {
    title: text(mr.title), state: mr.state, draft: mr.draft === true || mr.work_in_progress === true,
    assignees: names(mr.assignees), reviewers: names(mr.reviewers),
    hasConflicts: typeof mr.has_conflicts === "boolean" ? mr.has_conflicts : null,
    discussionsResolved: typeof mr.blocking_discussions_resolved === "boolean" ? mr.blocking_discussions_resolved : null,
    mergeStatus: text(mr.detailed_merge_status),
    pipeline: mr.head_pipeline === null ? "none" : text(mr.head_pipeline?.status),
    review: parseReview(),
  } };
}
