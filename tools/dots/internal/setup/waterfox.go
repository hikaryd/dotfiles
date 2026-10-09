package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func waterfoxProfiles(base string) ([]string, error) {
	b, e := readOptional(filepath.Join(base, "profiles.ini"))
	if e != nil {
		return nil, e
	}
	sections := map[string]map[string]string{}
	var order []string
	var section, last string
	lastIndent := 0
	defaults := map[string]string{}
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			end := strings.Index(line, "]")
			if end < 1 {
				return nil, errors.New("invalid profiles.ini section")
			}
			section = line[1:end]
			last = ""
			if section == "DEFAULT" {
				continue
			}
			if _, ok := sections[section]; ok {
				return nil, errors.New("duplicate profiles.ini section")
			}
			sections[section] = map[string]string{}
			order = append(order, section)
			continue
		}
		if section == "" {
			return nil, errors.New("missing profiles.ini section")
		}
		values := sections[section]
		if section == "DEFAULT" {
			values = defaults
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " \t"))
		if indent > lastIndent && last != "" {
			values[last] += "\n" + line
			continue
		}
		idx := strings.IndexAny(line, "=:")
		if idx < 0 {
			return nil, errors.New("invalid profiles.ini value")
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		if _, ok := values[key]; ok {
			return nil, errors.New("duplicate profiles.ini value")
		}
		values[key] = strings.TrimSpace(line[idx+1:])
		last = key
		lastIndent = indent
	}
	var result []string
	seen := map[string]bool{}
	pattern := regexp.MustCompile(`^Profile\d+$`)
	for _, section := range order {
		if !pattern.MatchString(section) {
			continue
		}
		m := map[string]string{}
		for k, v := range defaults {
			m[k] = v
		}
		for k, v := range sections[section] {
			m[k] = v
		}
		p := m["path"]
		if p == "" {
			continue
		}
		relative := true
		if v, ok := m["isrelative"]; ok {
			switch strings.ToLower(v) {
			case "1", "yes", "true", "on":
				relative = true
			case "0", "no", "false", "off":
				relative = false
			default:
				return nil, errors.New("invalid IsRelative boolean")
			}
		}
		if relative {
			if filepath.IsAbs(p) {
				return nil, fmt.Errorf("Относительный профиль содержит абсолютный путь: %s", section)
			}
			p = filepath.Join(base, p)
		} else if !filepath.IsAbs(p) {
			return nil, fmt.Errorf("Абсолютный профиль содержит относительный путь: %s", section)
		}
		p, e = resolve(p)
		if e != nil {
			return nil, e
		}
		i, e := os.Stat(filepath.Join(p, "prefs.js"))
		if e != nil {
			if os.IsNotExist(e) {
				continue
			}
			return nil, e
		}
		if !seen[p] && i.Mode().IsRegular() {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result, nil
}
func waterfoxMerge(existing []byte, settings string) ([]byte, error) {
	begin, end := "// BEGIN dots Waterfox", "// END dots Waterfox"
	s := string(existing)
	block := begin + "\n" + strings.TrimRight(settings, " \t\r\n") + "\n" + end + "\n"
	if !strings.Contains(s, begin) && !strings.Contains(s, end) {
		if s != "" && !strings.HasSuffix(s, "\n") {
			s += "\n"
		}
		return []byte(s + block), nil
	}
	re := regexp.MustCompile(`(?ms)^` + regexp.QuoteMeta(begin) + `\r?\n.*?^` + regexp.QuoteMeta(end) + `(?:\r?\n|$)`)
	match := re.FindStringIndex(s)
	if strings.Count(s, begin) != 1 || strings.Count(s, end) != 1 || match == nil {
		return nil, errors.New("Повреждён или дублирован блок dots; файл не изменён")
	}
	return []byte(s[:match[0]] + block + s[match[1]:]), nil
}
func waterfoxApply(root string) error {
	base := filepath.Join(home(), "Library/Application Support/Waterfox")
	settings, e := os.ReadFile(filepath.Join(root, "config/waterfox/user.js"))
	if e != nil {
		return e
	}
	profiles, e := waterfoxProfiles(base)
	if e != nil {
		return e
	}
	if len(profiles) == 0 {
		fmt.Println("Использованных профилей Waterfox нет; пропуск.")
		return nil
	}
	var changes []change
	var changed []bool
	for _, profile := range profiles {
		p := filepath.Join(profile, "user.js")
		if i, e := os.Lstat(p); e == nil && i.Mode()&os.ModeSymlink != 0 {
			return errors.New("Отказ от изменения симлинка")
		}
		b, e := readOptional(p)
		if e != nil {
			return e
		}
		updated, e := waterfoxMerge(b, string(settings))
		if e != nil {
			return e
		}
		changed = append(changed, string(b) != string(updated))
		changes = append(changes, change{path: p, data: updated, backup: filepath.Join(profile, "user.js.dots-backup-"+stamp()), backupPrivate: true})
	}
	if e = commit(changes); e != nil {
		return e
	}
	for i, p := range profiles {
		status := "без изменений"
		if changed[i] {
			status = "обновлён"
		}
		fmt.Printf("Waterfox: %s: %s\n", filepath.Base(p), status)
	}
	return nil
}
