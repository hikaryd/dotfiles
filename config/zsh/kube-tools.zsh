# Compatibility cleanup for already-running zsh sessions. Kubernetes commands
# are native dots-kube aliases. This file does not read private settings, rewrite
# KUBECONFIG, build binaries or contact clusters.

local _dots_kube_name _dots_kube_env _dots_kube_action
# Remove only helpers shipped by the old module, not private _kube_* functions.
unfunction \
  _kube_validate_target_args _kube_kubectl _kube_prod_confirm _kube_kubectl_is_read_only \
  _kube_run _kube_env _kube_choose _kube_first_column _kube_colorize_logs \
  _kube_stream_logs _kube_stream_logs_to_file _kube_duration_seconds _kube_job_logs_to_file \
  _kube_log_file _kube_search_one_pod_logs _kube_search_pod_logs _kube_matching_pods \
  _kube_log_search _kube_dedent _kube_exec_dedented _kube_exec_dedented_at \
  _kube_choose_exec_workdir _kube_exec _kube_logs _kube_pod_restart_one \
  _kube_follow_restarted_pods _kube_pod_restart _kube_tool_binary _kube_log_watch_binary \
  _kube_logs_multi _kube_cron _kube_jobs _kube_deploy_watch_binary _kube_pod_analyze_binary \
  _kube_pod_analyze _kube_scale _kube_deploy_watch_configured_resources _kube_deploy_watch \
  2>/dev/null || true
for _dots_kube_env in dev stage preprod prod; do
  for _dots_kube_name in "k$_dots_kube_env" "k$_dots_kube_env-connect" "kp$_dots_kube_env"; do
    unfunction "$_dots_kube_name" 2>/dev/null || true
    unalias "$_dots_kube_name" 2>/dev/null || true
  done
  for _dots_kube_action in logs logs-save logs-multi cron-run jobs jobs-save exec log-search deploy-watch scale pod-analyze pod-restart rollout cron; do
    _dots_kube_name="kp$_dots_kube_env-$_dots_kube_action"
    unfunction "$_dots_kube_name" 2>/dev/null || true
    unalias "$_dots_kube_name" 2>/dev/null || true
  done
done
unfunction kube-tools-help 2>/dev/null || true
unalias kube-tools-help 2>/dev/null || true
unset _dots_kube_name _dots_kube_env _dots_kube_action
