// Сохраняет JSON-протокол Abide и направляет компиляцию через proxy-обёртку.
import { spawn } from "node:child_process";
import path from "node:path";

const [hook, ...args] = process.argv.slice(2);
let finished = false;
const notice = (message) => JSON.stringify({ systemMessage: `Abide: ${message}. This check was skipped.` });
const finish = (output) => {
  if (finished) return;
  finished = true;
  if (output) process.stdout.write(output + "\n");
};
const child = spawn(process.execPath, [hook, ...args], { stdio: ["pipe", "pipe", "inherit"] });
const chunks = [];
const budget = { "session-start": 8000, "turn-start": 8000, "post-tool-use": 18000, stop: 28000 }[args[0]] ?? 8000;
const timer = setTimeout(() => {
  child.kill();
  finish(notice("hook timed out"));
}, budget);
child.stdin.on("error", () => {});
process.stdin.pipe(child.stdin);
child.stdout.on("data", (chunk) => chunks.push(chunk));
child.on("error", () => {
  clearTimeout(timer);
  finish(notice("hook could not start"));
});
child.on("close", (code) => {
  clearTimeout(timer);
  if (code !== 0) {
    finish(notice("hook process failed"));
    return;
  }
  const output = Buffer.concat(chunks).toString("utf8").trim();
  if (!output) return;
  try {
    const parsed = JSON.parse(output);
    const context = parsed?.hookSpecificOutput?.additionalContext;
    if (typeof context === "string") {
      const invocation = `node ${JSON.stringify(path.join(path.dirname(hook), "bin.js"))} <command>`;
      parsed.hookSpecificOutput.additionalContext = context.replaceAll(invocation, '"$HOME/.local/bin/abide" <command>');
      if (context.startsWith("Abide is installed here and its rubric is missing or out of date.")) {
        parsed.hookSpecificOutput.additionalContext += "\n\nUse the abide-compile skill to write every listed JSON rubric first. Only then run rubric validate and calibrate; validation does not create a rubric from instructions. Project: abide rubric validate. Global: abide rubric validate --global.";
      }
    }
    finish(JSON.stringify(parsed));
  } catch {
    finish(notice("hook returned invalid JSON"));
  }
});
