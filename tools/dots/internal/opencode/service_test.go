package opencode

import (
	"bytes"
	"encoding/xml"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func serviceFixture(t *testing.T) *cli {
	c := testCLI(t)
	saveConfig(c.config, testConfig())
	os.MkdirAll(filepath.Dir(c.binary), 0700)
	os.WriteFile(c.binary, []byte("#!/bin/sh\n"), 0700)
	t.Setenv("LAUNCH_STATE", filepath.Join(c.home, "loaded"))
	t.Setenv("LAUNCH_ARGS", filepath.Join(c.home, "launch-args"))
	t.Setenv("LAUNCH_JOB", "gui/503/local.dots.opencode-proxy = {\n\tpath = "+c.servicePath()+"\n\tprogram = "+c.binary+"\n\targuments = {\n\t\t"+strings.Join([]string{c.binary, "proxy", "opencode", "serve", "--config", c.config}, "\n\t\t")+"\n\t}\n}\n")
	fakeCommand(t, "launchctl", `printf '%s\n' "$*" >> "$LAUNCH_ARGS"
case "$1" in
print) [ -f "$LAUNCH_STATE" ] || exit 113; printf '%s' "$LAUNCH_JOB" ;;
bootstrap) touch "$LAUNCH_STATE" ;;
bootout) rm -f "$LAUNCH_STATE" ;;
kickstart) exit 0 ;;
*) exit 9 ;;
esac
`)
	return c
}
func TestServicePlistIdempotentAndOwnLifecycle(t *testing.T) {
	c := serviceFixture(t)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg := testConfig()
	cfg.Port = l.Addr().(*net.TCPAddr).Port
	l.Close()
	b, err := c.servicePlist()
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if xml.Unmarshal(b, &doc) != nil {
		t.Fatal("invalid plist XML")
	}
	if !bytes.Contains(b, []byte("local.dots.opencode-proxy")) || !bytes.Contains(b, []byte("--config")) || bytes.Contains(b, []byte("test-secret")) || bytes.Contains(b, []byte("xray")) {
		t.Fatal("unsafe service definition")
	}
	if err := c.installService(cfg); err != nil {
		t.Fatal(err)
	}
	if err := c.installService(cfg); err != nil {
		t.Fatal(err)
	}
	calls, _ := os.ReadFile(os.Getenv("LAUNCH_ARGS"))
	if strings.Count(string(calls), "bootstrap ") != 1 || strings.Contains(string(calls), "kickstart") {
		t.Fatalf("setup restarted service: %s", calls)
	}
	if err := c.restartService(); err != nil {
		t.Fatal(err)
	}
	if err := c.stopService(); err != nil {
		t.Fatal(err)
	}
	if err := c.stopService(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.servicePath()); !os.IsNotExist(err) {
		t.Fatal("stop retained plist")
	}
	if _, err := loadConfig(c.config); err != nil {
		t.Fatal("stop deleted key")
	}
	calls, _ = os.ReadFile(os.Getenv("LAUNCH_ARGS"))
	if strings.Contains(string(calls), "pi-proxy") || strings.Contains(string(calls), "kill") {
		t.Fatal("touched another service")
	}
}
func TestServiceRefusesSameLabelForeignJob(t *testing.T) {
	for _, field := range []string{"path", "program", "arguments"} {
		t.Run(field, func(t *testing.T) {
			c := serviceFixture(t)
			data, err := c.servicePlist()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(c.servicePath()), 0700); err != nil {
				t.Fatal(err)
			}
			if err := atomicPrivate(c.servicePath(), data); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(os.Getenv("LAUNCH_STATE"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			job := os.Getenv("LAUNCH_JOB")
			switch field {
			case "path":
				job = strings.Replace(job, "path = "+c.servicePath(), "path = /tmp/foreign.plist", 1)
			case "program":
				job = strings.Replace(job, "program = "+c.binary, "program = /tmp/foreign", 1)
			case "arguments":
				job = strings.Replace(job, "\t\topencode\n", "\t\tforeign\n", 1)
			}
			t.Setenv("LAUNCH_JOB", job)
			if c.restartService() == nil || c.stopService() == nil {
				t.Fatal("modified a foreign loaded job sharing our label")
			}
			calls, _ := os.ReadFile(os.Getenv("LAUNCH_ARGS"))
			if strings.Contains(string(calls), "kickstart") || strings.Contains(string(calls), "bootout") {
				t.Fatalf("destructive call reached foreign job: %s", calls)
			}
		})
	}
}

func TestServicePreservesTrustedXDGBase(t *testing.T) {
	c := serviceFixture(t)
	target := filepath.Join(c.home, "config-target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	c.base = filepath.Join(c.home, "config-link")
	if err := os.Symlink(target, c.base); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", c.base)
	c.config = filepath.Join(c.base, "dots/opencode-proxy/config.json")
	if err := saveConfig(c.config, testConfig()); err != nil {
		t.Fatal(err)
	}
	data, err := c.servicePlist()
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	base := ""
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "key" {
			var key string
			if err := decoder.DecodeElement(&key, &start); err != nil {
				t.Fatal(err)
			}
			if key == "XDG_CONFIG_HOME" {
				token, err = decoder.Token()
				if err != nil {
					t.Fatal(err)
				}
				start = token.(xml.StartElement)
				if err := decoder.DecodeElement(&base, &start); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	// Reproduce launchd's environment, not the invoking shell's XDG setting.
	t.Setenv("XDG_CONFIG_HOME", base)
	if _, err := loadConfig(c.config); err != nil {
		t.Fatal("service cannot load the symlinked configuration accepted by setup")
	}
}

func TestServiceRefusesForeignPlistAndListener(t *testing.T) {
	c := serviceFixture(t)
	os.MkdirAll(filepath.Dir(c.servicePath()), 0700)
	os.WriteFile(c.servicePath(), []byte("foreign"), 0600)
	if c.installService(testConfig()) == nil || c.stopService() == nil {
		t.Fatal("replaced foreign service")
	}
	b, _ := os.ReadFile(c.servicePath())
	if string(b) != "foreign" {
		t.Fatal("changed foreign plist")
	}
	os.Remove(c.servicePath())
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	cfg := testConfig()
	cfg.Port = l.Addr().(*net.TCPAddr).Port
	if c.installService(cfg) == nil {
		t.Fatal("installed over foreign listener")
	}
	calls, _ := os.ReadFile(os.Getenv("LAUNCH_ARGS"))
	if strings.Contains(string(calls), "bootstrap") || strings.Contains(string(calls), "bootout") {
		t.Fatal("changed foreign service/port")
	}
}
