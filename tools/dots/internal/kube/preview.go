package kube

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"dots/native/internal/kube/logwatch"
	"dots/native/internal/kube/tui"
)

func preview(args []string) error {
	screen := "selector"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		screen = args[0]
		args = args[1:]
	}
	flags := flag.NewFlagSet("preview", flag.ContinueOnError)
	width := flags.Int("width", 80, "terminal columns")
	height := flags.Int("height", 24, "terminal rows")
	plain := flags.Bool("plain", false, "disable ANSI styling")
	forceColor := flags.Bool("color", false, "render Ayu colors even when redirected")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return exitError(2)
	}
	if flags.NArg() != 0 || *width < 20 || *width > 240 || *height < 8 || *height > 100 {
		return fmt.Errorf("Preview expects width 20..240 and height 8..100")
	}
	_, noColor := os.LookupEnv("NO_COLOR")
	color := !*plain && !noColor && (*forceColor || tui.ColorEnabled(os.Stdout))
	frame := tui.Frame{Environment: "stage", Context: "dots-stage", Namespace: "example-app", Mode: "READ ONLY", Width: *width, Height: *height, Color: color, Footer: "↑↓ move · / search · Tab mark · Enter open · Esc back · Ctrl+C cancel"}
	switch screen {
	case "setup":
		frame.Title = "Настройка · 1/3 · Файлы"
		frame.Mode = "LOCAL CONFIG"
		frame.Environment = ""
		frame.Context = ""
		frame.Namespace = ""
		frame.Body = "Найдены ваши kubeconfig. Выберите один или несколько файлов.\nОкружения могут использовать разные файлы — объединять их не нужно.\nИсходные файлы и ключи не изменяются.\n\nПоиск › \n\n› ● stage.yaml     ~/.kube/config\n  ● prod.yaml      ~/.kube/config\n  ○ development.yaml ~/.kube/config\n\nCtrl+O — другая папка или файл"
		frame.Footer = "Выбрано: 2 · Space отметить · Ctrl+A все · Enter дальше · Esc выйти"
	case "selector":
		frame.Title = "Pods"
		frame.Body = "Search › api\n\n  NAME                  READY  STATUS       RESTARTS  AGE\n" + tui.Paint("› api-7c9df-abc          2/2    Running      0         12m", tui.Accent+tui.Bold, color) + "\n  api-7c9df-def          1/2    Pending      1         30s\n  worker-6d8c2-ghi       1/1    Running      0         2h\n\n2 matching pods · choose one or mark several"
	case "logs":
		frame = logwatch.Preview(*width, *height, color)
	case "deploy":
		frame.Title = "Deployment watch"
		frame.Body = tui.Paint("HEALTHY · 3/3 replicas ready", tui.Green+tui.Bold, color) + "\n\nWORKLOADS\n  NAME             DESIRED  UPDATED  READY  AVAILABLE\n  api              3        3        3      3\n\nPODS\n  api-abc          Running  2/2      0 restarts\n  api-def          Running  2/2      0 restarts\n  api-ghi          Running  2/2      0 restarts\n\nEVENTS\n  Normal · rollout complete"
		frame.Footer = "Updates every 2s · Esc / q back · Ctrl+C cancel"
	case "analyze":
		frame.Title = "Pod analytics"
		frame.Body = "api-abc · Running · 2/2 ready · 0 restarts\n\n  CONTAINER  CPU       RAM        CPU/REQ  RAM/REQ\n  app        120m      256 MiB    24%      50%\n  sidecar    10m       32 MiB     10%      25%\n\nCPU history   ▁▂▂▃▄▃▂▂▁\nRAM history   ▂▂▃▃▄▄▃▃▂\n\nNo warnings · metrics and process samples are observational"
		frame.Footer = "Updates every 2s · Esc / q back · Ctrl+C cancel"
	default:
		return fmt.Errorf("Unknown preview screen: %s (setup, selector, logs, deploy, analyze)", screen)
	}
	fmt.Print(frame.Render())
	return nil
}
