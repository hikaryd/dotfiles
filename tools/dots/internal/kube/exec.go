package kube

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"dots/native/internal/tui"
)

func dedent(input string) string {
	lines := strings.Split(input, "\n")
	margin := -1
	for _, line := range lines {
		if strings.Trim(line, " \t") == "" {
			continue
		}
		width := len(line) - len(strings.TrimLeft(line, " \t"))
		if margin < 0 || width < margin {
			margin = width
		}
	}
	if margin > 0 {
		for i, line := range lines {
			if strings.Trim(line, " \t") == "" {
				lines[i] = ""
			} else {
				lines[i] = line[margin:]
			}
		}
	}
	return strings.Join(lines, "\n")
}
func (s *session) exec(args []string) error {
	target := ""
	chooseMode := len(args) == 0
	if len(args) == 0 || args[0] == "--" {
		items, err := s.resources("pods,deployments,statefulsets,daemonsets")
		if err != nil {
			return err
		}
		rows := []string{}
		for _, item := range items {
			rows = append(rows, resourceName(item))
		}
		sort.Strings(rows)
		selected, err := s.choose(rows, "choose where to execute", "", false)
		if err != nil {
			return err
		}
		target = selected[0]
	} else {
		target = args[0]
		args = args[1:]
		chooseMode = len(args) == 0
	}
	if strings.HasPrefix(target, "-") {
		fmt.Fprintln(os.Stderr, "Exec target must be a resource name, not an option")
		return exitError(2)
	}
	if chooseMode {
		return s.execInteractive(target)
	}
	if len(args) < 2 || args[0] != "--" {
		fmt.Fprintln(os.Stderr, "Usage: kp"+s.target.environment+"-exec [target] -- <command> [args...]")
		return exitError(2)
	}
	command := args[1:]
	if isTerminal(os.Stdin) {
		return s.execute(os.Stdin, append([]string{"exec", "-it", target, "--"}, command...)...)
	}
	data, err := readInput(s.ctx, os.Stdin)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Executing in %s (%s/%s); common input indentation removed\n", target, s.target.environment, s.target.namespace)
	return s.execute(strings.NewReader(dedent(data)), append([]string{"exec", "-i", target, "--"}, command...)...)
}
func (s *session) execInteractive(target string) error {
	selected, err := s.choose([]string{"bash\tinteractive shell", "python\tpaste multiline script · auto-dedent"}, target+" · what to execute", "", false)
	if err != nil {
		return err
	}
	mode := firstColumn(selected[0])
	selected, err = s.choose([]string{"default\tcontainer WORKDIR", "..\tparent of container WORKDIR", "../..\ttwo levels above container WORKDIR", "custom\tenter a path"}, target+" · working directory", "", false)
	if err != nil {
		return err
	}
	workdir := firstColumn(selected[0])
	switch workdir {
	case "default":
		workdir = ""
	case "custom":
		workdir, err = s.input("Working directory in "+target, "")
		if err != nil {
			return err
		}
		if workdir == "" {
			return errors.New("Working directory cannot be empty")
		}
	}
	if mode == "bash" {
		command := []string{"bash"}
		if workdir != "" {
			command = []string{"bash", "-lc", `cd -- "$1" && exec bash -l`, "kube-exec", workdir}
		}
		return s.execute(os.Stdin, append([]string{"exec", "-it", target, "--"}, command...)...)
	}
	script, err := tui.Editor(s.ctx, s.frame("Python in "+target))
	if err != nil {
		return err
	}
	if strings.TrimSpace(script) == "" {
		return errors.New("Empty Python script; nothing executed")
	}
	command := []string{"python", "-"}
	if workdir != "" {
		command = append([]string{"sh", "-c", `cd "$1" && shift && exec "$@"`, "kube-exec", workdir}, command...)
	}
	return s.execute(strings.NewReader(dedent(script)), append([]string{"exec", "-i", target, "--"}, command...)...)
}
