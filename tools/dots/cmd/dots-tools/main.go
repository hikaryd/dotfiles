// dots-tools is the native runtime shared by the dotfiles' command aliases.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"dots/native/internal/agents"
	"dots/native/internal/bootstrap"
	"dots/native/internal/codexconfig"
	"dots/native/internal/desktop"
	"dots/native/internal/kube"
	"dots/native/internal/palette"
	"dots/native/internal/proxy"
	"dots/native/internal/setup"
	"dots/native/internal/vpn"
)

// Set by the build script. DOTS_ROOT overrides it when the checkout is moved.
var defaultRoot string

func repositoryRoot() string {
	if root := os.Getenv("DOTS_ROOT"); root != "" {
		return root
	}
	if defaultRoot != "" {
		return defaultRoot
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, "dots")
}

func dispatch(root, name string, args []string) int {
	name = filepath.Base(name)
	if kube.IsCommand(name) {
		return kube.Run(append([]string{name}, args...))
	}
	switch name {
	case "dots":
		return desktop.Run(root, args)
	case "dots-nu":
		return desktop.Run(root, append([]string{"nu"}, args...))
 case "dots-kube":
  return kube.Run(args)
	case "ai_helper":
		return agents.Run(root, append([]string{"ai"}, args...))
	case "dots-vpn":
		return runVPN(root, args)
	case "pi-install", "pi-install.sh":
		return bootstrap.Run(root, args)
	case "dots-task-titles":
		return agents.Run(root, append([]string{"task-titles"}, args...))
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println("Usage: dots-tools desktop|setup|codexconfig|agents|proxy|palette|vpn|kube <command> [args]")
		fmt.Println("Aliases: dots, dots-nu, ai_helper, dots-vpn, dots-task-titles")
		return 0
	}
	switch args[0] {
	case "desktop":
		return desktop.Run(root, args[1:])
	case "setup":
		if len(args) > 1 && args[1] == "pi-install" {
			return bootstrap.Run(root, args[2:])
		}
		return setup.Run(root, args[1:])
	case "codexconfig":
		return codexconfig.Run(root, args[1:])
	case "agents":
		return agents.Run(root, args[1:])
	case "proxy":
		return proxy.Run(root, args[1:])
	case "palette":
		return palette.Run(root, args[1:])
	case "vpn":
		return runVPN(root, args[1:])
	case "kube":
		return kube.Run(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "dots-tools: unknown group %q\n", args[0])
		return 2
	}
}

func runVPN(root string, args []string) int {
	if len(args) > 0 && (args[0] == "awg-params" || args[0] == "tuning-env") {
		return bootstrap.RunVPNHelper(root, args)
	}
	return vpn.Run(root, args)
}

func main() {
	os.Exit(dispatch(repositoryRoot(), os.Args[0], os.Args[1:]))
}
