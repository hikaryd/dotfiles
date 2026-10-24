// dots-kube is the shareable Kubernetes-only binary. It needs no dotfiles
// checkout or shell configuration; all cluster settings live in kubeconfig.
package main

import (
	"os"
	"path/filepath"

	"dots/native/internal/kube"
)

func run(name string, args []string) int {
	name = filepath.Base(name)
	if kube.IsCommand(name) {
		return kube.Run(append([]string{name}, args...))
	}
	// Dashboards are children of the same executable. Both the standalone and
	// integrated runtimes accept the group prefix used by that internal protocol.
	if len(args) > 0 && args[0] == "kube" {
		args = args[1:]
	}
	return kube.Run(args)
}
func main() { os.Exit(run(os.Args[0], os.Args[1:])) }
