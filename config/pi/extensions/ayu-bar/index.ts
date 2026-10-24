import { basename } from "node:path";
import type {
  ExtensionAPI,
  ExtensionContext,
  Theme,
} from "@earendil-works/pi-coding-agent";
import { truncateToWidth, visibleWidth } from "@earendil-works/pi-tui";
import { formatTokens, sessionUsage } from "./status.mjs";
import { createQuotaReader, fetchCodexQuota, formatQuota } from "./quota.mjs";
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
  width: number,
  throughput?: ReturnType<typeof latestThroughput>,
  usage = { input: 0, output: 0, cost: 0 },
  quotaState?: ReturnType<QuotaReader["getState"]>,
  now = Date.now(),
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

  const sessionLine = theme.fg(
    "muted",
    `↑${formatTokens(usage.input)} ↓${formatTokens(usage.output)} $${usage.cost.toFixed(3)}`,
  );
  const pad = (line: string) => truncateToWidth(" " + line, width, "…");
  const lines = [
    pad(fitRow(modelLine, responseStats, inner)),
    pad(fitRow(sessionLine, contextLine, inner)),
  ];
  if (quotaState) {
    const remaining = quotaState.quota?.weekly.remainingPercent;
    const tone = quotaState.error
      ? "warning"
      : remaining == null
        ? "muted"
        : remaining <= 10
          ? "error"
          : remaining <= 25
            ? "warning"
            : "accent";
    lines.push(pad(theme.fg(tone, formatQuota(quotaState, now))));
  }
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
          fetchCodexQuota(
            () => ctx.modelRegistry.getProviderAuth("openai-codex"),
            signal,
          ),
        () => tui.requestRender(),
      );
      reader = current;
      const timer = setInterval(() => void current.refresh(), 60000);
      timer.unref();
      let lastLeaf: string | null | undefined;
      let throughput: ReturnType<typeof latestThroughput>;
      let usage = { input: 0, output: 0, cost: 0 };
      const unsubscribe = footerData.onBranchChange(() => tui.requestRender());
      let disposed = false;
      const dispose = () => {
        if (disposed) return;
        disposed = true;
        clearInterval(timer);
        current.stop();
        if (reader === current) reader = undefined;
        unsubscribe();
      };
      disposeFooter = dispose;
      void current.refresh();
      return {
        invalidate() {},
        dispose,
        render: (width: number) => {
          const leaf = ctx.sessionManager.getLeafId();
          if (leaf !== lastLeaf) {
            lastLeaf = leaf;
            const branch = ctx.sessionManager.getBranch();
            throughput = latestThroughput(branch);
            usage = sessionUsage(branch);
          }
          return renderStatusBar(
            ctx,
            pi,
            theme,
            footerData,
            width,
            throughput,
            usage,
            current.getState(),
          );
        },
      };
    });
  });
  pi.on("agent_end", () => void reader?.refresh());
  pi.on("session_shutdown", () => disposeFooter());
  pi.registerCommand("quota", {
    description: "Refresh the account-wide Codex weekly allowance",
    handler: async (_args, ctx) => {
      if (!reader) {
        ctx.ui.notify("The quota bar is available in terminal mode.", "info");
        return;
      }
      await reader.refresh(true);
      const state = reader.getState();
      ctx.ui.notify(
        state.error === "login_required"
          ? "Run /login and choose OpenAI Codex (legacy). This connects quota access without installing Codex CLI or changing your model. Then run /quota."
          : formatQuota(state),
        state.error ? "warning" : "info",
      );
    },
  });
}
