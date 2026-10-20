import test from "node:test";
import assert from "node:assert/strict";
import { readStatusInTab } from "../status-reader.js";

// Contract observed in the real Kaiten web client: /api/cards/:id uses the
// browser session; the versioned public API is not its authenticated endpoint.
for (const background of [false, true]) test(`Kaiten web session reads cards without a public API token (${background ? "worker" : "tab"})`, async t => {
  const old = Object.getOwnPropertyDescriptor(globalThis, "location");
  Object.defineProperty(globalThis, "location", { configurable: true, value: background
    ? { protocol: "chrome-extension:", origin: "chrome-extension://test" }
    : { origin: "https://kaiten.x5.ru" } });
  t.after(() => { if (old) Object.defineProperty(globalThis, "location", old); else delete globalThis.location; });
  const paths = [];
  t.mock.method(globalThis, "fetch", async (url, options) => {
    const path = new URL(url, "https://kaiten.x5.ru").pathname;
    paths.push(path);
    assert.equal(options.method, "GET");
    assert.deepEqual(options.headers, { Accept: "application/json" });
    return Response.json(path === "/api/cards/99"
      ? { id: 99, state: 2, column: { title: "Разработка" }, members: [{ type: 2, full_name: "Исполнитель" }] }
      : { error: "Public API requires different authentication" }, { status: path === "/api/cards/99" ? 200 : 401 });
  });
  const result = await readStatusInTab({ kind: "kaiten", origin: "https://kaiten.x5.ru", id: "99" }, background);
  assert.equal(result.data?.column, "Разработка", `the web session works, but the plugin reports ${result.error?.code}`);
  assert.deepEqual(result.data.assignees, ["Исполнитель"]);
  assert.deepEqual(paths, ["/api/cards/99"]);
});
