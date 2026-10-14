export function formatTokens(value) {
  if (value < 1000) return String(Math.round(value));
  if (value < 1_000_000) return `${Math.round(value / 1000)}k`;
  return `${(value / 1_000_000).toFixed(1).replace(/\.0$/, "")}M`;
}

// Session-local usage comes from Pi's active branch, never an external CLI.
export function sessionUsage(branch) {
  const total = { input: 0, output: 0, cost: 0 };
  const count = (value) =>
    typeof value === "number" && Number.isFinite(value) && value >= 0
      ? value
      : 0;
  for (const entry of branch) {
    if (entry.type !== "message" || entry.message?.role !== "assistant")
      continue;
    const usage = entry.message.usage;
    total.input += count(usage?.input);
    total.output += count(usage?.output);
    total.cost += count(usage?.cost?.total);
  }
  return total;
}
