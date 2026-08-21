# Команды Kubernetes для zsh. Source этого файла не обращается к кластеру.

# Kubeconfig — default (stage/dev) loads automatically via native zsh glob (no fork)
if [[ -d "$HOME/.kube/configs/default" ]]; then
  local -a _kc=("$HOME/.kube/configs/default"/*(.N))
  (( ${#_kc} )) && export KUBECONFIG="${(j.:.)_kc}"
  unset _kc
fi

# Kubernetes environments are configured in $DOTS_PRIVATE_ZSH.
# Wrappers always pin both kubeconfig and context, so commands cannot accidentally
# fall through to another environment from the merged default KUBECONFIG.
_kube_kubectl() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  local config_path="${DOTS_PRIVATE_ZSH:-${XDG_CONFIG_HOME:-$HOME/.config}/dots/private.zsh}"
  shift 3

  if [[ -z "$kubeconfig" || -z "$context" ]]; then
    echo "Kubernetes environment is not configured in $config_path." >&2
    echo "Start from: ~/dots/config/zsh/private.example.zsh" >&2
    return 1
  fi

  if [[ ! -r "$kubeconfig" ]]; then
    echo "Kubeconfig is not readable: $kubeconfig" >&2
    return 1
  fi

  local -a namespace_arg=()
  [[ -n "$namespace" ]] && namespace_arg=(--namespace="$namespace")

  KUBECONFIG="$kubeconfig" \
    kubectl --context="$context" "${namespace_arg[@]}" "$@"
}

_kube_prod_confirm() {
  [[ "${KPROD_CONFIRMED:-}" = "1" ]] && return 0

  printf "⚠️  PROD MUTATING COMMAND — type 'yes' to proceed: "
  local answer
  read -r answer
  [[ "$answer" = "yes" ]] || {
    echo "Aborted."
    return 1
  }
}

_kube_kubectl_is_read_only() {
  local command="${1:-}"
  local subcommand="${2:-}"

  case "$command" in
    get|describe|logs|top|events|explain|api-resources|api-versions|cluster-info|version|diff|wait)
      return 0
      ;;
    rollout)
      [[ "$subcommand" = "status" || "$subcommand" = "history" ]]
      return
      ;;
    auth)
      [[ "$subcommand" = "can-i" || "$subcommand" = "whoami" ]]
      return
      ;;
    *)
      return 1
      ;;
  esac
}

_kube_prod_confirm_if_mutating() {
  _kube_kubectl_is_read_only "$@" && return 0
  _kube_prod_confirm
}

# Cluster-wide kubectl wrappers.
kdev() {
  _kube_kubectl "${KUBE_DEV_KUBECONFIG:-}" "${KUBE_DEV_CONTEXT:-}" "" "$@"
}

kstage() {
  _kube_kubectl "${KUBE_STAGE_KUBECONFIG:-}" "${KUBE_STAGE_CONTEXT:-}" "" "$@"
}

kpreprod() {
  _kube_kubectl "${KUBE_PREPROD_KUBECONFIG:-}" "${KUBE_PREPROD_CONTEXT:-}" "" "$@"
}

kprod() {
  _kube_prod_confirm_if_mutating "$@" || return
  _kube_kubectl "${KUBE_PROD_KUBECONFIG:-}" "${KUBE_PROD_CONTEXT:-}" "" "$@"
}

# Namespace-pinned kubectl wrappers.
kpdev() {
  _kube_kubectl \
    "${KUBE_DEV_KUBECONFIG:-}" "${KUBE_DEV_CONTEXT:-}" "${KUBE_DEV_NAMESPACE:-}" "$@"
}

kpstage() {
  _kube_kubectl \
    "${KUBE_STAGE_KUBECONFIG:-}" "${KUBE_STAGE_CONTEXT:-}" "${KUBE_STAGE_NAMESPACE:-}" "$@"
}

kppreprod() {
  _kube_kubectl \
    "${KUBE_PREPROD_KUBECONFIG:-}" "${KUBE_PREPROD_CONTEXT:-}" "${KUBE_PREPROD_NAMESPACE:-}" "$@"
}

kpprod() {
  _kube_prod_confirm_if_mutating "$@" || return
  _kube_kubectl \
    "${KUBE_PROD_KUBECONFIG:-}" "${KUBE_PROD_CONTEXT:-}" "${KUBE_PROD_NAMESPACE:-}" "$@"
}

# Explicit connection checks; no network request is made until one is called.
kdev-connect() {
  kdev cluster-info
}

kstage-connect() {
  kstage cluster-info
}

kpreprod-connect() {
  kpreprod cluster-info
}

kprod-connect() {
  kprod cluster-info
}

kube-tools-help() {
  cat <<'EOF'
Kubernetes commands:
  kdev <args>       cluster-wide kubectl in the configured dev context
  kstage <args>     cluster-wide kubectl in the configured stage context
  kpreprod <args>   cluster-wide kubectl in the configured preprod context
  kprod <args>      cluster-wide prod kubectl (confirm mutations)

  kpdev <args>      kubectl in the configured dev namespace
  kpstage <args>    kubectl in the configured stage namespace
  kppreprod <args>  kubectl in the configured preprod namespace
  kpprod <args>     kubectl in the configured prod namespace

  kdev-connect      check dev connection
  kstage-connect    check stage connection
  kpreprod-connect  check preprod connection
  kprod-connect     check prod connection

  kpdev-logs [text]       choose a pod and follow all container logs
  kpdev-logs-save [text]  choose a pod and save all available logs
  kpdev-cron-run [text]   ACTION: run CronJob; save logs to ~/Documents/kube-logs
  kpdev-exec              choose target, bash/Python, and working directory
  kpdev-exec [target] -- <command>  execute an explicit command
  kpdev-log-search [pod-pattern] [text]  search all matching pod logs
  kpdev-jobs [text]       choose a recent Job and follow all of its pod logs
  kpdev-jobs-save [text]  choose a recent Job and save all pod/container logs
  kpdev-deploy-watch [text]  watch a workload rollout; read-only
  kpdev-scale [text] [count] ACTION: inspect and change Deployment/StatefulSet replicas
  kpdev-pod-analyze [text]   choose one or more pods; live status/resources/processes
  kpdev-pod-restart [text]   ACTION: delete a pod, wait for replacement, follow logs

  Replace "dev" with "stage", "preprod", or "prod" in all interactive commands.

Examples:
  kpdev get pods
  kpstage get cronjobs
  kppreprod get deployments
  kpprod get deployments
  kpdev-logs api
  kpdev-jobs-save nightly-cleanup
  kpstage-cron-run nightly-cleanup
  kpstage-exec
  kpstage-log-search 'api-*' 'trace-id'
  kpstage-jobs nightly-cleanup
  kpdev-deploy-watch api
  kpstage-scale api 4
  kpstage-pod-analyze api
  kpstage-pod-restart api

Set KPROD_CONFIRMED=1 only for a deliberately pre-confirmed prod shell.
EOF
}

# Удаляем имена из старых source-версий, чтобы action/watch границы оставались явными.
unfunction \
  kpdev-rollout kpstage-rollout kppreprod-rollout kpprod-rollout \
  kpdev-cron kpstage-cron kppreprod-cron kpprod-cron podbor-kube-help \
  2>/dev/null || true
unfunction -m '_podbor_*' 2>/dev/null || true

_kube_env() {
  local environment="$1"
  local access_mode="${2:-read}"

  case "$environment" in
    dev)
      reply=(
        "${KUBE_DEV_KUBECONFIG:-}"
        "${KUBE_DEV_CONTEXT:-}"
        "${KUBE_DEV_NAMESPACE:-}"
        "dev"
      )
      ;;
    stage)
      reply=(
        "${KUBE_STAGE_KUBECONFIG:-}"
        "${KUBE_STAGE_CONTEXT:-}"
        "${KUBE_STAGE_NAMESPACE:-}"
        "stage"
      )
      ;;
    preprod)
      reply=(
        "${KUBE_PREPROD_KUBECONFIG:-}"
        "${KUBE_PREPROD_CONTEXT:-}"
        "${KUBE_PREPROD_NAMESPACE:-}"
        "preprod"
      )
      ;;
    prod)
      if [[ "$access_mode" = "action" ]]; then
        _kube_prod_confirm || return
      fi
      reply=(
        "${KUBE_PROD_KUBECONFIG:-}"
        "${KUBE_PROD_CONTEXT:-}"
        "${KUBE_PROD_NAMESPACE:-}"
        "prod"
      )
      ;;
    *)
      echo "Unknown Kubernetes environment: $environment" >&2
      return 2
      ;;
  esac
}

_kube_choose() {
  local header="$1"
  local placeholder="$2"
  local initial_filter="${3:-}"
  local preserve_order="${4:-0}"
  local selection_mode="${5:-single}"
  local -a order_args=() selection_args=()

  (( preserve_order )) && order_args=(--no-fuzzy-sort)
  if [[ "$selection_mode" = "multi" ]]; then
    header+=" · Tab toggle · Ctrl+A select all · Enter submit"
    selection_args=(--no-limit --show-help)
  else
    selection_args=(--limit=1)
  fi

  if (( $+commands[gum] )); then
    gum filter \
      --height=16 \
      --header="$header" \
      --placeholder="$placeholder" \
      --value="$initial_filter" \
      "${order_args[@]}" \
      "${selection_args[@]}" \
      --indicator="›" \
      --prompt="  " \
      --indicator.foreground="212" \
      --match.foreground="212"
    return
  fi

  local input
  input="$(cat)"
  local -a choices=("${(@f)input}")
  local index selected token
  local -A seen=()
  if (( ! ${#choices} )); then
    return 1
  fi

  echo "$header" >&2
  for (( index = 1; index <= ${#choices}; index++ )); do
    printf "  %2d) %s\n" "$index" "${choices[index]}" >&2
  done
  if [[ "$selection_mode" = "multi" ]]; then
    read -r "selected?Choose one or more [1-${#choices}], comma-separated: " </dev/tty || return
    for token in ${(s:,:)selected}; do
      token="${token//[[:space:]]/}"
      [[ "$token" = <-> ]] || return 1
      (( token >= 1 && token <= ${#choices} )) || return 1
      [[ -n "${seen[$token]:-}" ]] && continue
      seen[$token]=1
      print -r -- "${choices[token]}"
    done
    return
  fi

  read -r "selected?Choose [1-${#choices}]: " </dev/tty || return
  [[ "$selected" = <-> ]] || return 1
  (( selected >= 1 && selected <= ${#choices} )) || return 1
  print -r -- "${choices[selected]}"
}

_kube_first_column() {
  local row="$1"
  print -r -- "${row%%[[:space:]]*}"
}

_kube_colorize_logs() {
  if [[ -n "${NO_COLOR:-}" || ! -x /usr/bin/perl ]]; then
    cat
    return
  fi
  if [[ "${KUBE_COLOR:-auto}" != "always" && ! -t 1 ]]; then
    cat
    return
  fi

  /usr/bin/perl -pe '
    BEGIN {
      $| = 1;
      $reset = "\e[0m";
      $dim = "\e[2m";
      $red = "\e[31m";
      $green = "\e[32m";
      $yellow = "\e[33m";
      $blue = "\e[34m";
      $magenta = "\e[35m";
      $cyan = "\e[36m";
    }

    s/^(\[[^]\r\n]+\])/$magenta$1$reset/;
    s/(\d{4}-\d{2}-\d{2}[T ][0-9:.+-]+(?:Z)?)/$dim$cyan$1$reset/;
    s/("[^"\r\n]+"\s*:)/$cyan$1$reset/g;
    s/\b(FATAL|CRITICAL|PANIC|ERROR|ERR|EXCEPTION|TRACEBACK)\b/$red$1$reset/ig;
    s/\b(WARN|WARNING)\b/$yellow$1$reset/ig;
    s/\b(INFO|NOTICE)\b/$green$1$reset/ig;
    s/\b(DEBUG|TRACE)\b/$dim$1$reset/ig;
    s/\b(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)\b/$blue$1$reset/g;
    s/\b(2\d\d)\b/$green$1$reset/g;
    s/\b(4\d\d)\b/$yellow$1$reset/g;
    s/\b(5\d\d)\b/$red$1$reset/g;
  '
}

_kube_stream_logs() {
  _kube_kubectl "$@" 2>&1 | _kube_colorize_logs
  local kubectl_status="${pipestatus[1]}"
  return "$kubectl_status"
}

_kube_stream_logs_to_file() {
  local log_file="$1"
  shift

  _kube_kubectl "$@" 2>&1 |
    tee "$log_file" |
    _kube_colorize_logs
  local kubectl_status="${pipestatus[1]}"
  return "$kubectl_status"
}

_kube_duration_seconds() {
  local duration="${1:-}"
  local seconds

  case "$duration" in
    <->)
      seconds="$duration"
      ;;
    <->s)
      seconds="${duration%s}"
      ;;
    <->m)
      seconds=$(( ${duration%m} * 60 ))
      ;;
    <->h)
      seconds=$(( ${duration%h} * 3600 ))
      ;;
    *)
      echo "Unsupported duration '$duration'; use seconds, Nm, or Nh." >&2
      return 2
      ;;
  esac

  reply=("$seconds")
}

_kube_job_logs_to_file() {
  local log_file="$1"
  shift
  local wait_duration="${KUBE_JOB_WAIT:-2m}"
  local retry_interval="${KUBE_JOB_LOG_RETRY_INTERVAL:-2}"
  local wait_seconds deadline logs_status

  _kube_duration_seconds "$wait_duration" || return
  wait_seconds="${reply[1]}"
  [[ "$retry_interval" = <-> ]] || {
    echo "KUBE_JOB_LOG_RETRY_INTERVAL must be a non-negative integer." >&2
    return 2
  }
  deadline=$(( SECONDS + wait_seconds ))

  while true; do
    if _kube_stream_logs_to_file "$log_file" "$@"; then
      return 0
    else
      logs_status="$?"
    fi

    grep -Eq '^Error from server \([^)]*\): container "[^"]+" in pod "[^"]+" is waiting to start: (ContainerCreating|PodInitializing)$' "$log_file" ||
      return "$logs_status"

    if (( SECONDS >= deadline )); then
      echo "Timed out after $wait_duration waiting for the Job container to start." >&2
      return "$logs_status"
    fi

    echo "Job container is still starting; retrying logs in ${retry_interval}s…" >&2
    sleep "$retry_interval"
  done
}

_kube_log_file() {
  local environment="$1"
  local resource_type="$2"
  local resource_name="$3"
  local log_dir="${KUBE_LOG_DIR:-$HOME/Documents/kube-logs}"
  local timestamp="$(date +%Y%m%d-%H%M%S)"
  local log_file="$log_dir/${environment}-${resource_type}-${resource_name}-${timestamp}.log"

  mkdir -p -m 700 "$log_dir" || {
    echo "Cannot create Kubernetes log directory: $log_dir" >&2
    return 1
  }
  (umask 077; : >"$log_file") || {
    echo "Cannot create Kubernetes log file: $log_file" >&2
    return 1
  }
  reply=("$log_file")
}

_kube_search_one_pod_logs() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  local pod="$4"
  local since="$5"
  local include_previous="$6"
  local -a since_args=()
  local current_status=0

  [[ -n "$since" ]] && since_args=(--since="$since")

  _kube_kubectl "$kubeconfig" "$context" "$namespace" \
    logs "pod/$pod" \
    --all-containers=true \
    --prefix=true \
    --timestamps=true \
    --tail=-1 \
    "${since_args[@]}" \
    2> >(
      while IFS= read -r line; do
        print -u2 -r -- "[$pod] $line"
      done
    ) || current_status="$?"

  if [[ "$include_previous" = "1" ]]; then
    _kube_kubectl "$kubeconfig" "$context" "$namespace" \
      logs "pod/$pod" \
      --all-containers=true \
      --prefix=true \
      --timestamps=true \
      --tail=-1 \
      --previous=true \
      "${since_args[@]}" \
      2>/dev/null |
      sed 's/^/[previous] /' || true
  fi

  return "$current_status"
}

_kube_search_pod_logs() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  local since="$4"
  local include_previous="$5"
  shift 5
  local -a pods=("$@")
  local -a pids=()
  local pod pid wait_status
  local parallel="${KUBE_LOG_SEARCH_PARALLEL:-6}"
  local search_status=0

  if [[ "$parallel" != <-> ]] || (( parallel == 0 )); then
    echo "KUBE_LOG_SEARCH_PARALLEL must be a positive integer." >&2
    return 2
  fi

  for pod in "${pods[@]}"; do
    _kube_search_one_pod_logs \
      "$kubeconfig" "$context" "$namespace" \
      "$pod" "$since" "$include_previous" &
    pids+=("$!")

    if (( ${#pids} >= parallel )); then
      for pid in "${pids[@]}"; do
        if wait "$pid"; then
          :
        else
          wait_status="$?"
          (( search_status == 0 )) && search_status="$wait_status"
        fi
      done
      pids=()
    fi
  done

  for pid in "${pids[@]}"; do
    if wait "$pid"; then
      :
    else
      wait_status="$?"
      (( search_status == 0 )) && search_status="$wait_status"
    fi
  done
  return "$search_status"
}

_kube_matching_pods() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  local pod_pattern="$4"
  local pod_names normalized_pattern pod
  local -a pods matched

  pod_names="$(
    _kube_kubectl "$kubeconfig" "$context" "$namespace" \
      get pods \
      -o 'custom-columns=NAME:.metadata.name' \
      --no-headers
  )" || return
  pods=("${(@f)pod_names}")

  normalized_pattern="$pod_pattern"
  if [[ "$normalized_pattern" != *'*'* &&
        "$normalized_pattern" != *'?'* &&
        "$normalized_pattern" != *'['* ]]; then
    normalized_pattern="*$normalized_pattern*"
  fi

  for pod in "${pods[@]}"; do
    [[ "$pod" = ${~normalized_pattern} ]] && matched+=("$pod")
  done
  reply=("${matched[@]}")
}

_kube_log_search() {
  local environment="$1"
  shift
  local -a kube matched
  local pod_pattern="${1:-}"
  local query="${*:2}"
  local since="${KUBE_LOG_SEARCH_SINCE:-}"
  local include_previous="${KUBE_LOG_SEARCH_PREVIOUS:-1}"
  local color_mode="always"

  _kube_env "$environment" || return
  kube=("${reply[@]}")

  if (( ! $+commands[rg] )); then
    echo "ripgrep is required for Kubernetes log search." >&2
    return 1
  fi

  if [[ -z "$pod_pattern" ]]; then
    if (( ! $+commands[gum] )); then
      read -r "pod_pattern?Pod name pattern (for example api*): " </dev/tty || return
    else
      pod_pattern="$(
        gum input \
          --header="Kubernetes ${kube[4]} · ${kube[3]} · pod name pattern" \
          --placeholder="api*" \
          --prompt="  " \
          --cursor.foreground="212"
      )" || return
    fi
  fi
  [[ -n "$pod_pattern" ]] || {
    echo "Pod name pattern cannot be empty." >&2
    return 2
  }

  if [[ -z "$query" ]]; then
    if (( ! $+commands[gum] )); then
      read -r "query?Literal log text: " </dev/tty || return
    else
      query="$(
        gum input \
          --header="Literal text to find in matching pod logs" \
          --placeholder="trace ID, candidate ID, or message fragment" \
          --prompt="  " \
          --cursor.foreground="212"
      )" || return
    fi
  fi
  [[ -n "$query" ]] || {
    echo "Search text cannot be empty." >&2
    return 2
  }

  _kube_matching_pods \
    "${kube[1]}" "${kube[2]}" "${kube[3]}" "$pod_pattern" || return
  matched=("${reply[@]}")

  (( ${#matched} )) || {
    echo "No pods match '$pod_pattern' in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  [[ -n "${NO_COLOR:-}" || ! -t 1 ]] && color_mode="never"
  echo "Searching ${#matched} pod(s) in ${kube[3]} (${kube[4]}) for literal: $query" >&2
  [[ -n "$since" ]] && echo "Log window: since $since" >&2
  [[ "$include_previous" = "1" ]] && echo "Including previous container logs." >&2

  _kube_search_pod_logs \
    "${kube[1]}" "${kube[2]}" "${kube[3]}" \
    "$since" "$include_previous" \
    "${matched[@]}" |
    rg \
      --fixed-strings \
      --line-buffered \
      --color="$color_mode" \
      -- "$query"
  local -a pipeline_status=("${pipestatus[@]}")
  local pod_logs_status="${pipeline_status[1]}"
  local search_status="${pipeline_status[2]}"

  if (( pod_logs_status != 0 )); then
    echo "One or more kubectl logs requests failed." >&2
    return "$pod_logs_status"
  fi

  if (( search_status == 1 )); then
    echo "No matching log lines found." >&2
  fi
  return "$search_status"
}

_kube_dedent() {
  /usr/bin/perl -0777 -pe '
    my @lines = split /\n/, $_, -1;
    my $margin;

    for my $line (@lines) {
      next if $line =~ /^[ \t]*$/;
      $line =~ /^([ \t]*)/;
      my $width = length($1);
      $margin = $width if !defined($margin) || $width < $margin;
    }

    if (defined($margin) && $margin > 0) {
      for my $line (@lines) {
        if ($line =~ /^[ \t]*$/) {
          $line = "";
        } else {
          $line =~ s/^[ \t]{$margin}//;
        }
      }
      $_ = join("\n", @lines);
    }
  '
}

_kube_exec_dedented() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  local target="$4"
  shift 4

  _kube_dedent |
    _kube_kubectl "$kubeconfig" "$context" "$namespace" \
      exec -i "$target" -- "$@"
  local kubectl_status="${pipestatus[2]}"
  return "$kubectl_status"
}

_kube_exec_dedented_at() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  local target="$4"
  local workdir="$5"
  shift 5

  if [[ -z "$workdir" ]]; then
    _kube_exec_dedented \
      "$kubeconfig" "$context" "$namespace" "$target" "$@"
    return
  fi

  _kube_dedent |
    _kube_kubectl "$kubeconfig" "$context" "$namespace" \
      exec -i "$target" -- \
      sh -c 'cd "$1" && shift && exec "$@"' \
      kube-exec "$workdir" "$@"
  local kubectl_status="${pipestatus[2]}"
  return "$kubectl_status"
}

_kube_choose_exec_workdir() {
  local environment="$1"
  local target="$2"
  local selection workdir

  selection="$(
    printf '%s\n' \
      "default   container WORKDIR" \
      "..        parent of container WORKDIR" \
      "../..     two levels above container WORKDIR" \
      "custom    enter a path" |
      _kube_choose \
        "Kubernetes ${environment} · $target · working directory" \
        "Choose where the command starts…"
  )" || return
  selection="$(_kube_first_column "$selection")"

  case "$selection" in
    default)
      workdir=""
      ;;
    ..|../..)
      workdir="$selection"
      ;;
    custom)
      if (( $+commands[gum] )); then
        workdir="$(
          gum input \
            --header="Working directory in $target" \
            --placeholder="/app or ../src" \
            --prompt="  " \
            --cursor.foreground="212"
        )" || return
      else
        read -r "workdir?Working directory: " </dev/tty || return
      fi
      [[ -n "$workdir" ]] || {
        echo "Working directory cannot be empty." >&2
        return 1
      }
      ;;
    *)
      echo "Unknown working directory choice: $selection" >&2
      return 2
      ;;
  esac

  reply=("$workdir")
}

_kube_exec() {
  local environment="$1"
  shift
  local -a kube command
  local target rows choice mode script workdir workdir_display
  local choose_mode=0

  _kube_env "$environment" action || return
  kube=("${reply[@]}")

  if (( ! $# )) || [[ "$1" = "--" ]]; then
    rows="$(
      _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
        get pods,deployments,statefulsets,daemonsets \
        --no-headers
    )" || return
    rows="$(print -r -- "$rows" | LC_ALL=C sort -k1,1)"
    [[ -n "$rows" ]] || {
      echo "No exec targets found in ${kube[3]} (${kube[4]})." >&2
      return 1
    }

    choice="$(
      print -r -- "$rows" |
        _kube_choose \
          "Kubernetes ${kube[4]} · ${kube[3]} · choose where to execute" \
          "Filter pods and workloads…"
    )" || return
    [[ -n "$choice" ]] || return
    target="$(_kube_first_column "$choice")"
    (( ! $# )) && choose_mode=1
  else
    target="$1"
    shift
    (( ! $# )) && choose_mode=1
  fi

  if (( choose_mode )); then
    mode="$(
      printf '%s\n' \
        "bash     interactive shell" \
        "python   paste multiline script · auto-dedent" |
        _kube_choose \
          "Kubernetes ${kube[4]} · $target · what to execute" \
          "Choose bash or python…"
    )" || return
    mode="$(_kube_first_column "$mode")"

    _kube_choose_exec_workdir "${kube[4]}" "$target" || return
    workdir="${reply[1]}"
    workdir_display="${workdir:-container WORKDIR}"

    case "$mode" in
      bash)
        if [[ -z "$workdir" ]]; then
          _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
            exec -it "$target" -- bash
        else
          _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
            exec -it "$target" -- \
            bash -lc 'cd -- "$1" && exec bash -l' \
            kube-exec "$workdir"
        fi
        ;;
      python)
        if (( ! $+commands[gum] )); then
          echo "gum is required for interactive multiline Python input." >&2
          return 1
        fi
        script="$(
          gum write \
            --height="${KUBE_EXEC_EDITOR_HEIGHT:-18}" \
            --header="Python in $target · paste code, then press Enter to run" \
            --placeholder="Paste Python here…  Ctrl+J: newline · Ctrl+E: external editor" \
            --show-line-numbers \
            --show-help \
            --cursor.foreground="212"
        )" || return
        [[ -n "$script" ]] || {
          echo "Empty Python script; nothing executed." >&2
          return 1
        }
        echo "Executing Python in $target (${kube[4]}/${kube[3]}, cwd: $workdir_display); common indentation removed." >&2
        _kube_exec_dedented_at \
          "${kube[1]}" "${kube[2]}" "${kube[3]}" "$target" "$workdir" \
          python - <<<"$script"
        ;;
      *)
        echo "Unknown exec mode: $mode" >&2
        return 2
        ;;
    esac
    return
  fi

  [[ "${1:-}" = "--" ]] || {
    echo "Expected -- before the command." >&2
    echo "Usage: kp${environment}-exec [$target] -- <command> [args...]" >&2
    return 2
  }
  shift
  command=("$@")
  (( ${#command} )) || {
    echo "Command is required after --." >&2
    return 2
  }

  if [[ -t 0 ]]; then
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      exec -it "$target" -- "${command[@]}"
    return
  fi

  echo "Executing in $target (${kube[4]}/${kube[3]}); common input indentation removed." >&2
  _kube_exec_dedented \
    "${kube[1]}" "${kube[2]}" "${kube[3]}" "$target" \
    "${command[@]}"
}

_kube_logs() {
  local environment="$1"
  local initial_filter="${2:-}"
  local mode="${3:-follow}"
  local -a kube
  local rows choice pod log_file

  _kube_env "$environment" || return
  kube=("${reply[@]}")

  rows="$(
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get pods --sort-by=.metadata.creationTimestamp --no-headers
  )" || return

  [[ -n "$rows" ]] || {
    echo "No pods found in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  choice="$(
    print -r -- "$rows" |
      _kube_choose \
        "Kubernetes ${kube[4]} · ${kube[3]} · choose a pod" \
        "Filter pods…" \
        "$initial_filter"
  )" || return
  [[ -n "$choice" ]] || return

  pod="$(_kube_first_column "$choice")"
  if [[ "$mode" = "save" ]]; then
    _kube_log_file "${kube[4]}" pod "$pod" || return
    log_file="${reply[1]}"
    echo "Saving complete available logs for pod/$pod in ${kube[3]} (${kube[4]})."
    echo "File: $log_file"
    _kube_stream_logs_to_file "$log_file" \
      "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      logs "pod/$pod" \
      --all-containers=true \
      --prefix=true \
      --timestamps=true \
      --tail=-1
    local save_status="$?"
    (( save_status == 0 )) && echo "Saved: $log_file"
    return "$save_status"
  fi

  echo "Following logs for pod/$pod in ${kube[3]} (${kube[4]}). Press Ctrl-C to stop."
  _kube_stream_logs "${kube[1]}" "${kube[2]}" "${kube[3]}" \
    logs -f "pod/$pod" \
    --all-containers=true \
    --prefix=true \
    --tail="${KUBE_LOG_TAIL:-200}"
}

# ACTION: удаляет выбранный controller-managed pod, ждёт replacement с новым UID
# и после Ready подключается к его логам.
_kube_pod_restart_one() {
  local environment="$1"
  local pod="$2"
  local -a kube
  local pod_json owner_kind owner_name owner_resource owner_json
  local selector baseline_json current_json current_rows previous_rows replacement replacement_row
  local timeout started elapsed poll_interval delete_output interactive spinner frame color_reset
  local color_dim color_blue color_yellow color_green color_red status_color
  local replacement_ready replacement_total replacement_phase replacement_restarts

  timeout="${KUBE_POD_RESTART_TIMEOUT:-300}"
  poll_interval="${KUBE_POD_RESTART_POLL_INTERVAL:-2}"
  if [[ "$timeout" != <-> ]] || (( timeout == 0 )); then
    echo "KUBE_POD_RESTART_TIMEOUT must be a positive integer." >&2
    return 2
  fi
  if [[ "$poll_interval" != <-> ]] || (( poll_interval == 0 )); then
    echo "KUBE_POD_RESTART_POLL_INTERVAL must be a positive integer." >&2
    return 2
  fi

  _kube_env "$environment" || return
  kube=("${reply[@]}")

  if (( ! $+commands[jq] )); then
    echo "jq is required to track the replacement pod." >&2
    return 1
  fi

  pod_json="$(
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get "pod/$pod" -o json
  )" || return
  owner_kind="$(print -r -- "$pod_json" | jq -r '.metadata.ownerReferences[]? | select(.controller == true) | .kind' | head -n 1)"
  owner_name="$(print -r -- "$pod_json" | jq -r '.metadata.ownerReferences[]? | select(.controller == true) | .name' | head -n 1)"

  case "$owner_kind" in
    ReplicaSet) owner_resource="replicaset/$owner_name" ;;
    StatefulSet) owner_resource="statefulset/$owner_name" ;;
    DaemonSet) owner_resource="daemonset/$owner_name" ;;
    ReplicationController) owner_resource="replicationcontroller/$owner_name" ;;
    *)
      echo "Refusing to delete pod/$pod: it has no supported restarting controller." >&2
      [[ -n "$owner_kind" ]] && echo "Controller: $owner_kind/$owner_name" >&2
      return 1
      ;;
  esac

  owner_json="$(
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get "$owner_resource" -o json
  )" || return
  selector="$(
    print -r -- "$owner_json" |
      jq -r '
        .spec.selector as $selector
        | [
            ($selector.matchLabels // {} | to_entries[] | "\(.key)=\(.value)"),
            ($selector.matchExpressions // [] | .[] |
              if .operator == "In" then "\(.key) in (\(.values | join(",")))"
              elif .operator == "NotIn" then "\(.key) notin (\(.values | join(",")))"
              elif .operator == "Exists" then .key
              elif .operator == "DoesNotExist" then "!\(.key)"
              else empty
              end)
          ]
        | join(",")
      '
  )" || return
  [[ -n "$selector" ]] || {
    echo "Cannot derive a pod selector from $owner_resource; pod was not deleted." >&2
    return 1
  }

  baseline_json="$(
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get pods -l "$selector" -o json |
      jq -c '[.items[].metadata.uid]'
  )" || return

  interactive=0
  [[ -t 1 ]] && interactive=1
  if (( interactive )) && [[ -z "${NO_COLOR:-}" ]]; then
    color_reset=$'\e[0m'
    color_dim=$'\e[2m'
    color_blue=$'\e[38;5;75m'
    color_yellow=$'\e[38;5;214m'
    color_green=$'\e[38;5;114m'
    color_red=$'\e[38;5;203m'
  fi

  printf '%sRestarting%s pod/%s %s· %s/%s%s\n' \
    "$color_blue" "$color_reset" "$pod" \
    "$color_dim" "${kube[4]}" "${kube[3]}" "$color_reset"
  [[ "${KUBE_POD_RESTART_VERBOSE:-0}" = "1" ]] && \
    printf '%sController: %s · selector: %s%s\n' \
      "$color_dim" "$owner_resource" "$selector" "$color_reset"

  delete_output="$(
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      delete "pod/$pod" --wait=false
  )" || {
    local delete_status="$?"
    print -u2 -r -- "$delete_output"
    return "$delete_status"
  }
  printf '%s✓%s Delete accepted %s%s%s\n' \
    "$color_green" "$color_reset" "$color_dim" "$owner_resource" "$color_reset"

  started="$SECONDS"
  previous_rows=""
  spinner=('⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏')
  frame=1

  while (( SECONDS - started < timeout )); do
    current_json="$(
      _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
        get pods -l "$selector" -o json
    )" || return
    current_rows="$(
      print -r -- "$current_json" |
        jq -r '
          .items
          | sort_by(.metadata.creationTimestamp)
          | .[]
          | [
              .metadata.name,
              (([.status.containerStatuses[]? | select(.ready == true)] | length) | tostring)
                + "/" + (([.status.containerStatuses[]?] | length) | tostring),
              (.status.phase // "Unknown"),
              ([.status.containerStatuses[]?.restartCount] | add // 0 | tostring)
            ]
          | @tsv
        ' |
        column -t -s $'\t'
    )" || return

    elapsed=$(( SECONDS - started ))
    replacement_row="$(
      print -r -- "$current_json" |
        jq -r --argjson baseline "$baseline_json" '
          .items
          | map(select((.metadata.uid as $uid | $baseline | index($uid)) == null))
          | sort_by(.metadata.creationTimestamp)
          | first // empty
          | [
              .metadata.name,
              ([.status.containerStatuses[]? | select(.ready == true)] | length),
              ([.status.containerStatuses[]?] | length),
              (.status.phase // "Unknown"),
              ([.status.containerStatuses[]?.restartCount] | add // 0)
            ]
          | @tsv
        '
    )" || return

    if (( interactive )); then
      if [[ -n "$replacement_row" ]]; then
        IFS=$'\t' read -r replacement replacement_ready replacement_total replacement_phase replacement_restarts \
          <<<"$replacement_row"
        case "$replacement_phase" in
          Running) status_color="$color_blue" ;;
          Pending) status_color="$color_yellow" ;;
          Failed|Unknown) status_color="$color_red" ;;
          *) status_color="$color_dim" ;;
        esac
        printf '\r\e[2K%s%s%s  %-36s %s%-10s%s %s%s/%s ready%s %s%ss%s' \
          "$color_blue" "${spinner[frame]}" "$color_reset" \
          "$replacement" "$status_color" "$replacement_phase" "$color_reset" \
          "$color_dim" "$replacement_ready" "$replacement_total" "$color_reset" \
          "$color_dim" "$elapsed" "$color_reset"
      else
        printf '\r\e[2K%s%s%s  Waiting for replacement pod… %s%ss%s' \
          "$color_blue" "${spinner[frame]}" "$color_reset" \
          "$color_dim" "$elapsed" "$color_reset"
      fi
      (( frame = frame % ${#spinner} + 1 ))
    elif [[ "$current_rows" != "$previous_rows" ]]; then
      printf '\n[%3ss] NAME  READY  STATUS  RESTARTS\n' "$elapsed"
      print -r -- "$current_rows"
      previous_rows="$current_rows"
    fi

    replacement="$(
      print -r -- "$current_json" |
        jq -r --argjson baseline "$baseline_json" '
          .items[]
          | select((.metadata.uid as $uid | $baseline | index($uid)) == null)
          | select(any(.status.conditions[]?; .type == "Ready" and .status == "True"))
          | .metadata.name
        ' |
        head -n 1
    )"
    if [[ -n "$replacement" ]]; then
      (( interactive )) && printf '\r\e[2K'
      elapsed=$(( SECONDS - started ))
      printf '%s✓%s pod/%s is Ready %s· %ss%s\n' \
        "$color_green" "$color_reset" "$replacement" \
        "$color_dim" "$elapsed" "$color_reset"
      reply=("$replacement")
      return 0
    fi

    sleep "$poll_interval"
  done

  (( interactive )) && printf '\r\e[2K'
  echo "Timed out after ${timeout}s waiting for a Ready replacement of pod/$pod." >&2
  return 1
}

_kube_follow_restarted_pods() {
  local environment="$1"
  shift
  local -a pods=("$@") kube monitor monitor_args filter_args
  local pod

  (( ${#pods} )) || return 0
  _kube_env "$environment" || return
  kube=("${reply[@]}")

  if (( ${#pods} == 1 )); then
    printf 'Following logs for pod/%s (Ctrl-C to stop)\n' "${pods[1]}"
    _kube_stream_logs "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      logs -f "pod/${pods[1]}" \
      --all-containers=true \
      --prefix=true \
      --tail="${KUBE_LOG_TAIL:-200}"
    return
  fi

  for pod in "${pods[@]}"; do
    monitor_args+=(--pod "$pod")
  done
  [[ "${KUBE_MULTI_LOG_FILTER:-0}" = "1" ]] && filter_args=(--filter)

  _kube_log_watch_binary || return
  monitor=("${reply[@]}")
  printf 'Following logs for %d replacement pods (Ctrl-C to stop)\n' "${#pods}"
  "${monitor[1]}" \
    --kubeconfig "${kube[1]}" \
    --context "${kube[2]}" \
    --namespace "${kube[3]}" \
    --environment "${kube[4]}" \
    --tail="${KUBE_LOG_TAIL:-200}" \
    --buffer="${KUBE_MULTI_LOG_BUFFER:-5000}" \
    --query "" \
    "${filter_args[@]}" \
    "${monitor_args[@]}"
}

_kube_pod_restart() {
  local environment="$1"
  local initial_filter="${2:-}"
  local -a kube selected_rows pods replacements
  local rows choice row pod index

  _kube_env "$environment" action || return
  kube=("${reply[@]}")

  rows="$(
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get pods --sort-by=.metadata.creationTimestamp --no-headers
  )" || return

  [[ -n "$rows" ]] || {
    echo "No pods found in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  choice="$(
    print -r -- "$rows" |
      _kube_choose \
        "Kubernetes ${kube[4]} · ${kube[3]} · restart pods · ACTION · Tab selects" \
        "Filter pods, select one or more…" \
        "$initial_filter" \
        0 \
        multi
  )" || return
  [[ -n "$choice" ]] || return

  selected_rows=("${(@f)choice}")
  for row in "${selected_rows[@]}"; do
    pod="$(_kube_first_column "$row")"
    if [[ -n "$pod" ]] && (( ! ${pods[(Ie)$pod]} )); then
      pods+=("$pod")
    fi
  done
  (( ${#pods} )) || return

  printf 'Selected %d pod(s); restarting sequentially.\n' "${#pods}"
  for (( index = 1; index <= ${#pods}; index++ )); do
    pod="${pods[index]}"
    printf '\n[%d/%d] pod/%s\n' "$index" "${#pods}" "$pod"
    _kube_pod_restart_one "$environment" "$pod" || {
      echo "Stopped after restart failure for pod/$pod." >&2
      return 1
    }
    replacements+=("${reply[1]}")
  done

  printf '\nRestarted %d/%d pod(s) successfully.\n' "${#replacements}" "${#pods}"
  if [[ "${KUBE_POD_RESTART_FOLLOW_LOGS:-1}" = "1" ]]; then
    _kube_follow_restarted_pods "$environment" "${replacements[@]}"
  fi
}

_kube_tool_binary() {
  local tool="$1"
  local source_name="${2:-$tool}"
  local binary="$HOME/.local/bin/$tool"
  local source_dir="$HOME/dots/tools/$source_name"

  if [[ -x "$binary" && "$binary" -nt "$source_dir/main.go" && "$binary" -nt "$source_dir/go.mod" ]]; then
    reply=("$binary")
    return
  fi

  if (( ! $+commands[go] )); then
    echo "$tool is not installed and Go is unavailable." >&2
    echo "Run: cd ~/dots && ./install -c steps/terminal.yml" >&2
    return 1
  fi

  echo "Building $tool…"
  mkdir -p "${binary:h}"
  (
    cd "$source_dir" &&
      go build -o "$binary" .
  ) || return
  reply=("$binary")
}

_kube_log_watch_binary() {
  _kube_tool_binary kube-log-watch
}

# Read-only: follows logs from every pod matching a shell glob.
_kube_logs_multi() {
  local environment="$1"
  shift
  local pod_pattern="${1:-}"
  local initial_query="${*:2}"
  local pod
  local -a kube matched monitor monitor_args filter_args

  _kube_env "$environment" || return
  kube=("${reply[@]}")

  if [[ -z "$pod_pattern" ]]; then
    if (( ! $+commands[gum] )); then
      read -r "pod_pattern?Pod name pattern (for example api*): " </dev/tty || return
    else
      pod_pattern="$(
        gum input \
          --header="Kubernetes ${kube[4]} · ${kube[3]} · multi-pod live logs" \
          --placeholder="api*" \
          --prompt="  " \
          --cursor.foreground="212"
      )" || return
    fi
  fi
  [[ -n "$pod_pattern" ]] || {
    echo "Pod name pattern cannot be empty." >&2
    return 2
  }

  _kube_matching_pods \
    "${kube[1]}" "${kube[2]}" "${kube[3]}" "$pod_pattern" || return
  matched=("${reply[@]}")
  (( ${#matched} )) || {
    echo "No pods match '$pod_pattern' in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  for pod in "${matched[@]}"; do
    monitor_args+=(--pod "$pod")
  done
  [[ "${KUBE_MULTI_LOG_FILTER:-0}" = "1" ]] && filter_args=(--filter)

  _kube_log_watch_binary || return
  monitor=("${reply[@]}")
  "${monitor[1]}" \
    --kubeconfig "${kube[1]}" \
    --context "${kube[2]}" \
    --namespace "${kube[3]}" \
    --environment "${kube[4]}" \
    --tail="${KUBE_LOG_TAIL:-200}" \
    --buffer="${KUBE_MULTI_LOG_BUFFER:-5000}" \
    --query "$initial_query" \
    "${filter_args[@]}" \
    "${monitor_args[@]}"
}

_kube_cron() {
  local environment="$1"
  local initial_filter="${2:-}"
  local -a kube
  local rows choice cron job base log_dir log_file

  _kube_env "$environment" action || return
  kube=("${reply[@]}")

  rows="$(
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get cronjobs --sort-by=.metadata.name --no-headers
  )" || return

  [[ -n "$rows" ]] || {
    echo "No CronJobs found in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  choice="$(
    print -r -- "$rows" |
      _kube_choose \
        "Kubernetes ${kube[4]} · ${kube[3]} · run a CronJob now" \
        "Filter CronJobs…" \
        "$initial_filter"
  )" || return
  [[ -n "$choice" ]] || return

  cron="$(_kube_first_column "$choice")"
  # Kubernetes names are limited to 63 characters. The timestamped suffix is 22.
  base="${cron[1,41]}"
  base="${base%-}"
  job="${base}-manual-$(date +%Y%m%d%H%M%S)"
  log_dir="${KUBE_LOG_DIR:-$HOME/Documents/kube-logs}"
  log_file="$log_dir/${kube[4]}-$job.log"

  mkdir -p -m 700 "$log_dir" || {
    echo "Cannot create Kubernetes log directory: $log_dir" >&2
    return 1
  }
  (umask 077; : >"$log_file") || {
    echo "Cannot create Kubernetes log file: $log_file" >&2
    return 1
  }

  echo "Creating job/$job from cronjob/$cron in ${kube[3]} (${kube[4]})."
  _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
    create job --from="cronjob/$cron" "$job" || {
      local create_status="$?"
      command rm -f -- "$log_file"
      return "$create_status"
    }

  echo "Waiting for the Job container; logs will follow when it starts. Press Ctrl-C to stop."
  echo "Saving raw logs to: $log_file"
  _kube_job_logs_to_file "$log_file" \
    "${kube[1]}" "${kube[2]}" "${kube[3]}" \
    logs -f \
    -l "job-name=$job" \
    --all-containers=true \
    --prefix=true \
    --tail="${KUBE_LOG_TAIL:-200}" \
    --max-log-requests="${KUBE_JOB_LOG_STREAMS:-20}" \
    --pod-running-timeout="${KUBE_JOB_LOG_ATTEMPT_WAIT:-5s}"
}

_kube_jobs() {
  local environment="$1"
  local initial_filter="${2:-}"
  local mode="${3:-follow}"
  local -a kube
  local jobs_json rows choice job log_file

  _kube_env "$environment" || return
  kube=("${reply[@]}")

  if (( ! $+commands[jq] )); then
    echo "jq is required to build the Job history list." >&2
    return 1
  fi

  jobs_json="$(
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get jobs -o json
  )" || return

  rows="$(
    print -r -- "$jobs_json" |
      jq -r '
        .items
        | sort_by(.metadata.creationTimestamp)
        | reverse
        | .[]
        | (
            if (.status.active // 0) > 0 then "Running"
            elif (.status.succeeded // 0) > 0 then "Complete"
            elif (.status.failed // 0) > 0 then "Failed"
            else "Pending"
            end
          ) as $state
        | (
            [.metadata.ownerReferences[]?
              | select(.kind == "CronJob")
              | .name
            ][0] // "-"
          ) as $cronjob
        | [
            .metadata.name,
            $state,
            (
              (.status.startTime // .metadata.creationTimestamp)
              | fromdateiso8601
              | strflocaltime("%Y-%m-%d %H:%M:%S")
            ),
            $cronjob
          ]
        | @tsv
      ' |
      column -t -s $'\t'
  )" || return

  [[ -n "$rows" ]] || {
    echo "No Jobs found in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  choice="$(
    print -r -- "$rows" |
      _kube_choose \
        "Kubernetes ${kube[4]} · ${kube[3]} · Jobs newest first · NAME / STATUS / STARTED / CRONJOB" \
        "Filter Jobs or CronJobs…" \
        "$initial_filter" \
        1
  )" || return
  [[ -n "$choice" ]] || return

  job="$(_kube_first_column "$choice")"
  if [[ "$mode" = "save" ]]; then
    _kube_log_file "${kube[4]}" job "$job" || return
    log_file="${reply[1]}"
    echo "Saving complete available logs for all pods of job/$job in ${kube[3]} (${kube[4]})."
    echo "File: $log_file"
    _kube_stream_logs_to_file "$log_file" \
      "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      logs \
      -l "job-name=$job" \
      --all-containers=true \
      --prefix=true \
      --timestamps=true \
      --tail=-1 \
      --max-log-requests="${KUBE_JOB_LOG_STREAMS:-20}" \
      --pod-running-timeout="${KUBE_JOB_WAIT:-2m}"
    local save_status="$?"
    (( save_status == 0 )) && echo "Saved: $log_file"
    return "$save_status"
  fi

  echo "Following logs for all pods of job/$job in ${kube[3]} (${kube[4]}). Press Ctrl-C to stop."
  _kube_stream_logs "${kube[1]}" "${kube[2]}" "${kube[3]}" \
    logs -f \
    -l "job-name=$job" \
    --all-containers=true \
    --prefix=true \
    --tail="${KUBE_LOG_TAIL:-200}" \
    --max-log-requests="${KUBE_JOB_LOG_STREAMS:-20}" \
    --pod-running-timeout="${KUBE_JOB_WAIT:-2m}"
}

_kube_deploy_watch_binary() {
  _kube_tool_binary kube-deploy-watch kube-rollout
}

_kube_pod_analyze_binary() {
  _kube_tool_binary kube-pod-analyze
}

# Read-only: samples metrics-server, pod status and events, then stores local CSV history.
_kube_pod_analyze() {
  local environment="$1"
  local initial_filter="${2:-}"
  local -a kube analyzer analyzer_args choices pods
  local rows choice pod

  _kube_env "$environment" || return
  kube=("${reply[@]}")

  rows="$(
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get pods --no-headers
  )" || return
  rows="$(print -r -- "$rows" | LC_ALL=C sort -k1,1)"
  [[ -n "$rows" ]] || {
    echo "No pods found in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  choice="$(
    print -r -- "$rows" |
      _kube_choose \
        "Kubernetes ${kube[4]} · ${kube[3]} · pod analytics · choose one or more · READ ONLY" \
        "Choose pods or workload replicas to analyze…" \
        "$initial_filter" \
        0 \
        multi
  )" || return
  [[ -n "$choice" ]] || return
  choices=("${(@f)choice}")
  for choice in "${choices[@]}"; do
    pod="$(_kube_first_column "$choice")"
    (( ${pods[(Ie)$pod]} )) || pods+=("$pod")
  done
  (( ${#pods} )) || return

  _kube_pod_analyze_binary || return
  analyzer=("${reply[@]}")
  analyzer_args=(
    --kubeconfig "${kube[1]}"
    --context "${kube[2]}"
    --namespace "${kube[3]}"
    --environment "${kube[4]}"
    --live
    --refresh "${KUBE_POD_ANALYZE_REFRESH:-100ms}"
    --terminal-lines "${LINES:-0}"
    --history-window "${KUBE_POD_ANALYZE_WINDOW:-24h}"
    --retention "${KUBE_POD_ANALYZE_RETENTION:-720h}"
    --chart-points "${KUBE_POD_ANALYZE_CHART_POINTS:-72}"
    --history-dir "${KUBE_POD_ANALYZE_HISTORY_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/kube-tools/pod-metrics}"
    --processes="${KUBE_POD_ANALYZE_PROCESSES:-true}"
    --process-refresh "${KUBE_POD_ANALYZE_PROCESS_REFRESH:-5s}"
  )
  for pod in "${pods[@]}"; do
    analyzer_args+=(--pod "$pod")
  done
  [[ "${KUBE_POD_ANALYZE_ONCE:-0}" = "1" ]] && analyzer_args+=(--once)

  "${analyzer[1]}" "${analyzer_args[@]}"
}

# ACTION: changes desired replicas for a Deployment or StatefulSet.
_kube_scale() {
  local environment="$1"
  local initial_filter="${2:-}"
  local replicas="${3:-}"
  local -a kube fields
  local raw_rows row picker_rows choice resource current kind

  _kube_env "$environment" action || return
  kube=("${reply[@]}")

  raw_rows="$(
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get deployments,statefulsets \
      -o 'custom-columns=KIND:.kind,NAME:.metadata.name,DESIRED:.spec.replicas,CURRENT:.status.replicas,READY:.status.readyReplicas,AVAILABLE:.status.availableReplicas' \
      --no-headers
  )" || return

  [[ -n "$raw_rows" ]] || {
    echo "No scalable workloads found in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  for row in "${(@f)raw_rows}"; do
    fields=(${=row})
    (( ${#fields} >= 3 )) || continue
    kind="${(L)fields[1]}"
    picker_rows+="${kind}/${fields[2]}"$'\t'"${fields[3]:-0}"$'\t'"${fields[4]:-0}"$'\t'"${fields[5]:-0}"$'\t'"${fields[6]:-0}"$'\n'
  done
  picker_rows="${picker_rows%$'\n'}"

  choice="$(
    print -r -- "$picker_rows" |
      _kube_choose \
        "Kubernetes ${kube[4]} · ${kube[3]} · scale workload · RESOURCE / DESIRED / CURRENT / READY / AVAILABLE" \
        "Choose a Deployment or StatefulSet…" \
        "$initial_filter"
  )" || return
  [[ -n "$choice" ]] || return

  resource="$(_kube_first_column "$choice")"
  fields=(${=choice})
  current="${fields[2]:-0}"

  if [[ -z "$replicas" ]]; then
    if (( $+commands[gum] )); then
      replicas="$(
        gum input \
          --header="Scale $resource in ${kube[4]}/${kube[3]} · current desired replicas: $current" \
          --placeholder="$current" \
          --value="$current" \
          --prompt="  " \
          --cursor.foreground="212"
      )" || return
    else
      read -r "replicas?Desired replicas for $resource (current $current): " </dev/tty || return
    fi
  fi

  [[ "$replicas" = <-> ]] || {
    echo "Replica count must be a non-negative integer." >&2
    return 2
  }

  echo "Scaling $resource in ${kube[3]} (${kube[4]}): $current -> $replicas replicas."
  _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
    scale "$resource" --replicas="$replicas" || return
  echo "Watch progress: kp${environment}-deploy-watch ${resource#*/}"
}

_kube_deploy_watch_configured_resources() {
  local config_file="${KUBE_DEPLOY_WATCH_RESOURCES_FILE:-${XDG_CONFIG_HOME:-$HOME/.config}/kube-tools/deploy-watch.resources}"
  local name_pattern='^[a-z0-9]([-a-z0-9.]*[a-z0-9])?$'
  local line
  local -a configured=()

  if [[ ! -r "$config_file" ]]; then
    reply=()
    return
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line#"${line%%[![:space:]]*}"}"
    line="${line%"${line##*[![:space:]]}"}"
    [[ -z "$line" || "$line" == \#* ]] && continue

    if (( ${#line} > 253 )) || [[ ! "$line" =~ $name_pattern ]]; then
      echo "Invalid Kubernetes workload name in $config_file: $line" >&2
      return 1
    fi
    (( ${configured[(Ie)$line]} )) || configured+=("$line")
  done <"$config_file"

  reply=("${configured[@]}")
}

# Read-only: lists workloads, then starts a monitor that only executes get/logs.
_kube_deploy_watch() {
  local environment="$1"
  local initial_filter="${2:-}"
  local -a kube monitor monitor_args configured_names group_resources missing_names workload_rows
  local rows picker_rows choice resource row candidate candidate_name configured_name
  local found

  _kube_env "$environment" || return
  kube=("${reply[@]}")
  _kube_deploy_watch_configured_resources || return
  configured_names=("${reply[@]}")

  rows="$(
    _kube_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get deployments,statefulsets,daemonsets \
      --no-headers
  )" || return
  rows="$(print -r -- "$rows" | LC_ALL=C sort -k1,1)"

  [[ -n "$rows" ]] || {
    echo "No rollout workloads found in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  workload_rows=("${(@f)rows}")
  for configured_name in "${configured_names[@]}"; do
    found=0
    for row in "${workload_rows[@]}"; do
      candidate="$(_kube_first_column "$row")"
      candidate_name="${candidate#*/}"
      if [[ "$candidate_name" == "$configured_name" ]]; then
        group_resources+=("$candidate")
        found=1
        break
      fi
    done
    (( found )) || missing_names+=("$configured_name")
  done

  picker_rows="$rows"
  if (( ${#group_resources} > 1 )); then
    picker_rows=$'group/configured-suite\tPrivate configured group ('"${#group_resources}/${#configured_names}"$' workloads found)\n'"$rows"
  fi

  choice="$(
    print -r -- "$picker_rows" |
      _kube_choose \
        "Kubernetes ${kube[4]} · ${kube[3]} · deployment watch · READ ONLY" \
        "Choose the private configured group or one workload…" \
        "$initial_filter"
  )" || return
  [[ -n "$choice" ]] || return
  resource="$(_kube_first_column "$choice")"

  if [[ "$resource" == "group/configured-suite" ]]; then
    if (( ${#missing_names} )); then
      echo "Configured workloads not found in ${kube[3]}: ${(j:, :)missing_names}" >&2
    fi
    for candidate in "${group_resources[@]}"; do
      monitor_args+=(--resource "$candidate")
    done
  else
    monitor_args=(--resource "$resource")
  fi

  _kube_deploy_watch_binary || return
  monitor=("${reply[@]}")
  "${monitor[1]}" \
    --kubeconfig "${kube[1]}" \
    --context "${kube[2]}" \
    --namespace "${kube[3]}" \
    --environment "${kube[4]}" \
    --refresh "${KUBE_DEPLOY_WATCH_REFRESH:-100ms}" \
    "${monitor_args[@]}"
}

kpdev-logs() {
  _kube_logs dev "$@"
}

kpstage-logs() {
  _kube_logs stage "$@"
}

kppreprod-logs() {
  _kube_logs preprod "$@"
}

kpprod-logs() {
  _kube_logs prod "$@"
}

kpdev-logs-save() {
  _kube_logs dev "${1:-}" save
}

kpstage-logs-save() {
  _kube_logs stage "${1:-}" save
}

kppreprod-logs-save() {
  _kube_logs preprod "${1:-}" save
}

kpprod-logs-save() {
  _kube_logs prod "${1:-}" save
}

kpdev-logs-multi() {
  _kube_logs_multi dev "$@"
}

kpstage-logs-multi() {
  _kube_logs_multi stage "$@"
}

kppreprod-logs-multi() {
  _kube_logs_multi preprod "$@"
}

kpprod-logs-multi() {
  _kube_logs_multi prod "$@"
}

kpdev-cron-run() {
  _kube_cron dev "$@"
}

kpstage-cron-run() {
  _kube_cron stage "$@"
}

kppreprod-cron-run() {
  _kube_cron preprod "$@"
}

kpprod-cron-run() {
  _kube_cron prod "$@"
}

kpdev-jobs() {
  _kube_jobs dev "$@"
}

kpstage-jobs() {
  _kube_jobs stage "$@"
}

kppreprod-jobs() {
  _kube_jobs preprod "$@"
}

kpprod-jobs() {
  _kube_jobs prod "$@"
}

kpdev-jobs-save() {
  _kube_jobs dev "${1:-}" save
}

kpstage-jobs-save() {
  _kube_jobs stage "${1:-}" save
}

kppreprod-jobs-save() {
  _kube_jobs preprod "${1:-}" save
}

kpprod-jobs-save() {
  _kube_jobs prod "${1:-}" save
}

kpdev-exec() {
  _kube_exec dev "$@"
}

kpstage-exec() {
  _kube_exec stage "$@"
}

kppreprod-exec() {
  _kube_exec preprod "$@"
}

kpprod-exec() {
  _kube_exec prod "$@"
}

kpdev-log-search() {
  _kube_log_search dev "$@"
}

kpstage-log-search() {
  _kube_log_search stage "$@"
}

kppreprod-log-search() {
  _kube_log_search preprod "$@"
}

kpprod-log-search() {
  _kube_log_search prod "$@"
}

kpdev-deploy-watch() {
  _kube_deploy_watch dev "$@"
}

kpstage-deploy-watch() {
  _kube_deploy_watch stage "$@"
}

kppreprod-deploy-watch() {
  _kube_deploy_watch preprod "$@"
}

kpprod-deploy-watch() {
  _kube_deploy_watch prod "$@"
}

kpdev-scale() {
  _kube_scale dev "$@"
}

kpstage-scale() {
  _kube_scale stage "$@"
}

kppreprod-scale() {
  _kube_scale preprod "$@"
}

kpprod-scale() {
  _kube_scale prod "$@"
}

kpdev-pod-analyze() {
  _kube_pod_analyze dev "$@"
}

kpstage-pod-analyze() {
  _kube_pod_analyze stage "$@"
}

kppreprod-pod-analyze() {
  _kube_pod_analyze preprod "$@"
}

kpprod-pod-analyze() {
  _kube_pod_analyze prod "$@"
}

kpdev-pod-restart() {
  _kube_pod_restart dev "$@"
}

kpstage-pod-restart() {
  _kube_pod_restart stage "$@"
}

kppreprod-pod-restart() {
  _kube_pod_restart preprod "$@"
}

kpprod-pod-restart() {
  _kube_pod_restart prod "$@"
}
