// Package palette генерирует темы приложений из palette/ayu.toml.
package palette

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

var toolNames = []string{"ghostty", "shitty", "starship", "nushell", "tmux", "lazygit", "yazi", "jcode", "nvim", "zsh", "gitk", "fastfetch"}

type palette map[string]map[string]string

type generator struct {
	root, variant string
	p             palette
	out           io.Writer
}

func Run(root string, args []string) int { return run(root, args, os.Stdout, os.Stderr) }

const usage = "Использование: dots-tools palette [--variant dark|mirage] [--tool ИМЯ] [--dry-run]\n"

func run(root string, args []string, out, stderr io.Writer) int {
	variant, tool, dry := "dark", "", false
	argError := func(message string) int {
		fmt.Fprint(stderr, usage)
		fmt.Fprintln(stderr, "dots-tools palette: error: "+message)
		return 2
	}
	for i := 0; i < len(args); i++ {
		arg, value, hasValue := strings.Cut(args[i], "=")
		if arg == "-h" {
			arg = "--help"
		}
		name := ""
		if strings.HasPrefix(arg, "--") && len(arg) > 2 {
			for _, option := range []string{"--variant", "--tool", "--dry-run", "--help"} {
				if strings.HasPrefix(option, arg) {
					if name != "" {
						return argError("ambiguous option")
					}
					name = option
				}
			}
		}
		switch name {
		case "--help":
			if hasValue {
				return argError("--help does not take a value")
			}
			fmt.Fprint(out, usage+"\nГенерирует темы из palette/ayu.toml.\n  --variant  dark (по умолчанию) или mirage\n  --tool     только один: "+strings.Join(toolNames, ", ")+"\n  --dry-run  показать, без записи\n")
			return 0
		case "--dry-run":
			if hasValue {
				return argError("--dry-run does not take a value")
			}
			dry = true
		case "--variant", "--tool":
			if !hasValue {
				i++
				if i >= len(args) || strings.HasPrefix(args[i], "-") {
					return argError(name + " requires one argument")
				}
				value = args[i]
			}
			if name == "--variant" {
				if value != "dark" && value != "mirage" {
					return argError("--variant: choose dark or mirage")
				}
				variant = value
			} else {
				valid := false
				for _, n := range toolNames {
					if n == value {
						valid = true
					}
				}
				if !valid {
					return argError("--tool: invalid choice")
				}
				tool = value
			}
		default:
			if args[i] == "--" && i == len(args)-1 {
				continue
			}
			return argError("unrecognized argument")
		}
	}
	fail := func(err error) int { fmt.Fprintf(stderr, "dots-tools palette: %v\n", err); return 1 }
	root, err := filepath.Abs(root)
	if err != nil {
		return fail(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "palette", "ayu.toml"))
	if err != nil {
		return fail(err)
	}
	var all map[string]palette
	// У meta другая структура, чем у таблиц вариантов.
	var document struct {
		Dark   palette `toml:"dark"`
		Mirage palette `toml:"mirage"`
	}
	if err := toml.Unmarshal(data, &document); err != nil {
		return fail(fmt.Errorf("cannot parse palette/ayu.toml"))
	}
	all = map[string]palette{"dark": document.Dark, "mirage": document.Mirage}
	if all[variant] == nil {
		return fail(fmt.Errorf("palette variant %s is missing", variant))
	}
	g := generator{root: root, variant: variant, p: all[variant], out: out}
	targets := toolNames
	if tool != "" {
		targets = []string{tool}
	}
	fmt.Fprintf(out, "Generating ayu %s configs:\n", variant)
	for _, name := range targets {
		if dry {
			fmt.Fprintf(out, "  [dry-run] %s\n", name)
			continue
		}
		if err := g.generate(name); err != nil {
			return fail(err)
		}
	}
	// bat, delta и превью yazi читают собранный кеш, а не tmtheme.xml напрямую.
	// bat и delta закреплены за ayu-dark, поэтому кеш пересобирается только для dark.
	if !dry && variant == "dark" && (tool == "" || tool == "yazi") {
		if bat, err := exec.LookPath("bat"); err == nil {
			if output, err := exec.Command(bat, "cache", "--build").CombinedOutput(); err != nil {
				fmt.Fprintf(stderr, "dots-tools palette: bat cache --build: %v\n%s", err, output)
			} else {
				fmt.Fprintln(out, "  bat: cache rebuilt")
			}
		}
	}
	fmt.Fprintln(out, "Done.")
	return 0
}

func (g generator) color(section, key string) (string, error) {
	value, ok := g.p[section][key]
	if !ok {
		return "", fmt.Errorf("palette is missing %s.%s", section, key)
	}
	return value, nil
}

func (g generator) path(relative string) string {
	return filepath.Join(g.root, "config", filepath.FromSlash(relative))
}

func readText(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("cannot read %s: invalid UTF-8", path)
	}
	return strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n"), err
}

func replace(text, pattern, replacement string) string {
	return regexp.MustCompile(pattern).ReplaceAllStringFunc(text, func(string) string { return replacement })
}

type colorField struct{ name, section, key string }

var terminalKeys = []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white", "bright_black", "bright_red", "bright_green", "bright_yellow", "bright_blue", "bright_magenta", "bright_cyan", "bright_white"}

func (g generator) generate(name string) error {
	switch name {
	case "ghostty":
		return g.prependColors(name, "ghostty/config",
			`(?m)^[ \t]*(theme|background|foreground|selection-background|selection-foreground|cursor-color|unfocused-split-fill|palette)[ \t]*=.*\n?`,
			[]colorField{
				{"background", "editor", "bg"}, {"foreground", "editor", "fg"},
				{"selection-background", "ui", "selection"}, {"selection-foreground", "editor", "fg"},
				{"cursor-color", "common", "accent"}, {"unfocused-split-fill", "editor", "bg"},
			},
			func(f string, v string) string { return f + " = " + strings.TrimPrefix(v, "#") },
			func(i int, v string) string { return fmt.Sprintf("palette = %d=%s", i, v) })
	case "shitty":
		// Ключи TOML верхнего уровня обязаны стоять до первой таблицы, поэтому цвета идут в начало.
		if err := g.prependColors(name, "shitty/shitty.toml",
			`(?m)^[ \t]*(bg|fg|cr|color[0-9]+)[ \t]*=.*\n?`,
			[]colorField{{"bg", "editor", "bg"}, {"fg", "editor", "fg"}, {"cr", "common", "accent"}},
			func(f string, v string) string { return fmt.Sprintf("%s = %q", f, v) },
			func(i int, v string) string { return fmt.Sprintf("color%d = %q", i, v) }); err != nil {
			return err
		}
		// Живой конфиг shitty — сгенерированная копия, а не симлинк на репозиторий.
		if dots, err := exec.LookPath("dots"); err == nil {
			_ = exec.Command(dots, "shitty-config").Run()
		}
		return nil
	default:
		return g.generateTool(name)
	}
}

func (g generator) prependColors(name, relative, stale string, fields []colorField, field func(string, string) string, palette func(int, string) string) error {
	var lines []string
	for _, f := range fields {
		value, err := g.color(f.section, f.key)
		if err != nil {
			return err
		}
		lines = append(lines, field(f.name, value))
	}
	for i, key := range terminalKeys {
		value, err := g.color("terminal", key)
		if err != nil {
			return err
		}
		lines = append(lines, palette(i, value))
	}
	path := g.path(relative)
	text, err := readText(path)
	if err != nil {
		return err
	}
	text = strings.Join(lines, "\n") + "\n" + replace(text, stale, "")
	if err := os.WriteFile(path, []byte(text), 0666); err != nil {
		return err
	}
	fmt.Fprintf(g.out, "  %s: %s\n", name, path)
	return nil
}
