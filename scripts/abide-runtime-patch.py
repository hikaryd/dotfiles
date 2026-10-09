#!/usr/bin/env python3
"""Apply the reviewed runtime fixes to the pinned Abide package.

Only compiled JavaScript is changed. Exact old/new snippets make upgrades and
unexpected package edits fail before any file is written.
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import stat
import tempfile


VERSION = "0.0.7"

PATCHES = (
    (
        "dist/lib/credentials.js",
        "const READ_NAMES = [...KEY_NAMES, TYPESAFE_BASE_URL_ENV];",
        'const READ_NAMES = [...KEY_NAMES, "OPENROUTER_API_KEY", TYPESAFE_BASE_URL_ENV];',
    ),
    (
        "dist/lib/credentials.js",
        """const pick = (vars, from) => {
    const typesafe = vars.get(TYPESAFE_KEY_ENV);
    if (typesafe !== undefined)
        return { kind: "typesafe", key: typesafe, from };
    const gateway = vars.get(GATEWAY_KEY_ENV);
    if (gateway !== undefined)
        return { kind: "gateway", key: gateway, from };
    return { kind: "none" };
};""",
        """const directOpenRouter = (key, from) => ({
    kind: "typesafe", key, from, baseURL: "https://openrouter.ai/api/v1",
});
const pick = (vars, from) => {
    const typesafe = vars.get(TYPESAFE_KEY_ENV);
    if (typesafe !== undefined)
        return typesafe.startsWith("sk-or-")
            ? directOpenRouter(typesafe, from)
            : { kind: "typesafe", key: typesafe, from };
    const openrouter = vars.get("OPENROUTER_API_KEY");
    if (openrouter !== undefined)
        return directOpenRouter(openrouter, from);
    const gateway = vars.get(GATEWAY_KEY_ENV);
    if (gateway !== undefined)
        return gateway.startsWith("sk-or-")
            ? directOpenRouter(gateway, from)
            : { kind: "gateway", key: gateway, from };
    return { kind: "none" };
};""",
    ),
    (
        "dist/lib/credentials.js",
        'const withBaseURL = (creds, baseURL) => creds.kind === "typesafe" && baseURL !== undefined ? { ...creds, baseURL } : creds;',
        'const withBaseURL = (creds, baseURL) => creds.kind === "typesafe" && creds.baseURL === undefined && baseURL !== undefined ? { ...creds, baseURL } : creds;',
    ),
    (
        "dist/lib/credentials.js",
        'export const NO_KEY_HINT = `No API key found. Run "abide login" with your TypeSafe key, or put ${TYPESAFE_KEY_ENV} in the environment or a .env file at the repo root.`;',
        'export const NO_KEY_HINT = `No API key found. Run "abide login" with your TypeSafe or OpenRouter key, or put OPENROUTER_API_KEY / ${TYPESAFE_KEY_ENV} in the environment or a .env file at the repo root.`;',
    ),
    (
        "dist/lib/jev.js",
        """    const inputTokens = result.usage.inputTokens;
    const usage = {
        inputTokens,
        outputTokens: result.usage.outputTokens,
        costUsd: inputTokens === undefined ? undefined : inputTokens * JEV_USD_PER_INPUT_TOKEN,
    };""",
        """    const inputTokens = result.usage.inputTokens;
    // OpenRouter reports the charged USD amount in the native response body.
    const reportedCost = result.response?.body?.usage?.cost;
    const costUsd = typeof reportedCost === "number" && Number.isFinite(reportedCost) && reportedCost >= 0
        ? reportedCost
        : inputTokens === undefined ? undefined : inputTokens * JEV_USD_PER_INPUT_TOKEN;
    const usage = {
        inputTokens,
        outputTokens: result.usage.outputTokens,
        costUsd,
    };""",
    ),
    (
        "dist/commands/check.js",
        'import { runCheck } from "../lib/checkRunner.js";',
        'import { runCheck, selectRules } from "../lib/checkRunner.js";',
    ),
    (
        "dist/commands/check.js",
        """    if (!hasApiKey(root))
        throw new AbideError("NO_API_KEY", NO_KEY_HINT);
    const patch = values.diff === undefined
        ? workingTreeDiff(root, positionals)
        : readFileSync(values.diff, "utf8");
    const files = splitDiff(patch);
    const phase = values.phase;""",
        """    const phase = values.phase;
    if (phase !== "all" && phase !== "edit" && phase !== "turn")
        throw new AbideError("INVALID_PHASE", "Invalid phase: use all, edit, or turn.");
    const patch = values.diff === undefined
        ? workingTreeDiff(root, positionals)
        : readFileSync(values.diff, "utf8");
    const files = splitDiff(patch);
    const phases = phase === "all" ? ["edit", "turn"] : [phase];
    const touchedFiles = files.map((f) => f.file);
    if (files.length > 0 && !phases.some((p) => selectRules(loaded.rules, p, touchedFiles).length > 0))
        throw new AbideError("NO_MODEL_RULES", "Check skipped: no active model rules apply to the changed files in the selected phase. Compile the rubric with abide-compile, or add code rules to AGENTS.md / CLAUDE.md and recompile.");
    if (!hasApiKey(root))
        throw new AbideError("NO_API_KEY", NO_KEY_HINT);""",
    ),
    (
        "dist/commands/calibrate.js",
        'import { parseArgs } from "node:util";',
        'import { existsSync } from "node:fs";\nimport path from "node:path";\nimport { parseArgs } from "node:util";',
    ),
    (
        "dist/commands/calibrate.js",
        'import { recentHistory } from "../lib/git.js";',
        'import { isGitRepo, recentHistory } from "../lib/git.js";',
    ),
    (
        "dist/commands/calibrate.js",
        '    const history = recentHistory(repoRoot, Number(values.hunks), Number(values.commits));',
        """    if (!existsSync(path.join(repoRoot, ".git")) && !isGitRepo(repoRoot)) {
        if (values.json) {
            say(JSON.stringify({ root: repoRoot, file, skipped: "no-git-history", hunks: 0, commits: 0, calls: 0, spendUsd: 0, rows: [], weak: [], noisy: [] }));
        } else {
            await showStatic(Callout({ tone: "warn", title: "This directory has no Git history, so calibration is skipped. Rules are unchanged." }));
        }
        return 0;
    }
    const history = recentHistory(repoRoot, Number(values.hunks), Number(values.commits));""",
    ),
    (
        "dist/commands/rubric.js",
        'throw new AbideError("RUBRIC_MISSING", `${file} does not exist yet`);',
        'throw new AbideError("RUBRIC_MISSING", `${file} does not exist yet. Use the abide-compile skill to write the JSON rubric before validating.${values.global ? "" : " To validate existing global rules, run abide rubric validate --global."}`);',
    ),
    (
        "dist/hooks/turnStart.js",
        'import path from "node:path";',
        'import path from "node:path";\nimport { planCompile } from "./sessionStart.js";\nimport { compilePrompt } from "../lib/compilePrompt.js";\nimport { placeCompileSkill } from "../lib/packageRoot.js";',
    ),
    (
        "dist/hooks/turnStart.js",
        'export const handleTurnStart = async (raw) => {\n    const parsed = turnStartInputSchema.safeParse(raw);',
        'const recordTurnStart = async (raw) => {\n    const parsed = turnStartInputSchema.safeParse(raw);',
    ),
    (
        "dist/hooks/turnStart.js",
        '//# sourceMappingURL=turnStart.js.map',
        """// Recheck at each prompt: sessions opened before installation also need compilation.
export const handleTurnStart = async (raw) => {
    const output = await recordTurnStart(raw);
    const parsed = turnStartInputSchema.safeParse(raw);
    if (!parsed.success)
        return output;
    const root = findRepoRoot(parsed.data.cwd);
    const plan = planCompile(root);
    if (plan.targets.length === 0)
        return output;
    return { kind: "session-context", additionalContext: compilePrompt(placeCompileSkill(root), plan.targets) };
};
//# sourceMappingURL=turnStart.js.map""",
    ),
    (
        "dist/hooks/postToolUse.js",
        """            code: isAbideError(error) ? error.code : "CHECK_FAILED",
            message: error instanceof Error ? error.message : String(error),
            latencyMs: Math.round(performance.now() - started),
        });
        return { kind: "silent" };
    }
    for (const { edit, relative } of checked) {""",
        """            code: isAbideError(error) ? error.code : "CHECK_FAILED",
            message: error instanceof Error ? error.message : String(error),
            latencyMs: Math.round(performance.now() - started),
        });
        return {
            kind: "notice",
            systemMessage: `Abide: check failed (${isAbideError(error) ? error.code : "CHECK_FAILED"}). This check was skipped; details in .abide/events.jsonl.`,
        };
    }
    for (const { edit, relative } of checked) {""",
    ),
    (
        "dist/hooks/stop.js",
        """            code: isAbideError(error) ? error.code : "CHECK_FAILED",
            message: error instanceof Error ? error.message : String(error),
            latencyMs: Math.round(performance.now() - started),
        });
        return finish({ kind: "silent" });
    }
    const byId = new Map""",
        """            code: isAbideError(error) ? error.code : "CHECK_FAILED",
            message: error instanceof Error ? error.message : String(error),
            latencyMs: Math.round(performance.now() - started),
        });
        return finish({
            kind: "notice",
            systemMessage: `Abide: check failed (${isAbideError(error) ? error.code : "CHECK_FAILED"}). This check was skipped; details in .abide/events.jsonl.`,
        });
    }
    const byId = new Map""",
    ),
)


def _replace_once(text: str, before: str, after: str, path: Path) -> str:
    old_count, new_count = text.count(before), text.count(after)
    # Import and footer patches may retain the old snippet inside the new one.
    if new_count == 1 and text.replace(after, "", 1).count(before) == 0:
        return text
    if old_count == 1 and new_count == 0:
        return text.replace(before, after, 1)
    raise RuntimeError(f"Unrecognized Abide runtime in {path}; no patches applied")


def _regular_file(path: Path) -> os.stat_result:
    metadata = path.lstat()
    if not stat.S_ISREG(metadata.st_mode):
        raise RuntimeError(f"Refusing to patch non-regular file: {path}")
    return metadata


def _atomic_write(path: Path, text: str, mode: int) -> None:
    fd, temporary = tempfile.mkstemp(prefix=f".{path.name}.dots-", dir=path.parent)
    temporary_path = Path(temporary)
    try:
        os.fchmod(fd, stat.S_IMODE(mode))
        with os.fdopen(fd, "w", encoding="utf-8", newline="") as stream:
            stream.write(text)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary_path, path)
    finally:
        temporary_path.unlink(missing_ok=True)


def apply(package: Path) -> list[Path]:
    """Patch Abide 0.0.7 in place, returning only the files that changed.

    All snippets are checked before writing. A second call performs no writes.
    The installer may import this function or invoke this script with the
    absolute @coldtea/abide package directory as its only argument.
    """
    package = Path(package)
    manifest = package / "package.json"
    _regular_file(manifest)
    metadata = json.loads(manifest.read_text(encoding="utf-8"))
    if metadata.get("name") != "@coldtea/abide" or metadata.get("version") != VERSION:
        raise RuntimeError(f"Runtime fixes require @coldtea/abide {VERSION}")

    originals: dict[Path, str] = {}
    updated: dict[Path, str] = {}
    modes: dict[Path, int] = {}
    for relative, before, after in PATCHES:
        path = package / relative
        if path not in originals:
            modes[path] = _regular_file(path).st_mode
            originals[path] = path.read_text(encoding="utf-8")
            updated[path] = originals[path]
        updated[path] = _replace_once(updated[path], before, after, path)

    changed = [path for path, text in updated.items() if text != originals[path]]
    for path in changed:
        _atomic_write(path, updated[path], modes[path])
    return changed


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("package", type=Path)
    arguments = parser.parse_args()
    changed_files = apply(arguments.package)
    print(f"Abide runtime fixes: {len(changed_files)} files updated")
