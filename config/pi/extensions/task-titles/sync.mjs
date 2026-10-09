const ANSI = /\x1b(?:\][^\x07\x1b]*(?:\x07|\x1b\\)|\[[0-?]*[ -/]*[@-~])/g;

export function cleanTitle(value) {
  const text = value
    .replace(ANSI, "")
    .replace(/\p{C}/gu, "")
    .replace(/\s+/g, " ")
    .trim();
  const chars = Array.from(text);
  if (chars.length <= 32) return text;
  let prefix = chars.slice(0, 31).join("");
  const space = prefix.lastIndexOf(" ");
  if (space >= 0 && Array.from(prefix.slice(0, space)).length >= 16)
    prefix = prefix.slice(0, space);
  return prefix.trimEnd() + "…";
}

export function installTitleSync(pi, helper, environment = process.env) {
  const pane = environment.HERDR_PANE_ID;
  const socket = environment.HERDR_SOCKET_PATH;
  let pending = Promise.resolve(true);
  let last;
  let warned = false;

  function sync(ctx, name = pi.getSessionName()) {
    if (ctx.mode !== "tui" || !pane || !socket) return Promise.resolve(false);
    // Capture plain values now: session replacement invalidates the old context.
    const session = ctx.sessionManager.getSessionId();
    const title = name ?? "";
    const key = JSON.stringify([session, title]);
    const args = [
      "set",
      "--agent",
      "pi",
      "--pane",
      pane,
      "--session-id",
      session,
      "--title",
      title,
    ];
    pending = pending.then(async () => {
      if (last === key) return true;
      try {
        const result = await pi.exec(helper, args, { timeout: 20000 });
        if (result.code !== 0 || result.killed)
          throw new Error("Title sync failed");
        last = key;
        warned = false;
        return true;
      } catch {
        // Never expose task text, socket paths or subprocess stderr in diagnostics.
        if (!warned) {
          ctx.ui.notify(
            "Herdr: имя сессии сохранено, но подпись не обновилась. Повторю после ответа.",
            "warning",
          );
          warned = true;
        }
        return false;
      }
    });
    return pending;
  }

  pi.on("session_start", (_event, ctx) => sync(ctx));
  pi.on("session_info_changed", (event, ctx) => sync(ctx, event.name ?? ""));
  pi.on("agent_end", (_event, ctx) => sync(ctx));
  pi.on("session_shutdown", (event, ctx) => {
    // Reload/switch immediately republishes the next session's name; avoid flicker.
    return event.reason === "quit" ? sync(ctx, "") : pending;
  });
  pi.on("before_agent_start", (event, ctx) => {
    if (
      ctx.mode !== "tui" ||
      !pane ||
      !socket ||
      !pi.getActiveTools().includes("set_session_title")
    )
      return;
    event.systemPromptOptions.sections.herdr_task_title =
      "Herdr task title: after understanding the task, use set_session_title to give this Pi session " +
      "a concise Russian title: concrete subject/problem first, a known ticket ID at the end if it fits; " +
      "2–6 words, at most 32 characters. Examples: 'Переоткрытие заявки · 3812844', " +
      "'НСИ · Ложное увольнение', 'СБ · Создание проверки'. " +
      "Keep an established title for follow-ups such as testing, creating an MR or continuing work; " +
      "update it only when the actual problem becomes clear or the user changes the subject. " +
      "Respect a title explicitly chosen by the user with /name. Generic task verbs alone are not a title. " +
      "Use only this conversation, omit personal data and secrets. Only the main agent sets the title; " +
      "do not delegate it to subagents. This is optional presentation: failures must not block the task. " +
      "Current session name: " +
      JSON.stringify(pi.getSessionName() ?? null);
  });

  return {
    async setTitle(value, ctx) {
      if (ctx.mode !== "tui")
        throw new Error(
          "Session titles are only set by the interactive main agent.",
        );
      const title = cleanTitle(value);
      if (!title) throw new Error("Session title must not be empty.");
      pi.setSessionName(title);
      const synced = await sync(ctx, title);
      return { title, synced };
    },
  };
}
