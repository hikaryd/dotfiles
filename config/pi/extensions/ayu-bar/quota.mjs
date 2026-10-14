// Account-wide Codex allowance. API-token buckets, model and review limits
// are deliberately excluded. No Codex CLI, CodexBar or external auth files.
const WEEK_SECONDS = 7 * 24 * 60 * 60;

function parseWindow(value) {
  if (
    value?.limit_window_seconds !== WEEK_SECONDS ||
    typeof value.used_percent !== "number" ||
    !Number.isFinite(value.used_percent) ||
    value.used_percent < 0 ||
    value.used_percent > 100
  )
    return undefined;
  return {
    usedPercent: value.used_percent,
    remainingPercent: 100 - value.used_percent,
    resetsAt:
      typeof value.reset_at === "number" &&
      Number.isFinite(value.reset_at) &&
      value.reset_at > 0
        ? value.reset_at * 1000
        : undefined,
  };
}

export function parseQuota(payload) {
  const limits = payload?.rate_limit;
  const weekly = [limits?.primary_window, limits?.secondary_window]
    .map(parseWindow)
    .find(Boolean);
  if (!weekly) throw new Error("unavailable");
  return { weekly };
}

/** getAuth must resolve Pi's openai-codex (legacy) provider, including refresh. */
export async function fetchCodexQuota(getAuth, signal, request = fetch) {
  const result = await getAuth();
  signal.throwIfAborted();
  const token = result?.source === "OAuth" ? result.auth?.apiKey : undefined;
  if (!token) throw new Error("login_required");
  let account;
  try {
    const claims = JSON.parse(
      Buffer.from(token.split(".")[1], "base64url").toString("utf8"),
    );
    account = claims["https://api.openai.com/auth"]?.chatgpt_account_id;
  } catch {}
  if (typeof account !== "string" || !account) throw new Error("unavailable");
  const response = await request("https://chatgpt.com/backend-api/wham/usage", {
    headers: {
      Authorization: `Bearer ${token}`,
      "ChatGPT-Account-Id": account,
      Accept: "application/json",
    },
    signal: AbortSignal.any([signal, AbortSignal.timeout(15000)]),
    redirect: "error",
  });
  if (!response.ok) {
    await response.body?.cancel();
    throw new Error("unavailable");
  }
  return parseQuota(await response.json());
}

function formatReset(resetsAt, now) {
  if (resetsAt === undefined) return "";
  const minutes = Math.ceil((resetsAt - now) / 60000);
  if (minutes >= 1440)
    return `${Math.floor(minutes / 1440)}d ${Math.floor((minutes % 1440) / 60)}h`;
  if (minutes >= 60) return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
  return `${minutes}m`;
}

export function formatQuota(state, now = Date.now()) {
  const weekly = state.quota?.weekly;
  if (!weekly) {
    if (state.loading) return "Codex 7d …";
    return state.error === "login_required"
      ? "Codex 7d — /login: OpenAI Codex (legacy)"
      : "Codex 7d — unavailable";
  }
  if (weekly.resetsAt !== undefined && weekly.resetsAt <= now)
    return "Codex 7d — resetting";
  const count = Math.round((weekly.remainingPercent * 8) / 100);
  const stale =
    state.error ||
    (state.updatedAt !== undefined && now - state.updatedAt > 5 * 60000);
  const reset = formatReset(weekly.resetsAt, now);
  const suffix = stale ? " · stale" : reset ? ` · ↻ ${reset}` : "";
  return `Codex 7d ${"▰".repeat(count)}${"▱".repeat(8 - count)} ${Math.floor(weekly.remainingPercent)}% left${suffix}`;
}

/** Poll outside rendering; deduplicate requests and never invent allowance. */
export function createQuotaReader(run, onChange = () => {}, now = Date.now) {
  let state = {
    quota: undefined,
    error: undefined,
    loading: false,
    updatedAt: undefined,
  };
  let pending;
  let controller;
  let stopped = false;
  let lastAttempt = -Infinity;
  function refresh(force = false) {
    if (stopped) return Promise.resolve();
    if (pending) return pending;
    if (!force && now() - lastAttempt < 15000) return Promise.resolve();
    lastAttempt = now();
    controller = new AbortController();
    const signal = controller.signal;
    state = { ...state, loading: true };
    onChange();
    pending = (async () => {
      try {
        const quota = await Promise.resolve().then(() => run(signal));
        if (!stopped)
          state = { quota, error: undefined, loading: false, updatedAt: now() };
      } catch (error) {
        if (!stopped)
          state = {
            ...state,
            loading: false,
            error:
              error?.message === "login_required"
                ? "login_required"
                : "unavailable",
          };
      } finally {
        pending = undefined;
        if (!stopped) onChange();
      }
    })();
    return pending;
  }
  return {
    refresh,
    getState: () => state,
    stop() {
      stopped = true;
      controller?.abort();
    },
  };
}
