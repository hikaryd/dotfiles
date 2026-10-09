// Package codexconfig manages the portable subset of a mixed Codex TOML file.
package codexconfig

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Run executes apply, sync, or remove. All asset paths are explicit CLI options;
// root is accepted for the multicall dispatch contract.
func Run(root string, args []string) int { return run(args, os.Stdout, os.Stderr) }
func run(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: codex-config {apply,sync,remove} ...")
		return 2
	}
	if args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: codex-config {apply,sync,remove} ...")
		return 0
	}
	command := args[0]
	if command != "apply" && command != "remove" && command != "sync" {
		fmt.Fprintln(errOut, "codex-config: invalid command")
		return 2
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	var diagnostics bytes.Buffer
	fs.SetOutput(&diagnostics)
	target := fs.String("target", "", "target config path")
	var template, source, home, binary string
	if command == "sync" {
		home, _ = os.UserHomeDir()
		fs.StringVar(&source, "source", "", "live config path")
		fs.StringVar(&home, "home", home, "home path to sanitize")
	} else {
		fs.StringVar(&template, "template", "", "template config path")
	}
	if command == "apply" {
		fs.StringVar(&binary, "codex-bin", "", "Codex executable for strict validation")
	}
	if e := fs.Parse(args[1:]); e != nil {
		if e == flag.ErrHelp {
			fmt.Fprint(out, diagnostics.String())
			return 0
		}
		// A malformed argument can itself contain a credential.
		fmt.Fprintln(errOut, "codex-config: invalid command arguments")
		return 2
	}
	if fs.NArg() != 0 || *target == "" || (command == "sync" && source == "") || (command != "sync" && template == "") {
		fmt.Fprintln(errOut, "codex-config: required path options are missing or unexpected arguments were supplied")
		return 2
	}
	fail := func(e error) int { fmt.Fprintln(errOut, "codex-config: "+e.Error()); return 1 }
	if command == "sync" {
		if home == "" {
			return fail(fmt.Errorf("cannot determine home directory; use --home"))
		}
		changed, names, e := syncConfig(source, *target, filepath.Clean(home))
		if e != nil {
			return fail(e)
		}
		state := "unchanged"
		if changed {
			state = "updated"
		}
		fmt.Fprintf(out, "  config.toml: %s -> %s\n", state, *target)
		if len(names) > 0 {
			fmt.Fprintln(out, "  placeholders: "+strings.Join(names, ", "))
		}
		fmt.Fprintln(out, "  secrets/runtime leak gate: passed")
		return 0
	}
	if command == "apply" {
		r, e := applyConfig(template, *target, binary)
		if e != nil {
			return fail(e)
		}
		if binary != "" {
			fmt.Fprintln(out, "  Codex strict config validation: passed")
		}
		if r.backup != "" {
			fmt.Fprintln(out, "  backup: "+filepath.Base(r.backup))
		}
		state := "unchanged"
		if r.changed {
			state = "updated"
		}
		fmt.Fprintln(out, "  config.toml: "+state)
		if len(r.missing) > 0 {
			fmt.Fprintln(out, "  missing secrets; skipped template groups: "+strings.Join(r.missing, ", "))
		}
		if len(r.preserved) > 0 {
			fmt.Fprintln(out, "  preserved existing secret-backed groups: "+strings.Join(r.preserved, ", "))
		}
		return 0
	}
	changed, groups, e := removeConfig(template, *target)
	if e != nil {
		return fail(e)
	}
	state := "unchanged"
	if changed {
		state = "managed settings removed"
	}
	fmt.Fprintln(out, "  config.toml: "+state)
	if len(groups) > 0 {
		fmt.Fprintln(out, "  managed table groups: "+strings.Join(groups, ", "))
	}
	return 0
}
