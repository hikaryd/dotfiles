const COMPLETED = new Set(["stop", "length", "toolUse"]);

export function responseThroughput(message) {
  if (message?.role !== "assistant" || !COMPLETED.has(message.stopReason)) {
    return undefined;
  }
  const output = message.usage?.output;
  const duration = message.durationMs;
  if (
    typeof output !== "number" ||
    !Number.isFinite(output) ||
    output < 0 ||
    typeof duration !== "number" ||
    !Number.isFinite(duration) ||
    duration <= 0 ||
    typeof message.provider !== "string" ||
    typeof message.model !== "string"
  ) {
    return undefined;
  }
  // Output already includes reasoning tokens. Provider response time excludes tool execution.
  const tps = output / (duration / 1000);
  if (!Number.isFinite(tps)) return undefined;
  return { provider: message.provider, model: message.model, tps };
}

export function latestThroughput(entries) {
  for (let index = entries.length - 1; index >= 0; index--) {
    const entry = entries[index];
    if (
      entry.type === "message" &&
      entry.message?.role === "assistant" &&
      COMPLETED.has(entry.message.stopReason)
    ) {
      // Don't replace missing timing on the latest response with an older measurement.
      return responseThroughput(entry.message);
    }
  }
  return undefined;
}
