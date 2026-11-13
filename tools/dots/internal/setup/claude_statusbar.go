package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

var claudeStatusbarFiles = []string{"commands/statusbar.md", "commands/statusbar-doctor.md", "commands/statusbar-preview.md", "commands/statusbar-style.md", "commands/statusbar-reset.md", "skills/claude-statusbar/SKILL.md"}

func claudeStatusbarProfiles() ([]string, error) {
	standard, err := resolve(filepath.Join(home(), ".claude"))
	if err != nil {
		return nil, err
	}
	var profiles []string
	if p := os.Getenv("CLAUDE_CONFIG_DIR"); p != "" {
		profiles = append(profiles, absolute(p))
	}
	guard := filepath.Join(home(), ".local/share/claude-guard/home/.claude")
	info, err := os.Lstat(guard)
	guardMissing := os.IsNotExist(err)
	if err == nil {
		if !info.IsDir() {
			return nil, fmt.Errorf("guard profile is not a directory")
		}
		profiles = append(profiles, guard)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	seen := map[string]bool{standard: true}
	if guardMissing {
		resolvedGuard, err := resolve(guard)
		if err != nil {
			return nil, err
		}
		seen[resolvedGuard] = true
	}
	result := []string{standard}
	for _, profile := range profiles {
		profile, err = resolve(profile)
		if err != nil {
			return nil, err
		}
		if seen[profile] {
			continue
		}
		seen[profile] = true
		result = append(result, profile)
	}
	return result, nil
}

func claudeStatusbarPreflight() error {
	profiles, err := claudeStatusbarProfiles()
	if err != nil {
		return err
	}
	for _, profile := range profiles {
		settings := filepath.Join(profile, "settings.json")
		b, err := regularOptional(settings)
		if err != nil {
			return err
		}
		if b != nil {
			if _, err := object(b); err != nil {
				return fmt.Errorf("invalid JSON object in %s", settings)
			}
		}
		for _, name := range claudeStatusbarFiles {
			if _, err := regularOptional(filepath.Join(profile, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

func claudeStatusbarApply() error {
	profiles, err := claudeStatusbarProfiles()
	if err != nil {
		return err
	}
	data := make([][]byte, len(claudeStatusbarFiles))
	for i, name := range claudeStatusbarFiles {
		b, err := regularOptional(filepath.Join(profiles[0], name))
		if err != nil {
			return err
		}
		if b == nil {
			return fmt.Errorf("missing statusbar source %s", name)
		}
		data[i] = b
	}
	var changes []change
	for _, profile := range profiles[1:] {
		settings := filepath.Join(profile, "settings.json")
		prior, err := regularOptional(settings)
		if err != nil {
			return err
		}
		current := map[string]any{}
		if prior != nil {
			current, err = object(prior)
			if err != nil {
				return fmt.Errorf("invalid JSON object in %s", settings)
			}
		}
		command := "'" + strings.ReplaceAll(filepath.Join(home(), ".local/bin/cs"), "'", "'\\''") + "' render"
		status := map[string]any{"type": "command", "command": command, "refreshInterval": json.Number("1")}
		if !reflect.DeepEqual(current["statusLine"], status) {
			current["statusLine"] = status
			changes = append(changes, change{path: settings, data: encode(current), backup: privateBackup("claude-statusbar", settings), backupPrivate: true, expect: true, expected: prior})
		}
		for i, name := range claudeStatusbarFiles {
			p := filepath.Join(profile, name)
			prior, err := regularOptional(p)
			if err != nil {
				return err
			}
			changes = append(changes, change{path: p, data: data[i], backup: privateBackup("claude-statusbar", p), backupPrivate: true, expect: true, expected: prior})
		}
	}
	return commit(changes)
}
