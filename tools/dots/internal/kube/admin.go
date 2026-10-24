package kube

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"dots/native/internal/kube/kubeui"
	"dots/native/internal/tui"
)

type environmentReport struct {
	Environment string   `json:"environment"`
	Context     string   `json:"context"`
	Namespace   string   `json:"namespace"`
	Cluster     string   `json:"cluster,omitempty"`
	User        string   `json:"user,omitempty"`
	Status      string   `json:"status"`
	Problem     string   `json:"problem,omitempty"`
	Sources     []string `json:"sources,omitempty"`
}
type configurationReport struct {
	Sources          []string            `json:"sources"`
	CurrentContext   string              `json:"current_context"`
	Environments     []environmentReport `json:"environments"`
	KubectlAvailable bool                `json:"kubectl_available"`
}

func report(config *inventory) configurationReport {
	result := configurationReport{Sources: []string{}, CurrentContext: config.current, Environments: []environmentReport{}}
	_, err := exec.LookPath("kubectl")
	result.KubectlAvailable = err == nil
	for _, file := range config.files {
		result.Sources = append(result.Sources, file.path)
	}
	for _, environment := range environments {
		row := environmentReport{Environment: environment, Context: aliasContext(environment), Namespace: "—", Status: "unconfigured"}
		if item, ok := config.byName[row.Context]; ok {
			row.Namespace = item.Context.Namespace
			if row.Namespace == "" {
				row.Namespace = "default"
			}
			row.Cluster = item.Context.Cluster
			row.User = item.Context.User
			row.Status = "ready"
			if _, err := config.context(row.Context); err != nil {
				row.Status = "error"
				row.Problem = err.Error()
			}
		}
		result.Environments = append(result.Environments, row)
	}
	return result
}
func (s *session) configuration(args []string, doctor bool) error {
	flags := flag.NewFlagSet("config", flag.ContinueOnError)
	asJSON := flags.Bool("json", false, "print metadata only as JSON")
	strict := flags.Bool("strict", false, "doctor: require all four environments")
	path := flags.String("kubeconfig", "", "inspect a single file instead of the standard loading chain")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return exitError(2)
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "Usage: dots-kube config|doctor [--json] [--kubeconfig FILE] [--strict]")
		return exitError(2)
	}
	if *path != "" {
		s.useSources([]string{*path})
	}
	state, err := s.configurationState()
	if err != nil {
		return err
	}
	if *asJSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(state); err != nil {
			return err
		}
	} else {
		s.printConfiguration(state, doctor)
	}
	if doctor {
		ready := 0
		failed := !state.KubectlAvailable
		for _, row := range state.Environments {
			if row.Status == "ready" {
				ready++
			}
			if row.Status == "error" || *strict && row.Status != "ready" {
				failed = true
			}
		}
		if ready == 0 {
			failed = true
		}
		if failed {
			return exitError(1)
		}
	}
	return nil
}
func (s *session) printConfiguration(state configurationReport, doctor bool) {
	width, _ := tui.Size(os.Stdout)
	color := tui.ColorEnabled(os.Stdout)
	var body strings.Builder
	body.WriteString("Каждое окружение использует собственные файлы. Ключи не показываются.\n\n")
	for _, row := range state.Environments {
		status, tone := "✓ готово", tui.Green
		if row.Status == "error" {
			status, tone = "! требуется исправление", tui.Red
		}
		if row.Status == "unconfigured" {
			status, tone = "не настроено", tui.Dim
		}
		body.WriteString(tui.Paint(strings.ToUpper(row.Environment)+"  "+status, tone+tui.Bold, color) + "\n")
		if row.Status != "unconfigured" {
			for _, path := range row.Sources {
				body.WriteString("  Файл: " + s.displayPath(path) + "\n")
			}
			body.WriteString("  Контекст: " + tui.Clean(row.Context) + "\n  Namespace: " + tui.Clean(row.Namespace) + "\n")
		}
		if row.Problem != "" {
			body.WriteString("  " + tui.CleanText(row.Problem) + "\n")
		}
		body.WriteString("\n")
	}
	if state.CurrentContext != "" {
		body.WriteString("Обычный kubectl: " + tui.Clean(state.CurrentContext) + " · текущий контекст не переключается\n")
	}
	if state.KubectlAvailable {
		body.WriteString("✓ kubectl найден")
	} else {
		body.WriteString("! Для запросов установите kubectl; настройка файлов работает без него.")
	}
	title := "Окружения и подключения"
	if doctor {
		title = "Диагностика · без запросов к кластеру"
	}
	if knownEnvironment(s.target.environment) {
		title = "Подключение · " + strings.ToUpper(s.target.environment)
	}
	frame := kubeui.Frame(title, "", "", "", "LOCAL CONFIG")
	frame.Width, frame.Color, frame.Body = width, color, body.String()
	frame.Footer = "Изменить: dots-kube setup · проверить: dots-kube doctor · JSON: config --json"
	fmt.Print(frame.Render())
}
