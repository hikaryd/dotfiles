#!/usr/bin/env zsh

set -eu

repo_root="${0:A:h:h}"

for legacy_helper in \
  kpdev-rollout kpstage-rollout kppreprod-rollout kpprod-rollout \
  kpdev-cron kpstage-cron kppreprod-cron kpprod-cron podbor-kube-help; do
  functions[$legacy_helper]='return 0'
done
functions[_podbor_legacy_helper]='return 0'
source "$repo_root/config/zsh/kube-tools.zsh"
original_kube_pod_restart_one="${functions[_kube_pod_restart_one]}"

for legacy_helper in \
  kpdev-rollout kpstage-rollout kppreprod-rollout kpprod-rollout \
  kpdev-cron kpstage-cron kppreprod-cron kpprod-cron podbor-kube-help \
  _podbor_legacy_helper; do
  (( ! $+functions[$legacy_helper] )) || {
    print -u2 -r -- "expected stale helper to be removed on re-source: $legacy_helper"
    exit 1
  }
done
unset legacy_helper

print -r -- "ok: re-source removes stale action helper names"

unset KPROD_CONFIRMED
if prod_confirm_output="$(print -r -- no | _kube_prod_confirm 2>&1)"; then
  print -u2 -r -- "expected an unconfirmed prod action to abort"
  exit 1
fi
print -r -- "$prod_confirm_output" | grep -Fq -- "Aborted." || {
  print -u2 -r -- "prod confirmation must remain usable under set -u"
  exit 1
}
unset prod_confirm_output

print -r -- "ok: prod confirmation fails closed under set -u"

if unconfigured_output="$(
  unset KUBE_DEV_KUBECONFIG KUBE_DEV_CONTEXT KUBE_DEV_NAMESPACE DOTS_PRIVATE_ZSH
  kdev get pods 2>&1
)"; then
  print -u2 -r -- "expected an unconfigured environment to fail"
  exit 1
fi
print -r -- "$unconfigured_output" | grep -Fq -- \
  "Kubernetes environment is not configured" || {
  print -u2 -r -- "expected a controlled configuration error under set -u"
  exit 1
}
unset unconfigured_output

print -r -- "ok: unconfigured wrappers fail cleanly under set -u"

test_root="$(mktemp -d)"
trap 'rm -rf -- "$test_root"' EXIT

export KUBE_DEV_KUBECONFIG="$test_root/kubeconfig"
export KUBE_DEV_CONTEXT="example-dev"
export KUBE_DEV_NAMESPACE="example-app"
export KUBE_STAGE_KUBECONFIG="$test_root/kubeconfig"
export KUBE_STAGE_CONTEXT="example-stage"
export KUBE_STAGE_NAMESPACE="example-stage-app"
export KUBE_PREPROD_KUBECONFIG="$test_root/kubeconfig"
export KUBE_PREPROD_CONTEXT="example-preprod"
export KUBE_PREPROD_NAMESPACE="example-preprod-app"
export KUBE_LOG_DIR="$test_root/logs"
export KUBE_JOB_WAIT="3s"
export KUBE_JOB_LOG_RETRY_INTERVAL="0"

: >"$KUBE_DEV_KUBECONFIG"

original_home="$HOME"
export HOME="$test_root/home"
for tool_and_source in \
  kube-log-watch:kube-log-watch \
  kube-deploy-watch:kube-rollout \
  kube-pod-analyze:kube-pod-analyze; do
  tool="${tool_and_source%%:*}"
  source_name="${tool_and_source#*:}"
  mkdir -p "$HOME/.local/bin" "$HOME/dots/tools/$source_name"
  : >"$HOME/dots/tools/$source_name/main.go"
  : >"$HOME/dots/tools/$source_name/go.mod"
  cat >"$HOME/.local/bin/$tool" <<'EOF'
#!/bin/sh
exit 0
EOF
  chmod +x "$HOME/.local/bin/$tool"
done

for resolver in \
  _kube_log_watch_binary \
  _kube_deploy_watch_binary \
  _kube_pod_analyze_binary; do
  "$resolver"
  [[ -x "${reply[1]}" ]] || {
    print -u2 -r -- "expected $resolver to reuse an installed current binary"
    exit 1
  }
done

original_path=("${path[@]}")
fake_go_bin="$test_root/fake-go-bin"
mkdir -p "$fake_go_bin"
cat >"$fake_go_bin/go" <<'EOF'
#!/bin/sh
output=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    output="$2"
    shift 2
    continue
  fi
  shift
done
[ -n "$output" ] || exit 2
printf '#!/bin/sh\nexit 0\n' >"$output"
chmod +x "$output"
: >"$HOME/fake-go-called"
EOF
chmod +x "$fake_go_bin/go"
path=("$fake_go_bin" "${path[@]}")
rehash
command rm -f -- "$HOME/.local/bin/kube-log-watch"
_kube_log_watch_binary
[[ -x "${reply[1]}" && -e "$HOME/fake-go-called" ]] || {
  print -u2 -r -- "expected a missing helper binary to use the explicit Go build fallback"
  exit 1
}
path=("${original_path[@]}")
rehash
export HOME="$original_home"
unset original_home original_path fake_go_bin tool source_name tool_and_source resolver

print -r -- "ok: helper binary resolvers reuse current installations and rebuild missing ones"

startup_home="$test_root/startup-home"
startup_calls="$test_root/startup-kubectl-calls"
mkdir -p "$startup_home/.cargo" "$startup_home/.config/dots"
ln -s "$repo_root" "$startup_home/dots"
: >"$startup_home/.cargo/env"
for environment in dev stage preprod prod; do
  : >"$test_root/$environment-kubeconfig"
done
cat >"$startup_home/.config/dots/private.zsh" <<EOF
export KUBE_DEV_KUBECONFIG="$test_root/dev-kubeconfig"
export KUBE_DEV_CONTEXT="example-dev"
export KUBE_DEV_NAMESPACE="example-dev-app"
export KUBE_STAGE_KUBECONFIG="$test_root/stage-kubeconfig"
export KUBE_STAGE_CONTEXT="example-stage"
export KUBE_STAGE_NAMESPACE="example-stage-app"
export KUBE_PREPROD_KUBECONFIG="$test_root/preprod-kubeconfig"
export KUBE_PREPROD_CONTEXT="example-preprod"
export KUBE_PREPROD_NAMESPACE="example-preprod-app"
export KUBE_PROD_KUBECONFIG="$test_root/prod-kubeconfig"
export KUBE_PROD_CONTEXT="example-prod"
export KUBE_PROD_NAMESPACE="example-prod-app"
kubectl() {
  printf '%s|%s\n' "\$KUBECONFIG" "\$*" >>"\$KUBE_TEST_CALLS"
}
EOF

HOME="$startup_home" \
ZDOTDIR="$repo_root/config/zsh" \
TERM=xterm-256color \
KUBE_TEST_CALLS="$startup_calls" \
  zsh -dfc '
    source "$ZDOTDIR/.zshrc"
    kdev get pods
    kpstage logs pod/api
    kpreprod version
    KPROD_CONFIRMED=1 kpprod scale deployment/api --replicas=2
    _kube_kubectl_is_read_only get pods
    ! _kube_kubectl_is_read_only apply -f manifest.yaml
  '

grep -Fq -- "$test_root/dev-kubeconfig|--context=example-dev get pods" "$startup_calls" || exit 1
grep -Fq -- "$test_root/stage-kubeconfig|--context=example-stage --namespace=example-stage-app logs pod/api" "$startup_calls" || exit 1
grep -Fq -- "$test_root/preprod-kubeconfig|--context=example-preprod version" "$startup_calls" || exit 1
grep -Fq -- "$test_root/prod-kubeconfig|--context=example-prod --namespace=example-prod-app scale deployment/api --replicas=2" "$startup_calls" || exit 1

print -r -- "ok: startup exposes pinned wrappers and preserves the prod read-only boundary"

_kube_choose() {
  head -n 1
}

_kube_kubectl() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  shift 3

  case "$1 $2" in
    "get cronjobs")
      print -r -- "nightly-cleanup   0 0 * * *   False   0   1h   1d"
      ;;
    "create job")
      print -r -- "job.batch/nightly-cleanup-manual created"
      ;;
    "get pods")
      print -r -- "api-abc      1/1   Running   0   1m"
      print -r -- "worker-def   1/1   Running   0   1m"
      ;;
    "get deployments,statefulsets,daemonsets")
      print -r -- "deployment/api   1/1   1   1   1m"
      ;;
    "get deployments,statefulsets")
      print -r -- $'Deployment\tapi\t2\t2\t2\t2'
      print -r -- $'StatefulSet\tworker\t1\t1\t1\t1'
      ;;
    "scale deployment/api")
      print -r -- "$*" >"$test_root/scale-call"
      print -r -- "deployment.apps/api scaled"
      ;;
    "logs -f")
      if [[ "${3:-}" = "pod/api-abc" ]]; then
        print -r -- '[pod/api-abc/api] request completed'
        return 0
      fi

      local attempts_file="$test_root/log-attempts"
      local attempts=0
      [[ -r "$attempts_file" ]] && attempts="$(<"$attempts_file")"
      (( attempts += 1 ))
      print -r -- "$attempts" >"$attempts_file"

      if (( attempts == 1 )); then
        print -u2 -r -- 'Error from server (BadRequest): container "worker" in pod "nightly-cleanup-abc" is waiting to start: ContainerCreating'
        return 1
      fi

      print -r -- '[pod/nightly-cleanup-abc/worker] job completed'
      ;;
    *)
      print -u2 -r -- "unexpected fake kubectl call: $*"
      return 64
      ;;
  esac
}

_kube_cron dev nightly-cleanup

[[ "$(<"$test_root/log-attempts")" = "2" ]] || {
  print -u2 -r -- "expected two log attempts"
  exit 1
}

log_file=("$KUBE_LOG_DIR"/*.log(N[1]))
(( ${#log_file} == 1 )) || {
  print -u2 -r -- "expected one saved log file"
  exit 1
}

grep -q 'job completed' "$log_file[1]" || {
  print -u2 -r -- "expected successful logs in $log_file[1]"
  exit 1
}

print -r -- "ok: cron-run retries transient ContainerCreating log failures"

non_transient_attempts="$test_root/non-transient-attempts"
non_transient_log="$test_root/non-transient.log"
if (
  _kube_stream_logs_to_file() {
    local log_file="$1"
    local attempts=0
    [[ -r "$non_transient_attempts" ]] && attempts="$(<"$non_transient_attempts")"
    (( attempts += 1 ))
    print -r -- "$attempts" >"$non_transient_attempts"
    print -r -- "application: waiting to start: ContainerCreating" >"$log_file"
    return 42
  }
  _kube_job_logs_to_file "$non_transient_log" ignored
); then
  print -u2 -r -- "application logs must not trigger the Kubernetes startup retry"
  exit 1
else
  non_transient_status="$?"
fi
[[ "$non_transient_status" = "42" ]] || exit 1
[[ "$(<"$non_transient_attempts")" = "1" ]] || {
  print -u2 -r -- "expected exactly one non-transient log attempt"
  exit 1
}
unset non_transient_attempts non_transient_log non_transient_status

print -r -- "ok: application text cannot trigger the Kubernetes startup retry"

logs_output="$(_kube_logs stage api)"
print -r -- "$logs_output" | grep -q 'request completed' || {
  print -u2 -r -- "expected selected pod logs"
  exit 1
}

print -r -- "ok: pod log picker forwards kubeconfig coordinates to kubectl"

_kube_env preprod
[[ "${(j:|:)reply}" = \
  "$KUBE_PREPROD_KUBECONFIG|$KUBE_PREPROD_CONTEXT|$KUBE_PREPROD_NAMESPACE|preprod" ]] || {
  print -u2 -r -- "expected preprod kube coordinates"
  exit 1
}

for helper in \
  kppreprod-logs kppreprod-logs-save kppreprod-logs-multi \
  kppreprod-cron-run kppreprod-jobs kppreprod-jobs-save \
  kppreprod-exec kppreprod-log-search kppreprod-deploy-watch \
  kppreprod-scale kppreprod-pod-analyze kppreprod-pod-restart; do
  (( $+functions[$helper] )) || {
    print -u2 -r -- "expected preprod helper: $helper"
    exit 1
  }
done

print -r -- "ok: preprod exposes the complete interactive helper set"

_kube_choose() {
  head -n 2
}

_kube_pod_restart_one() {
  print -r -- "$2" >>"$test_root/restarted-pods"
  reply=("$2-replacement")
}

_kube_follow_restarted_pods() {
  print -r -- "$*" >"$test_root/followed-replacements"
}

export KUBE_POD_RESTART_FOLLOW_LOGS=1
_kube_pod_restart stage api

[[ "$(<"$test_root/restarted-pods")" = $'api-abc\nworker-def' ]] || {
  print -u2 -r -- "expected both selected pods to restart sequentially"
  exit 1
}
[[ "$(<"$test_root/followed-replacements")" = \
  "stage api-abc-replacement worker-def-replacement" ]] || {
  print -u2 -r -- "expected replacement log follow after all restarts"
  exit 1
}

print -r -- "ok: pod restart supports multi-select and follows all replacements"

_kube_choose() {
  head -n 1
}

cat >"$test_root/fake-deploy-watch" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" >"$TEST_ROOT/deploy-watch-args"
EOF
chmod +x "$test_root/fake-deploy-watch"
export TEST_ROOT="$test_root"
export KUBE_DEPLOY_WATCH_RESOURCES_FILE="$test_root/deploy-watch.resources"
: >"$KUBE_DEPLOY_WATCH_RESOURCES_FILE"

_kube_deploy_watch_binary() {
  reply=("$test_root/fake-deploy-watch")
}

unset KUBE_DEPLOY_WATCH_REFRESH
_kube_deploy_watch stage api

grep -A1 -Fx -- '--refresh' "$test_root/deploy-watch-args" | grep -Fxq '100ms' || {
  print -u2 -r -- "expected deploy-watch to refresh every 100ms by default"
  exit 1
}

print -r -- "ok: deploy-watch defaults to a 100ms refresh interval"

_kube_scale stage api 4

grep -Fq -- 'scale deployment/api --replicas=4' "$test_root/scale-call" || {
  print -u2 -r -- "expected deployment/api to scale to four replicas"
  exit 1
}

print -r -- "ok: scale shows scalable workloads and applies an explicit replica count"

_kube_choose() {
  head -n 2
}

cat >"$test_root/fake-pod-analyze" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" >"$TEST_ROOT/pod-analyze-args"
EOF
chmod +x "$test_root/fake-pod-analyze"

_kube_pod_analyze_binary() {
  reply=("$test_root/fake-pod-analyze")
}

LINES=19
_kube_pod_analyze stage api

[[ "$(grep -c -Fx -- '--pod' "$test_root/pod-analyze-args")" = "2" ]] || {
  print -u2 -r -- "expected two selected pods to be passed to pod-analyze"
  exit 1
}
grep -Fxq -- 'api-abc' "$test_root/pod-analyze-args" || exit 1
grep -Fxq -- 'worker-def' "$test_root/pod-analyze-args" || exit 1
grep -Fxq -- '--live' "$test_root/pod-analyze-args" || {
  print -u2 -r -- "expected pod-analyze to force live dashboard mode"
  exit 1
}
grep -A1 -Fx -- '--refresh' "$test_root/pod-analyze-args" | grep -Fxq '100ms' || {
  print -u2 -r -- "expected pod-analyze to refresh every 100ms by default"
  exit 1
}
grep -A1 -Fx -- '--terminal-lines' "$test_root/pod-analyze-args" | grep -Fxq '19' || {
  print -u2 -r -- "expected pod-analyze to receive the current terminal height"
  exit 1
}
grep -A1 -Fx -- '--process-refresh' "$test_root/pod-analyze-args" | grep -Fxq '5s' || {
  print -u2 -r -- "expected /proc process samples every five seconds by default"
  exit 1
}

print -r -- "ok: pod-analyze supports multiple selected pods"

_kube_kubectl() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  shift 3

  if [[ "$1 $2" = "get pods" && "$*" = *custom-columns=NAME:* ]]; then
    print -r -- "api-abc"
    print -r -- "worker-def"
    print -r -- "api-sidecar"
    return
  fi

  if [[ "$1" = "logs" ]]; then
    print -r -- "$2" >>"$test_root/searched-pods"
    print -r -- "[$2] needle"
    return
  fi

  print -u2 -r -- "unexpected matching-test kubectl call: $*"
  return 64
}

cat >"$test_root/fake-log-watch" <<'EOF'
#!/bin/sh
printf '%s\n' "$@" >"$TEST_ROOT/log-watch-args"
EOF
chmod +x "$test_root/fake-log-watch"

_kube_log_watch_binary() {
  reply=("$test_root/fake-log-watch")
}

_kube_logs_multi stage api
[[ "$(grep -c -Fx -- '--pod' "$test_root/log-watch-args")" = "2" ]] || {
  print -u2 -r -- "expected two pod matches for logs-multi"
  exit 1
}
grep -Fxq -- 'api-abc' "$test_root/log-watch-args" || exit 1
grep -Fxq -- 'api-sidecar' "$test_root/log-watch-args" || exit 1
! grep -Fxq -- 'worker-def' "$test_root/log-watch-args" || exit 1

export KUBE_LOG_SEARCH_PREVIOUS=0
search_output="$(_kube_log_search stage api needle)"
[[ "$(wc -l <"$test_root/searched-pods" | tr -d ' ')" = "2" ]] || {
  print -u2 -r -- "expected two pod matches for log-search"
  exit 1
}
print -r -- "$search_output" | grep -Fq -- 'needle' || exit 1

print -r -- "ok: log commands share the same implicit pod glob semantics"

_kube_kubectl() {
  shift 3
  if [[ "$*" = *--previous=true* ]]; then
    return 41
  fi
  print -r -- "current container log"
}

previous_output="$(_kube_search_one_pod_logs \
  "$KUBE_STAGE_KUBECONFIG" "$KUBE_STAGE_CONTEXT" "$KUBE_STAGE_NAMESPACE" \
  api-ok "" 1)" || {
  print -u2 -r -- "optional previous logs must not hide current container logs"
  exit 1
}
[[ "$previous_output" = "current container log" ]] || exit 1

print -r -- "ok: unavailable optional previous logs preserve current results"

_kube_kubectl() {
  shift 3

  if [[ "$1 $2" = "get pods" && "$*" = *custom-columns=NAME:* ]]; then
    print -r -- "api-ok"
    print -r -- "api-fail"
    return
  fi

  if [[ "$1 $2" = "logs pod/api-ok" ]]; then
    print -r -- "[pod/api-ok] needle"
    return
  fi
  if [[ "$1 $2" = "logs pod/api-fail" ]]; then
    print -u2 -r -- "simulated kubectl logs failure"
    return 42
  fi

  print -u2 -r -- "unexpected error-test kubectl call: $*"
  return 64
}

if _kube_log_search stage api needle >/dev/null; then
  print -u2 -r -- "expected log-search to preserve a kubectl failure"
  exit 1
else
  search_status="$?"
fi
[[ "$search_status" = "42" ]] || {
  print -u2 -r -- "expected kubectl status 42, got $search_status"
  exit 1
}

export KUBE_LOG_SEARCH_PARALLEL=0
if _kube_search_pod_logs \
  "$KUBE_STAGE_KUBECONFIG" "$KUBE_STAGE_CONTEXT" "$KUBE_STAGE_NAMESPACE" \
  "" 0 api-ok; then
  print -u2 -r -- "expected invalid log-search parallelism to fail"
  exit 1
else
  search_status="$?"
fi
[[ "$search_status" = "2" ]] || exit 1
unset KUBE_LOG_SEARCH_PARALLEL

_kube_kubectl() {
  print -r -- "$*" >>"$test_root/unexpected-restart-kubectl-call"
  return 64
}
functions[_kube_pod_restart_one]="$original_kube_pod_restart_one"

export KUBE_POD_RESTART_TIMEOUT=invalid
if _kube_pod_restart_one stage api; then
  print -u2 -r -- "expected invalid restart timeout to fail"
  exit 1
else
  restart_status="$?"
fi
[[ "$restart_status" = "2" ]] || exit 1

export KUBE_POD_RESTART_TIMEOUT=300
export KUBE_POD_RESTART_POLL_INTERVAL=0
if _kube_pod_restart_one stage api; then
  print -u2 -r -- "expected invalid restart poll interval to fail"
  exit 1
else
  restart_status="$?"
fi
[[ "$restart_status" = "2" ]] || exit 1
[[ ! -e "$test_root/unexpected-restart-kubectl-call" ]] || {
  print -u2 -r -- "invalid restart settings must fail before kubectl is called"
  exit 1
}
unset KUBE_POD_RESTART_TIMEOUT KUBE_POD_RESTART_POLL_INTERVAL
unset original_kube_pod_restart_one

print -r -- "ok: invalid settings and kubectl log failures stay visible"
