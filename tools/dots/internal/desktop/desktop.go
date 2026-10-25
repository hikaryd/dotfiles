// Package desktop implements the desktop group of dots-tools.
package desktop

import (
	"fmt"
	"os"
	"path/filepath"
)

// Run dispatches desktop commands. root is the checkout containing config/ and
// scripts/ assets; args starts with a desktop command, not the group name.
func Run(root string, args []string) int {
	if len(args) == 0 {
		return menu(root)
	}
	command := args[0]
	args = args[1:]
	if command != "nu" {
		for _, arg := range args {
			if has(arg, "--help", "-h") {
				return commandHelp(command)
			}
		}
	}
	switch command {
	case "menu":
		return menuRun(root, args)
	case "help", "--help", "-h":
		printHelp()
		return 0
	case "clip":
		return clipRun(args)
	case "refs":
		return refsRun(args)
	case "projects":
		return projectsRun(args)
	case "sessions":
		return sessionsRun(args)
	case "private":
		return privateRun(root, args)
	case "style":
		return styleRun(args)
	case "tmux-style":
		return tmuxStyleRun(args)
	case "ghostty-reload":
		if len(args) > 0 {
			return usageError(command)
		}
		return ghosttyReload()
	case "herdr-status":
		if len(args) > 0 {
			return usageError(command)
		}
		return herdrStatus()
	case "status":
		return statusRun(args)
	case "doctor":
		return doctorRun(root, args)
	case "notify":
		if len(args) == 0 {
			return commandHelp(command)
		}
		return notifyRun(args)
	case "nu":
		return nuRun(root, args)
	case "ports":
		code := interactive("lsof", "-nP", "-iTCP", "-sTCP:LISTEN")
		if code == 1 {
			return 0
		}
		return code
	}
	fmt.Fprintf(os.Stderr, "Неизвестная команда: %s; dots help\n", command)
	return 2
}
func commandHelp(command string) int {
	if command == "menu" {
		printMenuHelp()
		return 0
	}
	help := map[string]string{"projects": "list [--json] | refresh | open [target]", "sessions": "list [--json] | open [target]", "clip": "[json|compact|base64|jwt|time|redact] [--stdin] [--stdout|--write] [--keys KEY,...]", "refs": "[--pane ID] [--lines 1..5000] [--stdin] [--list|--json]", "private": "[init|check]", "style": "[glass|focus|presentation|status]", "tmux-style": "glass|focus|presentation|--current", "status": "[--all] [--json|--interactive]", "doctor": "[--json] [--strict]", "notify": "[--test] [--status 0..255] [--seconds N] [--pane ID]", "herdr-status": "", "ghostty-reload": "", "menu": "", "ports": ""}
	if s, ok := help[command]; ok {
		fmt.Printf("usage: dots %s %s\n", command, s)
		return 0
	}
	return usageError(command)
}
func pickMenu(title string, choices [][2]string) (string, error) {
	lines := []string{}
	for _, c := range choices {
		lines = append(lines, c[0]+" · "+c[1])
	}
	i, e := selectLine(lines, title)
	if e != nil {
		return "", e
	}
	if i < 0 {
		return "", nil
	}
	return choices[i][0], nil
}
func menu(root string) int {
	choice, err := pickMenu(menuTitle(menuContext()), menuChoices())
	if err != nil {
		return fail("dots menu", err)
	}
	switch choice {
	case "":
		return 0
	case "git":
		return interactive("lazygit")
	case "status":
		if isTerminal(os.Stdin) {
			return Run(root, []string{"status", "--interactive"})
		}
	case "style":
		style, err := pickMenu("Стиль", [][2]string{{"glass", "Текущее стекло"}, {"focus", "Без прозрачности и движения"}, {"presentation", "Крупнее, без путей в prompt"}})
		if err != nil {
			return fail("dots style", err)
		}
		if style == "" {
			return 0
		}
		return Run(root, []string{"style", style})
	case "config":
		configs := [][2]string{{"zsh", "config/zsh/.zshrc"}, {"tmux", "config/tmux/tmux.conf"}, {"herdr", "config/herdr/config.toml"}, {"ghostty", "config/ghostty/config"}, {"nvim", "config/nvim/init.lua"}, {"sesh", "config/sesh/sesh.toml"}, {"atuin", "config/atuin/config.toml"}}
		key, err := pickMenu("Редактировать конфиг", configs)
		if err != nil {
			return fail("dots config", err)
		}
		for _, c := range configs {
			if key == c[0] {
				return interactive("nvim", filepath.Join(root, c[1]))
			}
		}
		return 0
	}
	return Run(root, []string{choice})
}
