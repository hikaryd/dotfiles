package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func saveConfig(t *testing.T, profiles ...profile) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metrics.json")
	data, err := json.Marshal(inventory{Profiles: profiles})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fixtureProfile() profile {
	return profile{Name: "test", Namespace: "example", URL: "http://127.0.0.1:19090"}
}

func TestPrivateConfigCreationAndValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	path := configPath()
	if err := initConfig(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private file: %v %v", info, err)
	}
	dir, _ := os.Stat(filepath.Dir(path))
	if dir.Mode().Perm() != 0700 {
		t.Fatal(dir.Mode())
	}
	before, _ := os.ReadFile(path)
	if err := initConfig(path); err == nil {
		t.Fatal("overwrote existing config")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("config modified")
	}
	if _, err := loadConfig(path); err == nil {
		t.Fatal("empty coordinates accepted")
	}
	p := fixtureProfile()
	cfg, err := loadConfig(saveConfig(t, p))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles[0].metrics()) < 25 || cfg.Profiles[0].StatusLabel != "status" {
		t.Fatal("catalog/defaults missing")
	}
}

func TestDefaultPathEscapesGitManagedConfigDirectory(t *testing.T) {
	root, data := t.TempDir(), t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0700)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("XDG_DATA_HOME", data)
	path := configPath()
	if path != filepath.Join(data, "dots-private", "metrics.json") {
		t.Fatalf("unsafe default: %s", path)
	}
	if err := initConfig(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "dots-kube")); !os.IsNotExist(err) {
		t.Fatal("wrote to Git config root")
	}
	os.Mkdir(filepath.Join(data, ".git"), 0700)
	if err := initConfig(configPath()); err == nil {
		t.Fatal("fallback Git guard bypassed")
	}
}

func TestPrivateFilesCannotEnterWorkingTrees(t *testing.T) {
	for _, worktree := range []bool{false, true} {
		root := t.TempDir()
		if worktree {
			os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: elsewhere"), 0600)
		} else {
			os.Mkdir(filepath.Join(root, ".git"), 0700)
		}
		path := filepath.Join(root, "nested", "metrics.json")
		if err := initConfig(path); err == nil {
			t.Fatal("created config under Git")
		}
		link := filepath.Join(t.TempDir(), "linked")
		if err := os.Symlink(root, link); err != nil {
			t.Fatal(err)
		}
		if err := initConfig(filepath.Join(link, "new", "metrics.json")); err == nil {
			t.Fatal("symlink parent bypassed Git guard")
		}
		secret := filepath.Join(root, "credentials")
		os.WriteFile(secret, []byte("PRIVATE"), 0600)
		fileLink := filepath.Join(t.TempDir(), "credentials")
		os.Symlink(secret, fileLink)
		if err := outsideGit(fileLink); err == nil {
			t.Fatal("file symlink bypassed Git guard")
		}
	}
}

func TestPrivateConfigPermissionsSymlinksAndRedactedErrors(t *testing.T) {
	path := saveConfig(t, fixtureProfile())
	os.Chmod(path, 0644)
	if _, err := loadConfig(path); err == nil {
		t.Fatal("world-readable inventory accepted")
	}
	os.Chmod(path, 0400)
	if _, err := loadConfig(path); err != nil {
		t.Fatalf("read-only private file rejected: %v", err)
	}
	os.Chmod(path, 0700)
	if _, err := loadConfig(path); err == nil {
		t.Fatal("unexpected executable config accepted")
	}
	os.Chmod(path, 0600)
	link := filepath.Join(t.TempDir(), "link")
	os.Symlink(path, link)
	if _, err := loadConfig(link); err == nil {
		t.Fatal("config symlink accepted")
	}
	if _, err := readPrivate(path, 1); err == nil {
		t.Fatal("size bound ignored")
	}
	os.WriteFile(path, []byte(`{"profiles":[],"PRIVATE_CREDENTIAL":"SECRET"}`), 0600)
	_, err := loadConfig(path)
	if err == nil || strings.Contains(err.Error(), "PRIVATE") || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), path) {
		t.Fatalf("diagnostic leaked input: %v", err)
	}
}

func TestRejectInvalidConnectionAndQueries(t *testing.T) {
	cases := []func(*profile){
		func(p *profile) { p.URL = "https://name:SECRET@example.test" },
		func(p *profile) { p.URL = "https://example.test?token=SECRET" },
		func(p *profile) { p.URL = "http://example.test" },
		func(p *profile) { p.Namespace = "" },
		func(p *profile) { p.StatusLabel = "status\"}" },
		func(p *profile) { p.RequestMetric = "sum(up)" },
		func(p *profile) { p.StatusLabel = "namespace" },
		func(p *profile) { p.CustomOnly = true },
		func(p *profile) { p.Queries = []metric{{ID: "rps", Title: "duplicate", Query: "up"}} },
		func(p *profile) { p.Forward = &forward{} },
	}
	for i, change := range cases {
		p := fixtureProfile()
		change(&p)
		if _, err := loadConfig(saveConfig(t, p)); err == nil {
			t.Fatalf("invalid case %d accepted", i)
		}
	}
	p := fixtureProfile()
	p.URL = "https://metrics.example.test/select/tenant/prometheus"
	p.CustomOnly = true
	p.Namespace = ""
	p.Queries = []metric{{ID: "queue", Title: "Queue depth", Unit: "jobs", Query: "sum(queue_depth)"}}
	cfg, err := loadConfig(saveConfig(t, p))
	if err != nil || len(cfg.Profiles[0].metrics()) != 1 {
		t.Fatalf("custom-only: %v", err)
	}
	if _, err := loadConfig(saveConfig(t, p, p)); err == nil {
		t.Fatal("duplicate profile names accepted")
	}
}

func TestHTTPStatusFiltersAcceptCodesAndInstrumentatorClasses(t *testing.T) {
	p := fixtureProfile()
	p.validate()
	items := p.metrics()
	matcher := regexp.MustCompile(`status=~"([^"]+)"`)
	for _, id := range []string{"statuses", "errors5xx", "errors5xxrate", "errors4xx", "errors4xxrate", "errorservices"} {
		i, _ := findMetric(items, id)
		pattern := matcher.FindStringSubmatch(items[i].Query)
		if len(pattern) != 2 {
			t.Fatalf("no status matcher: %s", id)
		}
		re := regexp.MustCompile("^(" + pattern[1] + ")$")
		class := "5"
		if strings.Contains(id, "4xx") {
			class = "4"
		}
		for _, value := range []string{class + "00", class + "xx"} {
			if !re.MatchString(value) {
				t.Fatalf("%s rejects %s", id, value)
			}
		}
		if re.MatchString("5000") || re.MatchString("unknown") {
			t.Fatalf("overbroad matcher: %s", id)
		}
	}
}

func TestCatalogScopesUnitsAndFormulas(t *testing.T) {
	p := fixtureProfile()
	p.Service = `service"with\quotes`
	if err := p.validate(); err != nil {
		t.Fatal(err)
	}
	items := p.metrics()
	query := func(id string) string {
		i, err := findMetric(items, id)
		if err != nil {
			t.Fatal(err)
		}
		return items[i].Query
	}
	for _, id := range []string{"rps", "rpm", "p95", "errors5xx", "services"} {
		if !strings.Contains(query(id), `service="service\"with\\quotes"`) {
			t.Fatalf("unescaped scope: %s", query(id))
		}
	}
	for _, id := range []string{"cpu", "memory", "restarts", "unavailable", "notready", "scrapes"} {
		if strings.Contains(query(id), "service=") {
			t.Fatalf("HTTP filter leaked to namespace metric: %s", id)
		}
	}
	for id, want := range map[string]string{"rps": "rate(", "rpm": "increase(", "statuses": `"${1}xx"`, "p95": "1000 * histogram_quantile(0.95", "cpulimits": "and on(namespace,pod,container)", "notready": `phase="Running"`} {
		if !strings.Contains(query(id), want) {
			t.Fatalf("%s formula %s", id, query(id))
		}
	}
	for _, m := range items {
		if strings.Contains(m.Query, "or vector(0)") {
			t.Fatal("unknown data fabricated as zero")
		}
	}
}
