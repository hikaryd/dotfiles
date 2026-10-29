package rmuxstate

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func defaultState(home string) string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "rmux")
}
func Run(args []string, out, diagnostic io.Writer) int {
	usage := func() {
		fmt.Fprintln(out, "Usage: dots-tools rmux-state save|restore|status|install [--state-dir PATH] [--socket PATH] [--rmux-bin PATH] [--config PATH] [--helper PATH]")
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		usage()
		return 0
	}
	command := args[0]
	if command != "save" && command != "restore" && command != "status" && command != "install" {
		fmt.Fprintln(diagnostic, "rmux-state: unknown command")
		return 2
	}
	home, e := os.UserHomeDir()
	if e != nil {
		fmt.Fprintln(diagnostic, e)
		return 1
	}
	fs := flag.NewFlagSet("rmux-state", flag.ContinueOnError)
	fs.SetOutput(diagnostic)
	dir := fs.String("state-dir", defaultState(home), "private state directory")
	socket := fs.String("socket", "", "isolated rmux socket")
	bin := fs.String("rmux-bin", os.Getenv("DOTS_RMUX_BIN"), "rmux executable")
	cfgBase := os.Getenv("XDG_CONFIG_HOME")
	if cfgBase == "" {
		cfgBase = filepath.Join(home, ".config")
	}
	config := fs.String("config", filepath.Join(cfgBase, "rmux", "rmux.conf"), "rmux config")
	helper := fs.String("helper", "", "installed dots-tools executable used by launchd and launcher")
	if e = fs.Parse(args[1:]); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		return 2
	}
	if !filepath.IsAbs(*dir) || (*socket != "" && !filepath.IsAbs(*socket)) {
		fmt.Fprintln(diagnostic, "rmux-state: state and socket paths must be absolute")
		return 2
	}
	if command == "status" {
		if _, e = os.Lstat(*dir); errors.Is(e, os.ErrNotExist) {
			fmt.Fprintln(out, `{"enabled":false,"snapshot":false,"ready":false}`)
			return 0
		}
	}
	s, e := Open(*dir)
	if e != nil {
		fmt.Fprintln(diagnostic, "rmux-state:", e)
		return 1
	}
	defer s.Close()
	if *bin == "" {
		for _, candidate := range []string{filepath.Join(home, ".local", "bin", "rmux"), "/opt/homebrew/bin/rmux", "/usr/local/bin/rmux"} {
			if i, e := os.Stat(candidate); e == nil && i.Mode().Perm()&0111 != 0 {
				*bin = candidate
				break
			}
		}
	}
	if *bin != "" && !filepath.IsAbs(*bin) {
		if resolved, e := exec.LookPath(*bin); e == nil {
			*bin = resolved
		}
	}
	r := Runtime{Home: home, Bin: *bin, Socket: *socket, Config: *config, Store: s, Warnings: diagnostic}
	switch command {
	case "status":
		enabled, ee := s.read("enabled")
		if ee != nil && !errors.Is(ee, unix.ENOENT) {
			e = ee
			break
		}
		_, se := s.Load()
		if se != nil && !errors.Is(se, unix.ENOENT) {
			e = se
			break
		}
		status := struct {
			Enabled  bool   `json:"enabled"`
			Snapshot bool   `json:"snapshot"`
			Ready    bool   `json:"ready"`
			StateDir string `json:"state_dir"`
		}{ee == nil && strings.HasPrefix(string(enabled), "v1\n"), se == nil, false, *dir}
		if se == nil && r.Bin != "" {
			status.Ready = r.ready()
		}
		json.NewEncoder(out).Encode(status)
	case "save":
		var changed bool
		changed, e = r.Save()
		if e == nil {
			fmt.Fprintf(out, "saved changed=%t\n", changed)
		}
	case "restore":
		e = r.Restore()
		if e == nil {
			fmt.Fprintln(out, "restore reconciled")
		}
	case "install":
		if *helper == "" {
			*helper, e = os.Executable()
		}
		if e == nil {
			e = r.Install(*helper)
		}
		if e == nil {
			fmt.Fprintln(out, "Prepared private LaunchAgents and enabled first-terminal restore. No jobs loaded.")
		}
	}
	if e != nil {
		fmt.Fprintln(diagnostic, "rmux-state:", e)
		return 1
	}
	return 0
}
func xmlText(s string) string { var b bytes.Buffer; xml.EscapeText(&b, []byte(s)); return b.String() }
func (r *Runtime) Install(helper string) error {
	if !filepath.IsAbs(helper) || !cleanText(helper, 4096) || !trustedExecutable(helper) || !filepath.IsAbs(r.Bin) || !trustedExecutable(r.Bin) || !filepath.IsAbs(r.Config) || !cleanText(r.Config, 4096) {
		return errors.New("install requires safe absolute helper, rmux and config paths")
	}
	if i, e := os.Stat(r.Config); e != nil || !i.Mode().IsRegular() {
		return errors.New("rmux config missing")
	}
	if _, e := r.Store.Load(); e != nil && !errors.Is(e, unix.ENOENT) {
		return e
	}
	d, e := secureDir(filepath.Join(r.Home, "Library", "LaunchAgents"), false)
	if e != nil {
		return e
	}
	defer d.Close()
	jobs := Store{dir: d}
	for _, command := range []string{"restore", "save"} {
		label := "local.dots.rmux-state-" + command
		args := []string{helper, "rmux-state", command, "--state-dir", r.Store.Dir, "--rmux-bin", r.Bin, "--config", r.Config}
		if r.Socket != "" {
			args = append(args, "--socket", r.Socket)
		}
		var b strings.Builder
		b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>` + label + `</string><key>ProgramArguments</key><array>`)
		for _, a := range args {
			b.WriteString("<string>" + xmlText(a) + "</string>")
		}
		b.WriteString("</array><key>EnvironmentVariables</key><dict><key>HOME</key><string>" + xmlText(r.Home) + "</string><key>PATH</key><string>" + xmlText(filepath.Join(r.Home, ".local", "bin")+":/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin") + "</string></dict><key>ProcessType</key><string>Background</string>")
		if command == "restore" {
			b.WriteString("<key>RunAtLoad</key><true/>")
		} else {
			b.WriteString("<key>StartInterval</key><integer>60</integer>")
		}
		b.WriteString("</dict></plist>\n")
		if e = jobs.atomic(label+".plist", []byte(b.String())); e != nil {
			return e
		}
	}
	return r.Store.atomic("enabled", []byte("v1\n"+helper+"\n"))
}
