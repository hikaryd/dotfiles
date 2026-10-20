package proxy

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEnsureNeverSelectsOrSpawns(t *testing.T) {
	c, fixture, pid, cfg := startMonitored(t)
	os.Remove(c.path("selection.json"))
	code, err := c.dispatch([]string{"--ensure"})
	if code != 0 || err != errNotReady {
		t.Fatalf("ensure did more than local readiness: %d %v", code, err)
	}
	sameRoute(t, c, pid, cfg)
	if data, _ := os.ReadFile(filepath.Join(fixture, "requests")); len(data) != 0 {
		t.Fatal("ensure made external requests", string(data))
	}
}
func TestEnsureReadyAndRunAreLocal(t *testing.T) {
	c, fixture, pid, cfg := startMonitored(t)
	if err := c.maintain(&monitorState{}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(fixture, "requests"))
	if _, err := c.dispatch([]string{"--ensure"}); err != nil {
		t.Fatal(err)
	}
	c.rawIn = strings.NewReader("prompt stdin\n")
	c.out = &bytes.Buffer{}
	args := []string{"--daemon", "--install-service", "-p", "two words"}
	code, err := c.dispatch(append([]string{"--run", "--"}, args...))
	if err != nil || code != 7 {
		t.Fatalf("run result %d %v", code, err)
	}
	var result struct {
		Args  []string
		Stdin string
	}
	_, output, _ := strings.Cut(c.out.(*bytes.Buffer).String(), "\n")
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Args, args) || result.Stdin != "prompt stdin\n" {
		t.Fatalf("lost Pi arguments/stdin: %+v", result)
	}
	after, _ := os.ReadFile(filepath.Join(fixture, "requests"))
	if !bytes.Equal(before, after) {
		t.Fatal("consumer launch remeasured route")
	}
	sameRoute(t, c, pid, cfg)
}
func TestEnsureDoesNotCreateState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var out, diagnostic bytes.Buffer
	if code := run("", []string{"pi", "--ensure"}, strings.NewReader(""), &out, &diagnostic); code != 1 {
		t.Fatal("unready proxy accepted", code)
	}
	if _, err := os.Stat(filepath.Join(home, ".config/xray-pi")); !os.IsNotExist(err) {
		t.Fatal("ensure created state")
	}
}
