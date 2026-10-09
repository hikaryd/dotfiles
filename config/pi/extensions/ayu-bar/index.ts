import { basename } from "node:path";
import type {
  ExtensionAPI,
  ExtensionContext,
  Theme,
} from "@earendil-works/pi-coding-agent";
import { truncateToWidth, visibleWidth } from "@earendil-works/pi-tui";
import { createQuotaReader, formatReset, formatTokens } from "./quota.mjs";
import { latestThroughput } from "./throughput.mjs";

type QuotaReader = ReturnType<typeof createQuotaReader>;
type FooterData = Parameters<
  NonNullable<Parameters<ExtensionContext["ui"]["setFooter"]>[0]>
>[2];

function clean(text: string): string {
  return text
    .replace(/[\u0000-\u001f\u007f-\u009f]/g, " ")
    .replace(/\s+/g, " ")
    .trim();
}

function fitRow(left: string, right: string, width: number): string {
  left = truncateToWidth(left, width, "…");
  const room = width - visibleWidth(left) - 2;
  if (room <= 0 || !right) return left;
  right = truncateToWidth(right, room, "…");
  return (
    left + " ".repeat(width - visibleWidth(left) - visibleWidth(right)) + right
  );
}

export function renderStatusBar(
  ctx: ExtensionContext,
  pi: ExtensionAPI,
  theme: Theme,
  footerData: FooterData,
  state: ReturnType<QuotaReader["getState"]>,
  width: number,
  now = Date.now(),
  throughput?: ReturnType<typeof latestThroughput>,
): string[] {
  const inner = Math.max(0, width - 2);
  const separator = theme.fg("dim", " · ");
  const model = clean(
    ctx.model ? `${ctx.model.provider}/${ctx.model.id}` : "no model",
  );
  const thinking = ctx.model?.reasoning
    ? separator + theme.fg("muted", pi.getThinkingLevel())
    : "";
  const label = theme.style(" π ", {
    fg: "accent",
    bg: "selectedBg",
    bold: true,
  });
  const modelLine = label + " " + theme.fg("text", model) + thinking;
  const branch = footerData.getGitBranch();
  const location =
    theme.fg("muted", clean(basename(ctx.cwd))) +
    (branch ? separator + theme.fg("accent", clean(branch)) : "");
  const tps =
    throughput &&
    throughput.provider === ctx.model?.provider &&
    throughput.model === ctx.model?.id
      ? `${throughput.tps.toFixed(1)} tps`
      : "— tps";
  const responseStats = theme.fg("muted", tps) + separator + location;

  const context = ctx.getContextUsage();
  const percent = context?.percent;
  const tone =
    percent != null && percent >= 90
      ? "error"
      : percent != null && percent >= 70
        ? "warning"
        : "muted";
  const contextPercent = percent == null ? "?" : `${Math.round(percent)}%`;
  const window = context?.contextWindow ?? ctx.model?.contextWindow;
  const tokens = context?.tokens == null ? "?" : formatTokens(context.tokens);
  const contextLine =
    theme.fg(tone, `ctx ${contextPercent}`) +
    (window ? theme.fg("dim", ` ${tokens}/${formatTokens(window)}`) : "");

  const weekly = state.quota?.weekly;
  let quotaLine: string;
  if (!weekly) {
    quotaLine = theme.fg(
      "muted",
      `Codex 7d ${state.loading ? "…" : "— unavailable"}`,
    );
  } else if (weekly.resetsAt !== undefined && weekly.resetsAt <= now) {
    // An elapsed reset time is not proof the server has restored the allowance.
    quotaLine = theme.fg("muted", "Codex 7d — resetting");
  } else {
    const remaining = Math.floor(weekly.remainingPercent);
    const tone =
      remaining <= 10 ? "error" : remaining <= 25 ? "warning" : "accent";
    const count = Math.round((weekly.remainingPercent / 100) * 8);
    const meter =
      theme.fg(tone, "▰".repeat(count)) +
      theme.fg("dim", "▱".repeat(8 - count));
    const value = theme.fg(tone, `${remaining}% left`);
    const reset = formatReset(weekly.resetsAt, now);
    const stale =
      state.error ||
      (state.updatedAt !== undefined && now - state.updatedAt > 5 * 60000);
    const suffix = stale
      ? theme.fg("warning", " · stale")
      : reset
        ? theme.fg("dim", ` · ↻ ${reset}`)
        : "";
    const title = theme.fg("muted", "Codex 7d ");
    const variants = [
      title + meter + " " + value + suffix,
      title + value + suffix,
      title + value,
    ];
    quotaLine =
      variants.find(
        (text) => visibleWidth(text) + visibleWidth(contextLine) + 2 <= inner,
      ) ?? variants[2];
    if (stale && !quotaLine.includes("stale"))
      quotaLine += theme.fg("warning", " ~");
  }

  const pad = (line: string) => truncateToWidth(" " + line, width, "…");
  const lines = [
    pad(fitRow(modelLine, responseStats, inner)),
    pad(fitRow(quotaLine, contextLine, inner)),
  ];
  const statuses = [...footerData.getExtensionStatuses().values()].map((text) =>
    text.replace(/[\r\n\t]/g, " "),
  );
  if (statuses.length)
    lines.push(pad(truncateToWidth(statuses.join(" "), inner, "…")));
  return lines;
}

export default function (pi: ExtensionAPI) {
  let reader: QuotaReader | undefined;
  let disposeFooter = () => {};

  pi.on("session_start", (_event, ctx) => {
    if (ctx.mode !== "tui") return;
    ctx.ui.setFooter((tui, theme, footerData) => {
      disposeFooter();
      const current = createQuotaReader(
        (signal: AbortSignal) =>
          pi.exec(
            "codexbar",
            [
              "usage",
              "--provider",
              "codex",
              "--source",
              "cli",
              "--format",
              "json",
              "--no-credits",
              "--no-color",
            ],
            { signal, timeout: 15000 },
          ),
        () => tui.requestRender(),
      );
      reader = current;
      let lastLeaf: string | null | undefined;
      let throughput: ReturnType<typeof latestThroughput>;
      const getThroughput = () => {
        const leaf = ctx.sessionManager.getLeafId();
        if (leaf !== lastLeaf) {
          lastLeaf = leaf;
          throughput = latestThroughput(ctx.sessionManager.getBranch());
        }
        return throughput;
      };
      const unsubscribe = footerData.onBranchChange(() => tui.requestRender());
      const timer = setInterval(() => void current.refresh(), 60000);
      timer.unref();
      let disposed = false;
      const dispose = () => {
        if (disposed) return;
        disposed = true;
        clearInterval(timer);
        unsubscribe();
        current.stop();
        if (reader === current) reader = undefined;
      };
      disposeFooter = dispose;
      void current.refresh();
      return {
        invalidate() {},
        dispose,
        render: (width: number) =>
          renderStatusBar(
            ctx,
            pi,
            theme,
            footerData,
            current.getState(),
            width,
            undefined,
            getThroughput(),
          ),
      };
    });
  });

  pi.on("agent_end", () => {
    void reader?.refresh();
  });
  pi.on("session_shutdown", () => disposeFooter());
  pi.registerCommand("quota", {
    description:
      "Refresh the account-wide Codex weekly quota shown in the Ayu bar",
    handler: async (_args, ctx) => {
      if (!reader) {
        ctx.ui.notify("The quota bar is available in terminal mode.", "info");
        return;
      }
      await reader.refresh(true);
      const state = reader.getState();
      const weekly = state.quota?.weekly;
      if (state.error || !weekly) {
        ctx.ui.notify(
          "Codex quota unavailable. Check codexbar and Codex login; the last reading is marked stale.",
          "warning",
        );
      } else {
        ctx.ui.notify(
          `Codex account-wide weekly quota: ${weekly.usedPercent}% used, ${Math.floor(weekly.remainingPercent)}% left.`,
          "info",
        );
      }
    },
  });
}
