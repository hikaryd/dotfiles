package kube

import (
	"fmt"
	"strings"

	"dots/native/internal/tui"
)

func (s *session) menu() error {
	state, _ := s.configurationState()
	rows := []string{"setup\tНастроить файлы и окружения", "config\tПоказать подключения — без ключей", "doctor\tЛокальная диагностика — без запросов к кластеру"}
	for _, environment := range state.Environments {
		rows = append(rows, fmt.Sprintf("%s\t%s\t%s\t%s", environment.Environment, environment.Context, environment.Namespace, environment.Status))
	}
	selected, err := s.choose(rows, "Kubernetes · home", "", false)
	if err != nil {
		if err == tui.ErrCanceled {
			return nil
		}
		return err
	}
	entry := firstColumn(selected[0])
	switch entry {
	case "setup":
		s.action = "setup"
		return s.setup(nil)
	case "config":
		return s.configuration(nil, false)
	case "doctor":
		return s.configuration(nil, true)
	}
	if err := s.configure(invocation{entry, "namespace", "kubectl"}); err != nil {
		return err
	}
	choices := []string{"pods\tList pods · READ ONLY", "logs\tChoose pod logs · READ ONLY", "logs-multi\tFollow multiple pods · READ ONLY", "jobs\tRecent Job logs · READ ONLY", "deploy-watch\tDeployment dashboard · READ ONLY", "pod-analyze\tCPU/RAM and processes · READ ONLY", "exec\tExecute in a container · ACTION", "cron-run\tRun a CronJob · ACTION", "scale\tChange replicas · ACTION", "pod-restart\tRestart controller-managed pods · ACTION"}
	selected, err = s.choose(choices, "Choose an action", "", false)
	if err != nil {
		if err == tui.ErrCanceled {
			return nil
		}
		return err
	}
	action := firstColumn(selected[0])
	if action == "pods" {
		s.banner(s.target.namespace)
		return s.execute(nil, "get", "pods")
	}
	return s.workflow(strings.TrimSpace(action), nil)
}
