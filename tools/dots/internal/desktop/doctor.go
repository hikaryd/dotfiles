package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"dots/native/internal/mux"
	"github.com/pelletier/go-toml/v2"
)

type doctorCheck struct {
	Status string `json:"status"`
	Name   string `json:"name"`
	Detail string `json:"detail"`
}
type managedLink struct{ destination, source string }

func managedLinks(root string) ([]managedLink, error) {
	files, e := filepath.Glob(filepath.Join(root, "steps/*.yml"))
	if e != nil {
		return nil, e
	}
	links := []managedLink{}
	destRE := regexp.MustCompile(`^    (~/.+):\s*$`)
	pathRE := regexp.MustCompile(`^      path: (.+?)\s*$`)
	for _, f := range files {
		b, e := os.ReadFile(f)
		if e != nil {
			return nil, e
		}
		inLinks := false
		destination := ""
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "- ") {
				inLinks = strings.TrimSpace(line) == "- link:"
				destination = ""
			}
			if !inLinks {
				continue
			}
			if m := destRE.FindStringSubmatch(line); m != nil {
				destination = m[1]
			}
			if m := pathRE.FindStringSubmatch(line); m != nil && destination != "" {
				links = append(links, managedLink{destination, filepath.Join(root, m[1])})
				destination = ""
			}
		}
	}
	return links, nil
}
func diagnose(root, userHome string) ([]doctorCheck, error) {
	checks := []doctorCheck{}
	add := func(status, name, detail string) { checks = append(checks, doctorCheck{status, name, detail}) }
	links, e := managedLinks(root)
	if e != nil {
		return nil, e
	}
	if len(links) == 0 {
		add("FAIL", "managed links", "No supported link entries found in steps/*.yml")
	}
	critical := []string{"~/.zshrc", "~/.zshenv", "~/.config/zsh/functions", "~/.config/tmux/tmux.conf", "~/.config/rmux/rmux.conf", "~/.config/ghostty/config", "~/.config/nvim"}
	for _, l := range links {
		target := filepath.Join(userHome, l.destination[2:])
		severity := "WARN"
		if has(l.destination, critical...) {
			severity = "FAIL"
		}
		if _, e = os.Stat(l.source); e != nil {
			add(severity, l.destination, "Managed source is missing")
			continue
		}
		info, e := os.Lstat(target)
		if e != nil || info.Mode()&os.ModeSymlink == 0 {
			detail := "Missing managed symlink"
			if e == nil {
				detail = "Exists but is not a managed symlink"
			}
			add(severity, l.destination, detail)
			continue
		}
		p, e := filepath.EvalSymlinks(target)
		source, se := filepath.EvalSymlinks(l.source)
		if e != nil || se != nil {
			add(severity, l.destination, "Broken or cyclic symlink")
		} else if p != source {
			add(severity, l.destination, "Symlink points outside its configured source")
		} else {
			add("OK", l.destination, "Managed link resolves correctly")
		}
	}
	for _, name := range []string{"zsh", "git", "dots-tools", "tmux", "sesh", "nvim", "dots", "zoxide", "eza", "bat", "rg", "fd", "yazi", "lazygit", "atuin"} {
		status, detail := "OK", "Available"
		if !available(name) {
			status, detail = "WARN", "Not on PATH"
			if has(name, "zsh", "git", "dots-tools", "tmux") {
				status = "FAIL"
			}
		}
		add(status, "command:"+name, detail)
	}
	if mux.IsRmux() || isFile(filepath.Join(root, "config/rmux/rmux.conf")) {
		status := "OK"
		if !available("rmux") {
			status = "FAIL"
		}
		add(status, "command:rmux", "Default multiplexer for new clients")
	}
	zshrc := filepath.Join(root, "config/zsh/.zshrc")
	if isFile(zshrc) {
		b, e := os.ReadFile(zshrc)
		if e != nil {
			return nil, e
		}
		content := string(b)
		if strings.Contains(content, "alias deploy-dev=") {
			m := regexp.MustCompile(`alias deploy-dev=['"]([^'"]+)['"]`).FindStringSubmatch(content)
			if m != nil {
				script := strings.Replace(m[1], "~/", userHome+"/", 1)
				if isFile(script) {
					add("OK", "alias:deploy-dev", "Script exists")
				} else {
					add("WARN", "alias:deploy-dev", "Alias references a missing script")
				}
			}
		}
		if strings.Contains(content, "alias nvim-bench=") {
			if available("hyperfine") {
				add("OK", "alias:nvim-bench", "hyperfine available")
			} else {
				add("WARN", "alias:nvim-bench", "Requires missing hyperfine")
			}
		}
		if available("zsh") {
			r, e := captureCommand(5*time.Second, maxBytes, "", envSet(os.Environ(), "ZDOTDIR=/dev/null"), "zsh", "-f", "-n", zshrc)
			if e != nil {
				add("WARN", "syntax:zshrc", "Native syntax check unavailable or timed out")
			} else if r.code != 0 {
				add("FAIL", "syntax:zshrc", "zsh syntax validation failed")
			} else {
				add("OK", "syntax:zshrc", "zsh -f -n passed")
			}
		}
	}
	for _, p := range []string{filepath.Join(root, "config/sesh/sesh.toml"), filepath.Join(userHome, ".config/atuin/config.toml")} {
		parent := filepath.Base(filepath.Dir(p))
		name := "syntax:" + parent + "/" + filepath.Base(p)
		if !isFile(p) {
			add("WARN", name, "Config missing")
			continue
		}
		b, e := os.ReadFile(p)
		var parsed map[string]any
		if e != nil || toml.Unmarshal(b, &parsed) != nil {
			add("FAIL", name, "Cannot parse config")
			continue
		}
		add("OK", name, "TOML parsed")
		if parent == "atuin" {
			sync, sok := parsed["auto_sync"].(bool)
			update, uok := parsed["update_check"].(bool)
			if sok && uok && !sync && !update {
				add("OK", "atuin:local-only", "Sync and update checks disabled")
			} else {
				add("WARN", "atuin:local-only", "Set auto_sync=false and update_check=false")
			}
		}
	}
	private := filepath.Join(userHome, ".config/dots")
	if !isDir(private) {
		add("WARN", "private:dots", "Machine-local config directory missing")
	} else {
		for _, name := range []string{"projects.json", "style", "ghostty-style.conf"} {
			if !isFile(filepath.Join(private, name)) {
				add("WARN", "local:"+name, "Machine-local setup not initialized")
			}
		}
		if isFile(filepath.Join(private, "style")) {
			b, e := os.ReadFile(filepath.Join(private, "style"))
			if e != nil {
				return nil, e
			}
			if has(strings.TrimSpace(string(b)), styleModes...) {
				add("OK", "style:mode", "Known style mode")
			} else {
				add("FAIL", "style:mode", "Expected glass, focus or presentation")
			}
		}
		entries, e := os.ReadDir(private)
		if e != nil {
			return nil, e
		}
		paths := []string{private}
		for _, entry := range entries {
			paths = append(paths, filepath.Join(private, entry.Name()))
		}
		for _, p := range paths {
			info, e := os.Stat(p)
			name := "private:" + filepath.Base(p)
			if e != nil {
				add("WARN", name, "Cannot inspect metadata")
				continue
			}
			mode := info.Mode().Perm()
			status, detail := "OK", fmt.Sprintf("Mode %03o", mode)
			if mode&0077 != 0 {
				status = "WARN"
				detail += "; expected owner-only access"
			}
			add(status, name, detail)
			if filepath.Ext(p) == ".json" && info.Mode().IsRegular() {
				b, e := os.ReadFile(p)
				if e != nil || !json.Valid(b) {
					add("FAIL", "syntax:"+filepath.Base(p), "Cannot parse JSON")
				} else {
					add("OK", "syntax:"+filepath.Base(p), "JSON parsed")
				}
			}
		}
	}
	ssh := filepath.Join(userHome, ".ssh")
	if isDir(ssh) {
		keys, e := filepath.Glob(filepath.Join(ssh, "id_*"))
		if e != nil {
			return nil, e
		}
		sort.Strings(keys)
		for _, p := range append([]string{ssh}, keys...) {
			if filepath.Ext(p) == ".pub" {
				continue
			}
			name := "permissions:.ssh/" + filepath.Base(p)
			info, e := os.Stat(p)
			if e != nil {
				add("WARN", name, "Cannot inspect metadata")
				continue
			}
			mode := info.Mode().Perm()
			status := "OK"
			if mode&0077 != 0 {
				status = "WARN"
			}
			add(status, name, fmt.Sprintf("Mode %03o", mode))
		}
	}
	return checks, nil
}
func doctorRun(root string, args []string) int {
	o, e := parse(args, "--json --strict", "")
	if e != nil || len(o.pos) > 0 {
		return usageError("doctor")
	}
	checks, e := diagnose(envOr("DOTS_ROOT", root), home())
	if e != nil {
		checks = []doctorCheck{{"FAIL", "doctor", "Cannot inspect configuration (OSError)"}}
	}
	summary := map[string]int{"OK": 0, "WARN": 0, "FAIL": 0}
	for _, c := range checks {
		summary[c.Status]++
	}
	if o.flag("--json") {
		if code := jsonExit(map[string]any{"checks": checks, "summary": summary}, true); code != 0 {
			return code
		}
	} else {
		for _, c := range checks {
			fmt.Printf("%-4s %s: %s\n", c.Status, c.Name, c.Detail)
		}
		fmt.Printf("\nOK: %d / WARN: %d / FAIL: %d\n", summary["OK"], summary["WARN"], summary["FAIL"])
	}
	if summary["FAIL"] > 0 || o.flag("--strict") && summary["WARN"] > 0 {
		return 1
	}
	return 0
}
