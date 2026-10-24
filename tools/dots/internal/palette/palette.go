// Package palette generates application themes from palette/ayu.toml.
package palette

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

var toolNames = []string{"ghostty", "starship", "nushell", "sketchybar", "tmux", "fresh", "lazygit", "thorium", "yazi", "pi", "jcode"}

type palette map[string]map[string]string

type generator struct {
	root, variant string
	p             palette
	syntaxKeys    []string
	out           io.Writer
}

// Run implements dots-tools palette [--variant dark|mirage] [--tool TOOL] [--dry-run].
func Run(root string, args []string) int { return run(root, args, os.Stdout, os.Stderr) }

const usage = "usage: dots-tools palette [-h] [--variant {dark,mirage}]\n                          [--tool {ghostty,starship,nushell,sketchybar,tmux,fresh,lazygit,thorium,yazi,pi,jcode}]\n                          [--dry-run]\n"

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
			fmt.Fprint(out, usage+"\nGenerate configs from ayu palette\n\noptions:\n  -h, --help            show this help message and exit\n  --variant {dark,mirage}\n  --tool TOOL           Generate for one tool only\n  --dry-run             Print what would be done\n")
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
	// meta has a different shape from the variant tables.
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
	g.syntaxKeys, err = syntaxOrder(data, variant)
	if err != nil {
		return fail(fmt.Errorf("cannot parse palette/ayu.toml"))
	}
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

func (g generator) generate(name string) error {
	switch name {
	case "ghostty":
		path := g.path("ghostty/config")
		text, err := readText(path)
		if err != nil {
			return err
		}
		bg, err := g.color("editor", "bg")
		if err != nil {
			return err
		}
		theme := "Ayu"
		if g.variant == "mirage" {
			theme = "Ayu Mirage"
		}
		text = replace(text, `(?m)^theme\s*=\s*.*$`, "theme = "+theme)
		text = replace(text, `(?m)^background\s*=\s*.*$`, "background = "+strings.TrimLeft(bg, "#"))
		if err := os.WriteFile(path, []byte(text), 0666); err != nil {
			return err
		}
		fmt.Fprintf(g.out, "  ghostty: %s\n", path)
		return nil
	default:
		return g.generateTool(name)
	}
}
