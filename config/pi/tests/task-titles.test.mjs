import assert from "node:assert/strict";
import test from "node:test";
import {
  cleanTitle,
  installTitleSync,
} from "../extensions/task-titles/sync.mjs";

function fixture({ mode = "tui", environment, exec } = {}) {
  const handlers = new Map();
  const calls = [];
  const notices = [];
  let name;
  let session = "pi-session-one";
  const ctx = {
    mode,
    sessionManager: { getSessionId: () => session },
    ui: { notify: (...args) => notices.push(args) },
  };
  const pi = {
    on: (event, handler) => handlers.set(event, handler),
    getSessionName: () => name,
    getActiveTools: () => ["read", "set_session_title"],
    setSessionName: (value) => {
      name = value;
      void handlers.get("session_info_changed")({ name }, ctx);
    },
    exec: async (...args) => {
      calls.push(args);
      return exec ? exec(...args) : { code: 0, killed: false };
    },
  };
  const titles = installTitleSync(
    pi,
    "/dots/titles.py",
    environment ?? {
      HERDR_PANE_ID: "w1:p1",
      HERDR_SOCKET_PATH: "/herdr/session.sock",
    },
  );
  return {
    pi,
    ctx,
    titles,
    calls,
    notices,
    emit: (event, payload = {}) => handlers.get(event)(payload, ctx),
    name: (value) => {
      name = value;
    },
    session: (value) => {
      session = value;
    },
  };
}

const titleOf = (call) => call[1][call[1].indexOf("--title") + 1];
const sessionOf = (call) => call[1][call[1].indexOf("--session-id") + 1];

test("startup/resume publishes the real session name to the explicit pane", async () => {
  const f = fixture();
  f.name("Поиск API | UI");
  await f.emit("session_start");
  assert.deepEqual(f.calls, [
    [
      "python3",
      [
        "/dots/titles.py",
        "set",
        "--agent",
        "pi",
        "--pane",
        "w1:p1",
        "--session-id",
        "pi-session-one",
        "--title",
        "Поиск API | UI",
      ],
      { timeout: 20000 },
    ],
  ]);
  await f.emit("agent_end");
  assert.equal(f.calls.length, 1);
});

test("unnamed/new sessions and clearing /name mask stale titles", async () => {
  const f = fixture();
  await f.emit("session_start");
  assert.equal(titleOf(f.calls[0]), "");
  f.name("Старое имя");
  await f.emit("session_info_changed", { name: "Старое имя" });
  await f.emit("session_shutdown", { reason: "new" });
  f.session("pi-session-two");
  f.name(undefined);
  await f.emit("session_start");
  assert.equal(titleOf(f.calls.at(-1)), "");
  assert.equal(sessionOf(f.calls.at(-1)), "pi-session-two");
  f.name("Другое имя");
  await f.emit("session_info_changed", { name: "Другое имя" });
  // The event is authoritative even if a caller's getter is stale.
  await f.emit("session_info_changed", { name: undefined });
  assert.equal(titleOf(f.calls.at(-1)), "");
});

test("rapid renames are serialized and shell metacharacters stay in one argument", async () => {
  let active = 0;
  const f = fixture({
    exec: async () => {
      assert.equal(active++, 0);
      await new Promise((resolve) => setImmediate(resolve));
      active--;
      return { code: 0, killed: false };
    },
  });
  const names = [
    "Первое имя",
    "API 'quoted'; $(touch /tmp/nope)",
    "Последнее имя",
  ];
  await Promise.all(
    names.map((name) => f.emit("session_info_changed", { name })),
  );
  assert.deepEqual(f.calls.map(titleOf), names);
});

test("quit clears presentation, but reload/fork/resume do not clear it", async () => {
  const f = fixture();
  f.name("Название задачи");
  await f.emit("session_start");
  for (const reason of ["reload", "fork", "resume"]) {
    await f.emit("session_shutdown", { reason });
  }
  assert.equal(f.calls.length, 1);
  await f.emit("session_shutdown", { reason: "quit" });
  assert.equal(titleOf(f.calls.at(-1)), "");
});

test("optional Herdr failures are reported once and retried after the answer", async () => {
  let healthy = false;
  const f = fixture({
    exec: () =>
      healthy
        ? { code: 0, killed: false }
        : { code: 1, stderr: "private task text" },
  });
  f.name("Сохранённое имя");
  assert.equal(await f.emit("session_start"), false);
  assert.equal(await f.emit("agent_end"), false);
  assert.equal(f.notices.length, 1);
  assert.ok(!f.notices[0][0].includes("private"));
  healthy = true;
  assert.equal(await f.emit("agent_end"), true);
  assert.equal(f.pi.getSessionName(), "Сохранённое имя");
  assert.equal(f.calls.length, 3);
});

test("tool saves the Pi session name, sanitizes it and deduplicates the rename event", async () => {
  const f = fixture();
  const result = await f.titles.setTitle(
    "\x1b[31mПоиск\x1b[0m   API\u202e",
    f.ctx,
  );
  assert.equal(f.pi.getSessionName(), "Поиск API");
  assert.deepEqual(result, { title: "Поиск API", synced: true });
  assert.equal(f.calls.length, 1);
  await assert.rejects(f.titles.setTitle(" \u202e ", f.ctx), /empty/);
  assert.equal(f.pi.getSessionName(), "Поиск API");
});

test("failed optional sync still preserves the name set by the tool", async () => {
  const f = fixture({
    exec: () => {
      throw new Error("unavailable");
    },
  });
  const result = await f.titles.setTitle("Поиск API", f.ctx);
  assert.deepEqual(result, { title: "Поиск API", synced: false });
  assert.equal(f.pi.getSessionName(), "Поиск API");
});

test("prompt adds naming guidance without replacing existing prompt sections", () => {
  const f = fixture();
  f.name("Уже выбранная тема");
  const event = {
    systemPromptOptions: { sections: { existing: "Keep this" } },
  };
  f.emit("before_agent_start", event);
  assert.equal(event.systemPromptOptions.sections.existing, "Keep this");
  assert.match(
    event.systemPromptOptions.sections.herdr_task_title,
    /set_session_title/,
  );
  assert.match(
    event.systemPromptOptions.sections.herdr_task_title,
    /Уже выбранная тема/,
  );
  assert.match(event.systemPromptOptions.sections.herdr_task_title, /\/name/);
});

test("non-terminal modes and missing Herdr env never touch the parent pane", async () => {
  for (const options of [
    { mode: "print" },
    { mode: "json" },
    { mode: "rpc" },
    { environment: {} },
    { environment: { HERDR_PANE_ID: "w1:p1" } },
  ]) {
    const f = fixture(options);
    f.name("Не менять родителя");
    await f.emit("session_start");
    await f.emit("session_info_changed", { name: f.pi.getSessionName() });
    await f.emit("agent_end");
    await f.emit("session_shutdown", { reason: "quit" });
    const event = { systemPromptOptions: { sections: {} } };
    f.emit("before_agent_start", event);
    assert.deepEqual(event.systemPromptOptions.sections, {});
    assert.deepEqual(f.calls, []);
    if (options.mode)
      await assert.rejects(f.titles.setTitle("Не менять", f.ctx), /main agent/);
  }
});

test("title limit counts Unicode characters and keeps word boundaries", () => {
  assert.equal(cleanTitle("a".repeat(40)), "a".repeat(31) + "…");
  assert.equal(cleanTitle("😀".repeat(40)), "😀".repeat(31) + "…");
  const title = cleanTitle(
    "Очень длинное название задачи которое нужно обрезать",
  );
  assert.ok(Array.from(title).length <= 32);
  assert.equal(title, "Очень длинное название задачи…");
});
