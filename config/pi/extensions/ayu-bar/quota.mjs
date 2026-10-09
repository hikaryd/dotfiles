// Only the account-wide windows: never substitute a model-specific or review limit.
const WEEK_MINUTES = 7 * 24 * 60;

function parseWindow(value, minutes) {
  if (
    !value ||
    value.windowMinutes !== minutes ||
    typeof value.usedPercent !== "number" ||
    !Number.isFinite(value.usedPercent) ||
    value.usedPercent < 0 ||
    value.usedPercent > 100
  ) {
    return undefined;
  }
  const resetsAt = Date.parse(value.resetsAt);
  return {
    usedPercent: value.usedPercent,
    remainingPercent: 100 - value.usedPercent,
    resetsAt: Number.isFinite(resetsAt) ? resetsAt : undefined,
  };
}

export function parseQuota(output) {
  const payload = JSON.parse(output);
  const entries = Array.isArray(payload) ? payload : [payload];
  // Account selection must be unambiguous; don't pick the first of several accounts.
  const accounts = entries.filter((entry) => entry?.provider === "codex");
  if (accounts.length !== 1 || accounts[0].error) {
    throw new Error("Codex quota unavailable");
  }
  const usage = accounts[0].usage;
  const windows = [usage?.secondary, usage?.primary];
  const weekly = windows
    .map((value) => parseWindow(value, WEEK_MINUTES))
    .find(Boolean);
  if (!weekly) throw new Error("No account-wide weekly window");
  return { weekly };
}

export function formatReset(resetsAt, now = Date.now()) {
  if (resetsAt === undefined) return "";
  const minutes = Math.ceil((resetsAt - now) / 60000);
  if (minutes <= 0) return "resetting";
  if (minutes >= 1440)
    return `${Math.floor(minutes / 1440)}d ${Math.floor((minutes % 1440) / 60)}h`;
  if (minutes >= 60) return `${Math.floor(minutes / 60)}h ${minutes % 60}m`;
  return `${minutes}m`;
}

export function formatTokens(value) {
  if (value < 1000) return String(Math.round(value));
  if (value < 1_000_000) return `${Math.round(value / 1000)}k`;
  return `${(value / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
}

/**
 * @param {(signal: AbortSignal) => Promise<{code: number, stdout: string}>} run
 * @param {() => void} [onChange]
 * @param {() => number} [now]
 */
export function createQuotaReader(run, onChange = () => {}, now = Date.now) {
  /** @type {{quota: ReturnType<typeof parseQuota> | undefined, error: string | undefined, loading: boolean, updatedAt: number | undefined}} */
  let state = {
    quota: undefined,
    error: undefined,
    loading: false,
    updatedAt: undefined,
  };
  /** @type {Promise<void> | undefined} */
  let pending;
  let stopped = false;
  /** @type {AbortController | undefined} */
  let controller;
  let lastAttempt = -Infinity;

  function refresh(force = false) {
    if (stopped) return Promise.resolve();
    if (pending) return pending;
    if (!force && now() - lastAttempt < 15000) return Promise.resolve();
    lastAttempt = now();
    controller = new AbortController();
    state = { ...state, loading: true };
    onChange();
    pending = (async () => {
      try {
        const signal = controller.signal;
        const result = await Promise.resolve().then(() => run(signal));
        if (stopped) return;
        if (result.code !== 0) throw new Error("Quota command failed");
        state = {
          quota: parseQuota(result.stdout),
          error: undefined,
          loading: false,
          updatedAt: now(),
        };
      } catch {
        if (stopped) return;
        // Keep the last real reading, visibly stale. Never manufacture a 0%/100% quota.
        state = { ...state, error: "unavailable", loading: false };
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
