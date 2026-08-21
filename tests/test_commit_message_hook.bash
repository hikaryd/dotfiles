#!/usr/bin/env bash

set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
hook="$repo_root/config/git/hooks/prepare-commit-msg"
ai_commit="$repo_root/scripts/ai_commit"
test_root=$(mktemp -d)
trap 'rm -rf "$test_root"' EXIT

mkdir -p "$test_root/home"

cat >"$test_root/ai-helper" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" >"$AI_HELPER_ARGS_LOG"
if [[ "${AI_HELPER_FAIL:-0}" == "1" ]]; then
  printf 'synthetic provider failure\n' >&2
  exit 7
fi
printf 'fix: generated message\n\n- generated body\n'
EOF
chmod +x "$test_root/ai-helper"

run_hook() {
  local source="$1"
  local initial="$2"
  local skip="${3:-0}"
  local message_file="$test_root/message"

  printf '%s\n' "$initial" >"$message_file"
  : >"$test_root/helper-args"
  HOME="$test_root/home" \
    AI_HELPER_SKIP_COMMIT="$skip" \
    AI_HELPER_BIN="$test_root/ai-helper" \
    AI_HELPER_ARGS_LOG="$test_root/helper-args" \
    bash "$hook" "$message_file" "$source"
}

run_hook message "manual message"
grep -Fxq 'manual message' "$test_root/message"
[[ ! -s "$test_root/helper-args" ]]
printf '%s\n' 'ok: explicit git commit -m message is preserved'

run_hook "" ""
grep -Fxq 'fix: generated message' "$test_root/message"
grep -Fxq -- '--provider' "$test_root/helper-args"
grep -Fxq -- 'openai-compatible' "$test_root/helper-args"
printf '%s\n' 'ok: editor-based git commit invokes AI generation'

run_hook "" "manual message" 1
grep -Fxq 'manual message' "$test_root/message"
[[ ! -s "$test_root/helper-args" ]]
printf '%s\n' 'ok: explicit skip keeps LazyGit non-AI commit path'

mkdir -p "$test_root/ticket-bin"
cat >"$test_root/ticket-bin/git" <<'EOF'
#!/usr/bin/env bash
case "$1" in
rev-parse)
  printf '%s\n' 'feature/3631875_autopub'
  ;;
config)
  exit 1
  ;;
log)
  printf '%s\n' 'TASK-42 fix: historical example'
  ;;
*)
  exit 2
  ;;
esac
EOF
chmod +x "$test_root/ticket-bin/git"
printf '%s\n' 'fix: generated message' >"$test_root/message"
PATH="$test_root/ticket-bin:$PATH" \
  AI_HELPER_SKIP_COMMIT=1 \
  bash "$hook" "$test_root/message" message
grep -Fxq 'TASK-3631875 fix: generated message' "$test_root/message"
printf '%s\n' 'ok: numeric branch names recover the repository ticket prefix'

export AI_HELPER_FAIL=1
run_hook "" "manual fallback"
grep -Fxq 'manual fallback' "$test_root/message"
grep -Fq '# AI commit generation failed; details:' "$test_root/message"
grep -Fxq 'synthetic provider failure' "$test_root/home/.local/state/ai-helper/last-error.log"
[[ "$(stat -f '%Lp' "$test_root/home/.local/state/ai-helper/last-error.log")" == "600" ]]
printf '%s\n' 'ok: provider failures stay non-blocking and expose a private diagnostic log'

unset AI_HELPER_FAIL
cat >"$test_root/fake-git" <<'EOF'
#!/usr/bin/env bash
case "$1" in
diff)
  [[ "${GIT_HAS_STAGED:-0}" == "1" ]] && exit 1
  exit 0
  ;;
commit)
  printf '%s\n' "$@" >"$GIT_ARGS_LOG"
  while (($#)); do
    if [[ "$1" == "--file" ]]; then
      cp "$2" "$GIT_MESSAGE_LOG"
      break
    fi
    shift
  done
  ;;
*)
  exit 2
  ;;
esac
EOF
chmod +x "$test_root/fake-git"

: >"$test_root/helper-args"
: >"$test_root/git-args"
HOME="$test_root/home" \
  GIT_HAS_STAGED=1 \
  GIT_BIN="$test_root/fake-git" \
  GIT_ARGS_LOG="$test_root/git-args" \
  GIT_MESSAGE_LOG="$test_root/git-message" \
  AI_HELPER_BIN="$test_root/ai-helper" \
  AI_HELPER_ARGS_LOG="$test_root/helper-args" \
  bash "$ai_commit"
grep -Fxq 'fix: generated message' "$test_root/git-message"
grep -Fxq -- '--edit' "$test_root/git-args"
grep -Fxq -- '--file' "$test_root/git-args"
printf '%s\n' 'ok: LazyGit wrapper generates a message before opening git commit'

: >"$test_root/git-args"
export AI_HELPER_FAIL=1
if HOME="$test_root/home" \
  GIT_HAS_STAGED=1 \
  GIT_BIN="$test_root/fake-git" \
  GIT_ARGS_LOG="$test_root/git-args" \
  GIT_MESSAGE_LOG="$test_root/git-message" \
  AI_HELPER_BIN="$test_root/ai-helper" \
  AI_HELPER_ARGS_LOG="$test_root/helper-args" \
  bash "$ai_commit" >/dev/null 2>&1; then
  printf 'wrapper unexpectedly succeeded after provider failure\n' >&2
  exit 1
fi
[[ ! -s "$test_root/git-args" ]]
printf '%s\n' 'ok: LazyGit wrapper never opens an empty commit after provider failure'
