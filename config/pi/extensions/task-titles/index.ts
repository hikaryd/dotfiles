import { realpathSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { Type } from "@earendil-works/pi-ai";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { installTitleSync } from "./sync.mjs";

export default function (pi: ExtensionAPI) {
  if (!process.env.HERDR_PANE_ID || !process.env.HERDR_SOCKET_PATH) return;
  const script = resolve(
    dirname(realpathSync(fileURLToPath(import.meta.url))),
    "../../../herdr/plugins/task-titles/titles.py",
  );
  const titles = installTitleSync(pi, script);

  // Print/JSON/RPC runs (including subagents) must not rename their parent pane.
  pi.on("session_start", (_event, ctx) => {
    if (ctx.mode !== "tui") return;
    pi.registerTool({
      name: "set_session_title",
      label: "Session title",
      description:
        "Set a concise Pi session name and sync it to Herdr pane/tab titles. Presentation only; do not include secrets or personal data.",
      parameters: Type.Object({
        title: Type.String({
          description:
            "Concrete task subject, 2–6 Russian words, at most 32 characters",
        }),
      }),
      async execute(_id, params, _signal, _onUpdate, context) {
        const details = await titles.setTitle(params.title, context);
        return {
          content: [
            {
              type: "text",
              text: details.synced
                ? "Имя сессии и подписи Herdr обновлены."
                : "Имя сессии сохранено; Herdr временно недоступен.",
            },
          ],
          details,
        };
      },
    });
  });
}
