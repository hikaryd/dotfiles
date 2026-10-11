#!/bin/zsh -f
# Isolated contracts: no real pytest, installs, private config or CPU-heavy work.
set -eu
set -o pipefail
root=${0:A:h:h}
work=$(mktemp -d "${TMPDIR:-/tmp}/dots-pytest-safe-test.XXXXXX")
trap 'rm -rf -- "$work"' EXIT
mkdir -p "$work/bin"
export PYTEST_SAFE_LOG="$work/calls"

cat > "$work/bin/taskpolicy" <<'SH'
#!/bin/sh
printf 'taskpolicy=<%s>\n' "$1" >> "$PYTEST_SAFE_LOG"
[ "$1" = -b ] || exit 91
shift
exec "$@"
SH
cat > "$work/bin/nice" <<'SH'
#!/bin/sh
printf 'nice=<%s> <%s>\n' "$1" "$2" >> "$PYTEST_SAFE_LOG"
[ "$1" = -n ] && [ "$2" = 10 ] || exit 92
shift 2
exec "$@"
SH
cat > "$work/bin/pytest" <<'SH'
#!/bin/sh
printf 'command=<%s>\nworkers=<%s>\n' "${0##*/}" "$PYTEST_XDIST_AUTO_NUM_WORKERS"
printf 'arg=<%s>\n' "$@"
exit "${PYTEST_SAFE_TEST_STATUS:-0}"
SH
chmod +x "$work/bin/"*
ln -s pytest "$work/bin/uv"
ln -s pytest "$work/bin/python"
path=("$work/bin" $path)
fpath=("$root/config/zsh/functions" $fpath)
autoload -Uz pytest-safe

check() {
  if ! "$@" > /dev/null; then
    print -u2 -r -- "pytest-safe assertion failed: $*"
    exit 1
  fi
}
expect_status() {
  local wanted=$1 actual=0
  shift
  : > "$PYTEST_SAFE_LOG"
  "$@" > "$work/output" 2>&1 || actual=$?
  if [[ $actual != $wanted ]]; then
    print -u2 -- "pytest-safe exit $actual, expected $wanted"
    /bin/cat "$work/output" >&2
    exit 1
  fi
}

unset PYTEST_XDIST_AUTO_NUM_WORKERS
source "$root/config/zsh/.zshenv"
check test "$PYTEST_XDIST_AUTO_NUM_WORKERS" = 2
check zsh -f -c '[[ $PYTEST_XDIST_AUTO_NUM_WORKERS == 2 ]]'
export PYTEST_XDIST_AUTO_NUM_WORKERS=4
source "$root/config/zsh/.zshenv"
check test "$PYTEST_XDIST_AUTO_NUM_WORKERS" = 4
print 'PASS global xdist default is inherited and explicit overrides survive'

unset PYTEST_XDIST_AUTO_NUM_WORKERS
expect_status 0 pytest-safe 'tests/test with spaces.py' -q
check grep -Fx 'taskpolicy=<-b>' "$PYTEST_SAFE_LOG"
check grep -Fx 'nice=<-n> <10>' "$PYTEST_SAFE_LOG"
check grep -Fx 'command=<pytest>' "$work/output"
check grep -Fx 'workers=<2>' "$work/output"
check grep -Fx 'arg=<tests/test with spaces.py>' "$work/output"
check grep -Fx 'arg=<-q>' "$work/output"
check test -z "${PYTEST_XDIST_AUTO_NUM_WORKERS+x}"
print 'PASS default pytest uses background policy, preserves arguments and scopes its environment'

expect_status 0 pytest-safe -- uv run pytest -n auto 'tests/test with spaces.py'
check grep -Fx 'command=<uv>' "$work/output"
check grep -Fx 'arg=<run>' "$work/output"
check grep -Fx 'arg=<pytest>' "$work/output"
check grep -Fx 'arg=<-n>' "$work/output"
check grep -Fx 'arg=<auto>' "$work/output"
check grep -Fx 'arg=<tests/test with spaces.py>' "$work/output"
print 'PASS explicit uv command is forwarded without shell re-parsing'

export PYTEST_XDIST_AUTO_NUM_WORKERS=3
expect_status 0 pytest-safe -- python -m pytest -q
check grep -Fx 'command=<python>' "$work/output"
check grep -Fx 'arg=<-m>' "$work/output"
check grep -Fx 'workers=<3>' "$work/output"
check test "$PYTEST_XDIST_AUTO_NUM_WORKERS" = 3
export PYTEST_SAFE_TEST_STATUS=17
expect_status 17 pytest-safe
unset PYTEST_SAFE_TEST_STATUS
expect_status 127 pytest-safe -- dots-pytest-safe-no-such-command
print 'PASS python launcher, worker overrides and test/launcher exit codes are preserved'

expect_status 0 pytest-safe --help
check grep -F 'Usage: pytest-safe' "$work/output"
check test ! -s "$PYTEST_SAFE_LOG"
expect_status 2 pytest-safe --
check grep -F 'expected a command after --' "$work/output"
check test ! -s "$PYTEST_SAFE_LOG"
print 'PASS help and invalid invocation never launch tests'

# Hide system taskpolicy as well as the fake to exercise the portable fallback.
local_path=("${path[@]}")
rm "$work/bin/taskpolicy"
path=("$work/bin")
rehash
expect_status 0 pytest-safe
path=("${local_path[@]}")
check grep -Fx 'nice=<-n> <10>' "$PYTEST_SAFE_LOG"
if grep -q 'taskpolicy=' "$PYTEST_SAFE_LOG"; then exit 1; fi
print 'PASS systems without taskpolicy still lower process priority'
