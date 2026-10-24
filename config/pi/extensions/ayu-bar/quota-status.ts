import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { createQuotaReader, fetchCodexQuota, formatQuota } from "./quota.mjs";

// Publish quota independently of the old Ayu footer so Powerline can display it.
export default function (pi: ExtensionAPI) {
  let reader: ReturnType<typeof createQuotaReader> | undefined;
  let dispose = () => {};

  pi.on("session_start", (_event, ctx) => {
    dispose();
    if (ctx.mode !== "tui") {
      return;
    }

    const current = createQuotaReader(
      (signal: AbortSignal) =>
        fetchCodexQuota(
          () => ctx.modelRegistry.getProviderAuth("openai-codex"),
          signal,
        ),
      () => ctx.ui.setStatus("codex-quota", formatQuota(current.getState())),
    );
    reader = current;
    const timer = setInterval(() => void current.refresh(), 60000);
    timer.unref();
    dispose = () => {
      clearInterval(timer);
      current.stop();
      if (reader === current) {
        reader = undefined;
        ctx.ui.setStatus("codex-quota", undefined);
      }
    };
    void current.refresh();
  });

  pi.on("agent_end", () => void reader?.refresh());
  pi.on("session_shutdown", () => dispose());
  pi.registerCommand("quota", {
    description: "Refresh the account-wide Codex weekly allowance",
    handler: async (_args, ctx) => {
      const current = reader;
      if (!current) {
        ctx.ui.notify("The quota bar is available in terminal mode.", "info");
        return;
      }
      await current.refresh(true);
      const state = current.getState();
      ctx.ui.notify(
        state.error === "login_required"
          ? "Run /login and choose OpenAI Codex (legacy). This connects quota access without installing Codex CLI or changing your model. Then run /quota."
          : formatQuota(state),
        state.error ? "warning" : "info",
      );
    },
  });
}
