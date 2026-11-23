## Adversarial review after finishing a task

This rule is the user's standing request to spawn the `adversarial-reviewer` subagent within the budget below; no further permission is needed within that budget.

- After finishing a task that changed code or config, spawn a fresh `adversarial-reviewer` before reporting done, unless the change is trivial (typo, rename, comment, formatting, a one-line change with an obvious invariant).
- Always review, even for one line: authorization, tenant or replay guards, sensitive data, salary or money semantics, durable serialization or migrations, concurrency, process or signal ownership, terminal escape sanitization, risky integration, or any uncertain invariant.
- Pass only the task, acceptance criteria and the diff reference (commit range or "uncommitted changes in <repo>"). Do not pass your reasoning or your opinion of the change; the reviewer must stay independent.
- Review budget: at most TWO reviewer launches per user task, including retries and replacement reviewers. Commits, fixups, new findings and reviewer suggestions do not reset the budget. Only an explicit user request can authorize more.
- Ask for actionable correctness, security and data-loss findings within the task's scope. Style, optional refactors and speculative improvements are not blockers.
- Fix CONFIRMED findings. Verify PLAUSIBLE findings yourself and fix them or explain why they do not apply. Stop reviewing when no actionable findings remain. Use the second launch only after a non-trivial fix to a confirmed finding, and scope it to that fix and its regressions.
- After the second launch, stop spawning reviewers. Address confirmed issues with focused local checks where possible, then report remaining issues and verification gaps. If a critical issue remains unresolved, report the task as blocked rather than safe or complete.
- In the final summary, state that the review ran and what was fixed or left open. Questions, plans and read-only work need no review.
