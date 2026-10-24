// Package kubeui adapts Kubernetes target metadata to the shared terminal UI.
package kubeui

import (
	"strings"

	"dots/native/internal/tui"
)

func Frame(title, environment, context, namespace, mode string) tui.Frame {
	if mode == "" {
		mode = "READ ONLY"
	}
	tone := tui.Green
	if mode == "ACTION" {
		tone = tui.Yellow
	}
	if environment == "prod" {
		tone = tui.Red
	}
	if context == "" {
		context = "—"
	}
	if namespace == "" {
		namespace = "—"
	}
	subtitle := "context: " + context + "   namespace: " + namespace
	if mode == "LOCAL CONFIG" && context == "—" && namespace == "—" {
		subtitle = "Локальная настройка · без запросов к кластеру"
	}
	return tui.Frame{Brand: "dots kube", Title: title, Targets: []string{strings.ToUpper(environment)}, Subtitle: subtitle, Mode: mode, Tone: tone}
}
