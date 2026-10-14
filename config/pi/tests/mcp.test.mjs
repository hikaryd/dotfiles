import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const config = JSON.parse(await readFile(new URL("../mcp.json", import.meta.url), "utf8"));

test("Excalidraw uses the official HTTP endpoint through codemode without credentials", () => {
  const server = config.mcpServers.excalidraw;
  assert.equal(server.type, "http");
  assert.equal(server.url, "https://mcp.excalidraw.com/mcp");
  assert.equal(server.exposure, "codemode");
  assert.equal(server.timeout, 60);
  assert.match(server.description, /MCP Apps/);
  for (const key of ["command", "args", "env", "headers", "oauth"]) {
    assert.equal(key in server, false, `Unexpected ${key} in the remote configuration`);
  }
});
