# Интерактивные команды для Podbor Kubernetes.
# Базовые _podbor_kubectl и _podbor_prod_confirm определены в .zshrc.

# Remove names from older sourced versions so action/watch boundaries stay explicit.
unfunction \
  kpdev-rollout kpstage-rollout kpprod-rollout \
  kpdev-cron kpstage-cron kpprod-cron \
  2>/dev/null || true

_podbor_kube_env() {
  local environment="$1"
  local access_mode="${2:-read}"

  case "$environment" in
    dev)
      reply=(
        "$PODBOR_DEV_KUBECONFIG"
        "$PODBOR_DEV_CONTEXT"
        "$PODBOR_DEV_NAMESPACE"
        "dev"
      )
      ;;
    stage)
      reply=(
        "$PODBOR_STAGE_KUBECONFIG"
        "$PODBOR_STAGE_CONTEXT"
        "$PODBOR_STAGE_NAMESPACE"
        "stage"
      )
      ;;
    prod)
      if [[ "$access_mode" = "action" ]]; then
        _podbor_prod_confirm || return
      fi
      reply=(
        "$PODBOR_PROD_KUBECONFIG"
        "$PODBOR_PROD_CONTEXT"
        "$PODBOR_PROD_NAMESPACE"
        "prod"
      )
      ;;
    *)
      echo "Unknown Podbor environment: $environment" >&2
      return 2
      ;;
  esac
}

_podbor_kube_choose() {
  local header="$1"
  local placeholder="$2"
  local initial_filter="${3:-}"
  local preserve_order="${4:-0}"
  local -a order_args=()

  (( preserve_order )) && order_args=(--no-fuzzy-sort)

  if (( $+commands[gum] )); then
    gum filter \
      --limit=1 \
      --height=16 \
      --header="$header" \
      --placeholder="$placeholder" \
      --value="$initial_filter" \
      "${order_args[@]}" \
      --indicator="›" \
      --prompt="  " \
      --indicator.foreground="212" \
      --match.foreground="212"
    return
  fi

  local input
  input="$(cat)"
  local -a choices=("${(@f)input}")
  local index selected
  if (( ! ${#choices} )); then
    return 1
  fi

  echo "$header" >&2
  for (( index = 1; index <= ${#choices}; index++ )); do
    printf "  %2d) %s\n" "$index" "${choices[index]}" >&2
  done
  read -r "selected?Choose [1-${#choices}]: " </dev/tty || return
  [[ "$selected" = <-> ]] || return 1
  (( selected >= 1 && selected <= ${#choices} )) || return 1
  print -r -- "${choices[selected]}"
}

_podbor_kube_first_column() {
  local row="$1"
  print -r -- "${row%%[[:space:]]*}"
}

_podbor_colorize_logs() {
  if [[ -n "${NO_COLOR:-}" || ! -x /usr/bin/perl ]]; then
    cat
    return
  fi
  if [[ "${PODBOR_COLOR:-auto}" != "always" && ! -t 1 ]]; then
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

_podbor_kube_logs() {
  _podbor_kubectl "$@" 2>&1 | _podbor_colorize_logs
  local kubectl_status="${pipestatus[1]}"
  return "$kubectl_status"
}

_podbor_kube_logs_to_file() {
  local log_file="$1"
  shift

  _podbor_kubectl "$@" 2>&1 |
    tee "$log_file" |
    _podbor_colorize_logs
  local kubectl_status="${pipestatus[1]}"
  return "$kubectl_status"
}

_podbor_kube_log_file() {
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

_podbor_search_one_pod_logs() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  local pod="$4"
  local since="$5"
  local include_previous="$6"
  local -a since_args=()

  [[ -n "$since" ]] && since_args=(--since="$since")

  _podbor_kubectl "$kubeconfig" "$context" "$namespace" \
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
    ) || true

  if [[ "$include_previous" = "1" ]]; then
    _podbor_kubectl "$kubeconfig" "$context" "$namespace" \
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
}

_podbor_search_pod_logs() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  local since="$4"
  local include_previous="$5"
  shift 5
  local -a pods=("$@")
  local -a pids=()
  local pod pid
  local parallel="${KUBE_LOG_SEARCH_PARALLEL:-6}"

  [[ "$parallel" = <-> ]] && (( parallel > 0 )) || parallel=6

  for pod in "${pods[@]}"; do
    _podbor_search_one_pod_logs \
      "$kubeconfig" "$context" "$namespace" \
      "$pod" "$since" "$include_previous" &
    pids+=("$!")

    if (( ${#pids} >= parallel )); then
      for pid in "${pids[@]}"; do
        wait "$pid" || true
      done
      pids=()
    fi
  done

  for pid in "${pids[@]}"; do
    wait "$pid" || true
  done
}

_podbor_log_search() {
  local environment="$1"
  shift
  local -a kube pods matched
  local pod_pattern="${1:-}"
  local query="${*:2}"
  local pod_names pod normalized_pattern
  local since="${KUBE_LOG_SEARCH_SINCE:-}"
  local include_previous="${KUBE_LOG_SEARCH_PREVIOUS:-1}"
  local color_mode="always"

  _podbor_kube_env "$environment" || return
  kube=("${reply[@]}")

  if (( ! $+commands[rg] )); then
    echo "ripgrep is required for Kubernetes log search." >&2
    return 1
  fi

  if [[ -z "$pod_pattern" ]]; then
    if (( ! $+commands[gum] )); then
      read -r "pod_pattern?Pod name pattern (for example service-api*): " </dev/tty || return
    else
      pod_pattern="$(
        gum input \
          --header="Podbor ${kube[4]} · ${kube[3]} · pod name pattern" \
          --placeholder="service-api*" \
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

  pod_names="$(
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
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

  (( ${#matched} )) || {
    echo "No pods match '$pod_pattern' in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  [[ -n "${NO_COLOR:-}" || ! -t 1 ]] && color_mode="never"
  echo "Searching ${#matched} pod(s) in ${kube[3]} (${kube[4]}) for literal: $query" >&2
  [[ -n "$since" ]] && echo "Log window: since $since" >&2
  [[ "$include_previous" = "1" ]] && echo "Including previous container logs." >&2

  _podbor_search_pod_logs \
    "${kube[1]}" "${kube[2]}" "${kube[3]}" \
    "$since" "$include_previous" \
    "${matched[@]}" |
    rg \
      --fixed-strings \
      --line-buffered \
      --color="$color_mode" \
      -- "$query"
  local search_status="${pipestatus[2]}"

  if (( search_status == 1 )); then
    echo "No matching log lines found." >&2
  fi
  return "$search_status"
}

_podbor_dedent() {
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

_podbor_exec_dedented() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  local target="$4"
  shift 4

  _podbor_dedent |
    _podbor_kubectl "$kubeconfig" "$context" "$namespace" \
      exec -i "$target" -- "$@"
  local kubectl_status="${pipestatus[2]}"
  return "$kubectl_status"
}

_podbor_exec_dedented_at() {
  local kubeconfig="$1"
  local context="$2"
  local namespace="$3"
  local target="$4"
  local workdir="$5"
  shift 5

  if [[ -z "$workdir" ]]; then
    _podbor_exec_dedented \
      "$kubeconfig" "$context" "$namespace" "$target" "$@"
    return
  fi

  _podbor_dedent |
    _podbor_kubectl "$kubeconfig" "$context" "$namespace" \
      exec -i "$target" -- \
      sh -c 'cd "$1" && shift && exec "$@"' \
      podbor-exec "$workdir" "$@"
  local kubectl_status="${pipestatus[2]}"
  return "$kubectl_status"
}

_podbor_choose_exec_workdir() {
  local environment="$1"
  local target="$2"
  local selection workdir

  selection="$(
    printf '%s\n' \
      "default   container WORKDIR" \
      "..        parent of container WORKDIR" \
      "../..     two levels above container WORKDIR" \
      "custom    enter a path" |
      _podbor_kube_choose \
        "Podbor ${environment} · $target · working directory" \
        "Choose where the command starts…"
  )" || return
  selection="$(_podbor_kube_first_column "$selection")"

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

_podbor_exec() {
  local environment="$1"
  shift
  local -a kube command
  local target rows choice mode script workdir workdir_display
  local choose_mode=0

  _podbor_kube_env "$environment" action || return
  kube=("${reply[@]}")

  if (( ! $# )) || [[ "$1" = "--" ]]; then
    rows="$(
      _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
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
        _podbor_kube_choose \
          "Podbor ${kube[4]} · ${kube[3]} · choose where to execute" \
          "Filter pods and workloads…"
    )" || return
    [[ -n "$choice" ]] || return
    target="$(_podbor_kube_first_column "$choice")"
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
        _podbor_kube_choose \
          "Podbor ${kube[4]} · $target · what to execute" \
          "Choose bash or python…"
    )" || return
    mode="$(_podbor_kube_first_column "$mode")"

    _podbor_choose_exec_workdir "${kube[4]}" "$target" || return
    workdir="${reply[1]}"
    workdir_display="${workdir:-container WORKDIR}"

    case "$mode" in
      bash)
        if [[ -z "$workdir" ]]; then
          _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
            exec -it "$target" -- bash
        else
          _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
            exec -it "$target" -- \
            bash -lc 'cd -- "$1" && exec bash -l' \
            podbor-exec "$workdir"
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
        _podbor_exec_dedented_at \
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
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      exec -it "$target" -- "${command[@]}"
    return
  fi

  echo "Executing in $target (${kube[4]}/${kube[3]}); common input indentation removed." >&2
  _podbor_exec_dedented \
    "${kube[1]}" "${kube[2]}" "${kube[3]}" "$target" \
    "${command[@]}"
}

_podbor_logs() {
  local environment="$1"
  local initial_filter="${2:-}"
  local mode="${3:-follow}"
  local -a kube
  local rows choice pod log_file

  _podbor_kube_env "$environment" || return
  kube=("${reply[@]}")

  rows="$(
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get pods --sort-by=.metadata.creationTimestamp --no-headers
  )" || return

  [[ -n "$rows" ]] || {
    echo "No pods found in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  choice="$(
    print -r -- "$rows" |
      _podbor_kube_choose \
        "Podbor ${kube[4]} · ${kube[3]} · choose a pod" \
        "Filter pods…" \
        "$initial_filter"
  )" || return
  [[ -n "$choice" ]] || return

  pod="$(_podbor_kube_first_column "$choice")"
  if [[ "$mode" = "save" ]]; then
    _podbor_kube_log_file "${kube[4]}" pod "$pod" || return
    log_file="${reply[1]}"
    echo "Saving complete available logs for pod/$pod in ${kube[3]} (${kube[4]})."
    echo "File: $log_file"
    _podbor_kube_logs_to_file "$log_file" \
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
  _podbor_kube_logs "${kube[1]}" "${kube[2]}" "${kube[3]}" \
    logs -f "pod/$pod" \
    --all-containers=true \
    --prefix=true \
    --tail="${KUBE_LOG_TAIL:-200}"
}

# ACTION: удаляет выбранный controller-managed pod, ждёт replacement с новым UID
# и после Ready подключается к его логам.
_podbor_pod_restart() {
  local environment="$1"
  local initial_filter="${2:-}"
  local -a kube
  local rows choice pod pod_json owner_kind owner_name owner_resource owner_json
  local selector baseline_json current_json current_rows previous_rows replacement replacement_row
  local timeout started elapsed poll_interval delete_output interactive spinner frame color_reset
  local color_dim color_blue color_yellow color_green color_red status_color
  local replacement_ready replacement_total replacement_phase replacement_restarts

  _podbor_kube_env "$environment" action || return
  kube=("${reply[@]}")

  if (( ! $+commands[jq] )); then
    echo "jq is required to track the replacement pod." >&2
    return 1
  fi

  rows="$(
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get pods --sort-by=.metadata.creationTimestamp --no-headers
  )" || return

  [[ -n "$rows" ]] || {
    echo "No pods found in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  choice="$(
    print -r -- "$rows" |
      _podbor_kube_choose \
        "Podbor ${kube[4]} · ${kube[3]} · restart a pod · ACTION" \
        "Filter pods…" \
        "$initial_filter"
  )" || return
  [[ -n "$choice" ]] || return
  pod="$(_podbor_kube_first_column "$choice")"

  pod_json="$(
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
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
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
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
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
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
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      delete "pod/$pod" --wait=false
  )" || {
    local delete_status="$?"
    print -u2 -r -- "$delete_output"
    return "$delete_status"
  }
  printf '%s✓%s Delete accepted %s%s%s\n' \
    "$color_green" "$color_reset" "$color_dim" "$owner_resource" "$color_reset"

  timeout="${KUBE_POD_RESTART_TIMEOUT:-300}"
  poll_interval="${KUBE_POD_RESTART_POLL_INTERVAL:-2}"
  [[ "$timeout" = <-> ]] && (( timeout > 0 )) || timeout=300
  [[ "$poll_interval" = <-> ]] && (( poll_interval > 0 )) || poll_interval=2
  started="$SECONDS"
  previous_rows=""
  spinner=('⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏')
  frame=1

  while (( SECONDS - started < timeout )); do
    current_json="$(
      _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
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
      if [[ "${KUBE_POD_RESTART_FOLLOW_LOGS:-1}" = "1" ]]; then
        printf '%sFollowing logs%s %s(Ctrl-C to stop)%s\n' \
          "$color_blue" "$color_reset" "$color_dim" "$color_reset"
        _podbor_kube_logs "${kube[1]}" "${kube[2]}" "${kube[3]}" \
          logs -f "pod/$replacement" \
          --all-containers=true \
          --prefix=true \
          --tail="${KUBE_LOG_TAIL:-200}"
      fi
      return 0
    fi

    sleep "$poll_interval"
  done

  (( interactive )) && printf '\r\e[2K'
  echo "Timed out after ${timeout}s waiting for a Ready replacement of pod/$pod." >&2
  return 1
}

_podbor_log_watch_binary() {
  local binary="$HOME/.local/bin/podbor-log-watch"
  local source_dir="$HOME/dots/tools/podbor-log-watch"

  if [[ -x "$binary" && "$binary" -nt "$source_dir/main.go" && "$binary" -nt "$source_dir/go.mod" ]]; then
    reply=("$binary")
    return
  fi

  if (( ! $+commands[go] )); then
    echo "podbor-log-watch is not installed and Go is unavailable." >&2
    echo "Run: cd ~/dots && ./install -c steps/terminal.yml" >&2
    return 1
  fi

  echo "Building podbor-log-watch…"
  mkdir -p "${binary:h}"
  (
    cd "$source_dir" &&
      go build -o "$binary" .
  ) || return
  reply=("$binary")
}

# Read-only: follows logs from every pod matching a shell glob.
_podbor_logs_multi() {
  local environment="$1"
  shift
  local pod_pattern="${1:-}"
  local initial_query="${*:2}"
  local normalized_pattern pod_names pod
  local -a kube pods matched monitor monitor_args filter_args

  _podbor_kube_env "$environment" || return
  kube=("${reply[@]}")

  if [[ -z "$pod_pattern" ]]; then
    if (( ! $+commands[gum] )); then
      read -r "pod_pattern?Pod name pattern (for example service-api*): " </dev/tty || return
    else
      pod_pattern="$(
        gum input \
          --header="Podbor ${kube[4]} · ${kube[3]} · multi-pod live logs" \
          --placeholder="service-api*" \
          --prompt="  " \
          --cursor.foreground="212"
      )" || return
    fi
  fi
  [[ -n "$pod_pattern" ]] || {
    echo "Pod name pattern cannot be empty." >&2
    return 2
  }

  pod_names="$(
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
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
  (( ${#matched} )) || {
    echo "No pods match '$pod_pattern' in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  for pod in "${matched[@]}"; do
    monitor_args+=(--pod "$pod")
  done
  [[ "${KUBE_MULTI_LOG_FILTER:-0}" = "1" ]] && filter_args=(--filter)

  _podbor_log_watch_binary || return
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

_podbor_cron() {
  local environment="$1"
  local initial_filter="${2:-}"
  local -a kube
  local rows choice cron job base log_dir log_file

  _podbor_kube_env "$environment" action || return
  kube=("${reply[@]}")

  rows="$(
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get cronjobs --sort-by=.metadata.name --no-headers
  )" || return

  [[ -n "$rows" ]] || {
    echo "No CronJobs found in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  choice="$(
    print -r -- "$rows" |
      _podbor_kube_choose \
        "Podbor ${kube[4]} · ${kube[3]} · run a CronJob now" \
        "Filter CronJobs…" \
        "$initial_filter"
  )" || return
  [[ -n "$choice" ]] || return

  cron="$(_podbor_kube_first_column "$choice")"
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
  _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
    create job --from="cronjob/$cron" "$job" || {
      local create_status="$?"
      command rm -f -- "$log_file"
      return "$create_status"
    }

  echo "Following all job pod logs. Press Ctrl-C to stop."
  echo "Saving raw logs to: $log_file"
  _podbor_kube_logs_to_file "$log_file" \
    "${kube[1]}" "${kube[2]}" "${kube[3]}" \
    logs -f \
    -l "job-name=$job" \
    --all-containers=true \
    --prefix=true \
    --tail="${KUBE_LOG_TAIL:-200}" \
    --max-log-requests="${KUBE_JOB_LOG_STREAMS:-20}" \
    --pod-running-timeout="${KUBE_JOB_WAIT:-2m}"
}

_podbor_jobs() {
  local environment="$1"
  local initial_filter="${2:-}"
  local mode="${3:-follow}"
  local -a kube
  local jobs_json rows choice job log_file

  _podbor_kube_env "$environment" || return
  kube=("${reply[@]}")

  if (( ! $+commands[jq] )); then
    echo "jq is required to build the Job history list." >&2
    return 1
  fi

  jobs_json="$(
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
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
      _podbor_kube_choose \
        "Podbor ${kube[4]} · ${kube[3]} · Jobs newest first · NAME / STATUS / STARTED / CRONJOB" \
        "Filter Jobs or CronJobs…" \
        "$initial_filter" \
        1
  )" || return
  [[ -n "$choice" ]] || return

  job="$(_podbor_kube_first_column "$choice")"
  if [[ "$mode" = "save" ]]; then
    _podbor_kube_log_file "${kube[4]}" job "$job" || return
    log_file="${reply[1]}"
    echo "Saving complete available logs for all pods of job/$job in ${kube[3]} (${kube[4]})."
    echo "File: $log_file"
    _podbor_kube_logs_to_file "$log_file" \
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
  _podbor_kube_logs "${kube[1]}" "${kube[2]}" "${kube[3]}" \
    logs -f \
    -l "job-name=$job" \
    --all-containers=true \
    --prefix=true \
    --tail="${KUBE_LOG_TAIL:-200}" \
    --max-log-requests="${KUBE_JOB_LOG_STREAMS:-20}" \
    --pod-running-timeout="${KUBE_JOB_WAIT:-2m}"
}

_podbor_deploy_watch_binary() {
  local binary="$HOME/.local/bin/podbor-deploy-watch"
  local source_dir="$HOME/dots/tools/podbor-rollout"

  if [[ -x "$binary" && "$binary" -nt "$source_dir/main.go" && "$binary" -nt "$source_dir/go.mod" ]]; then
    reply=("$binary")
    return
  fi

  if (( ! $+commands[go] )); then
    echo "podbor-deploy-watch is not installed and Go is unavailable." >&2
    echo "Run: cd ~/dots && ./install -c steps/terminal.yml" >&2
    return 1
  fi

  echo "Building podbor-deploy-watch…"
  mkdir -p "${binary:h}"
  (
    cd "$source_dir" &&
      go build -o "$binary" .
  ) || return
  reply=("$binary")
}

_podbor_pod_analyze_binary() {
  local binary="$HOME/.local/bin/podbor-pod-analyze"
  local source_dir="$HOME/dots/tools/podbor-pod-analyze"

  if [[ -x "$binary" && "$binary" -nt "$source_dir/main.go" && "$binary" -nt "$source_dir/go.mod" ]]; then
    reply=("$binary")
    return
  fi

  if (( ! $+commands[go] )); then
    echo "podbor-pod-analyze is not installed and Go is unavailable." >&2
    echo "Run: cd ~/dots && ./install -c steps/terminal.yml" >&2
    return 1
  fi

  echo "Building podbor-pod-analyze…"
  mkdir -p "${binary:h}"
  (
    cd "$source_dir" &&
      go build -o "$binary" .
  ) || return
  reply=("$binary")
}

# Read-only: samples metrics-server, pod status and events, then stores local CSV history.
_podbor_pod_analyze() {
  local environment="$1"
  local initial_filter="${2:-}"
  local -a kube analyzer analyzer_args
  local rows choice pod

  _podbor_kube_env "$environment" || return
  kube=("${reply[@]}")

  rows="$(
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
      get pods --no-headers
  )" || return
  rows="$(print -r -- "$rows" | LC_ALL=C sort -k1,1)"
  [[ -n "$rows" ]] || {
    echo "No pods found in ${kube[3]} (${kube[4]})." >&2
    return 1
  }

  choice="$(
    print -r -- "$rows" |
      _podbor_kube_choose \
        "Podbor ${kube[4]} · ${kube[3]} · pod analytics · READ ONLY" \
        "Choose a pod to analyze…" \
        "$initial_filter"
  )" || return
  [[ -n "$choice" ]] || return
  pod="$(_podbor_kube_first_column "$choice")"

  _podbor_pod_analyze_binary || return
  analyzer=("${reply[@]}")
  analyzer_args=(
    --kubeconfig "${kube[1]}"
    --context "${kube[2]}"
    --namespace "${kube[3]}"
    --environment "${kube[4]}"
    --pod "$pod"
    --refresh "${KUBE_POD_ANALYZE_REFRESH:-5s}"
    --history-window "${KUBE_POD_ANALYZE_WINDOW:-24h}"
    --retention "${KUBE_POD_ANALYZE_RETENTION:-720h}"
    --chart-points "${KUBE_POD_ANALYZE_CHART_POINTS:-72}"
    --history-dir "${KUBE_POD_ANALYZE_HISTORY_DIR:-${XDG_STATE_HOME:-$HOME/.local/state}/podbor-kube/pod-metrics}"
  )
  [[ "${KUBE_POD_ANALYZE_ONCE:-0}" = "1" ]] && analyzer_args+=(--once)

  "${analyzer[1]}" "${analyzer_args[@]}"
}

_podbor_deploy_watch_configured_resources() {
  local config_file="${KUBE_DEPLOY_WATCH_RESOURCES_FILE:-${XDG_CONFIG_HOME:-$HOME/.config}/podbor-kube/deploy-watch.resources}"
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
_podbor_deploy_watch() {
  local environment="$1"
  local initial_filter="${2:-}"
  local -a kube monitor monitor_args configured_names group_resources missing_names workload_rows
  local rows picker_rows choice resource row candidate candidate_name configured_name
  local found

  _podbor_kube_env "$environment" || return
  kube=("${reply[@]}")
  _podbor_deploy_watch_configured_resources || return
  configured_names=("${reply[@]}")

  rows="$(
    _podbor_kubectl "${kube[1]}" "${kube[2]}" "${kube[3]}" \
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
      candidate="$(_podbor_kube_first_column "$row")"
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
      _podbor_kube_choose \
        "Podbor ${kube[4]} · ${kube[3]} · deployment watch · READ ONLY" \
        "Choose the private configured group or one workload…" \
        "$initial_filter"
  )" || return
  [[ -n "$choice" ]] || return
  resource="$(_podbor_kube_first_column "$choice")"

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

  _podbor_deploy_watch_binary || return
  monitor=("${reply[@]}")
  "${monitor[1]}" \
    --kubeconfig "${kube[1]}" \
    --context "${kube[2]}" \
    --namespace "${kube[3]}" \
    --environment "${kube[4]}" \
    "${monitor_args[@]}"
}

kpdev-logs() {
  _podbor_logs dev "$@"
}

kpstage-logs() {
  _podbor_logs stage "$@"
}

kpprod-logs() {
  _podbor_logs prod "$@"
}

kpdev-logs-save() {
  _podbor_logs dev "${1:-}" save
}

kpstage-logs-save() {
  _podbor_logs stage "${1:-}" save
}

kpprod-logs-save() {
  _podbor_logs prod "${1:-}" save
}

kpdev-logs-multi() {
  _podbor_logs_multi dev "$@"
}

kpstage-logs-multi() {
  _podbor_logs_multi stage "$@"
}

kpprod-logs-multi() {
  _podbor_logs_multi prod "$@"
}

kpdev-cron-run() {
  _podbor_cron dev "$@"
}

kpstage-cron-run() {
  _podbor_cron stage "$@"
}

kpprod-cron-run() {
  _podbor_cron prod "$@"
}

kpdev-jobs() {
  _podbor_jobs dev "$@"
}

kpstage-jobs() {
  _podbor_jobs stage "$@"
}

kpprod-jobs() {
  _podbor_jobs prod "$@"
}

kpdev-jobs-save() {
  _podbor_jobs dev "${1:-}" save
}

kpstage-jobs-save() {
  _podbor_jobs stage "${1:-}" save
}

kpprod-jobs-save() {
  _podbor_jobs prod "${1:-}" save
}

kpdev-exec() {
  _podbor_exec dev "$@"
}

kpstage-exec() {
  _podbor_exec stage "$@"
}

kpprod-exec() {
  _podbor_exec prod "$@"
}

kpdev-log-search() {
  _podbor_log_search dev "$@"
}

kpstage-log-search() {
  _podbor_log_search stage "$@"
}

kpprod-log-search() {
  _podbor_log_search prod "$@"
}

kpdev-deploy-watch() {
  _podbor_deploy_watch dev "$@"
}

kpstage-deploy-watch() {
  _podbor_deploy_watch stage "$@"
}

kpprod-deploy-watch() {
  _podbor_deploy_watch prod "$@"
}

kpdev-pod-analyze() {
  _podbor_pod_analyze dev "$@"
}

kpstage-pod-analyze() {
  _podbor_pod_analyze stage "$@"
}

kpprod-pod-analyze() {
  _podbor_pod_analyze prod "$@"
}

kpdev-pod-restart() {
  _podbor_pod_restart dev "$@"
}

kpstage-pod-restart() {
  _podbor_pod_restart stage "$@"
}

kpprod-pod-restart() {
  _podbor_pod_restart prod "$@"
}
