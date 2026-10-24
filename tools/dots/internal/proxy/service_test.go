package proxy

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func fakeLaunchctl(fixture string) {
	f, err := os.OpenFile(filepath.Join(fixture, "launch-calls"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(1)
	}
	json.NewEncoder(f).Encode(os.Args[1:])
	f.Close()
	fail, _ := os.ReadFile(filepath.Join(fixture, "launch-error"))
	if len(os.Args) < 2 {
		os.Exit(2)
	}
	if string(fail) == os.Args[1] {
		os.Stderr.WriteString("fixture-secret-do-not-log")
		os.Exit(5)
	}
	loaded := filepath.Join(fixture, "loaded")
	switch os.Args[1] {
	case "print":
		data, err := os.ReadFile(loaded)
		if err != nil {
			os.Exit(113)
		}
		os.Stdout.Write(data)
	case "bootstrap":
		data, _ := os.ReadFile(os.Args[3])
		var plist struct {
			Args []string `xml:"dict>array>string"`
		}
		xml.Unmarshal(data, &plist)
		if len(plist.Args) == 0 {
			os.Exit(2)
		}
		job := "job = {\npath = " + os.Args[3] + "\nprogram = " + plist.Args[0] + "\narguments = {\n" + strings.Join(plist.Args, "\n") + "\n}\n}\n"
		os.WriteFile(loaded, []byte(job), 0600)
	case "bootout":
		os.Remove(loaded)
	default:
		os.Exit(2)
	}
}
func serviceCalls(t *testing.T, fixture string) [][]string {
	t.Helper()
	data, _ := os.ReadFile(filepath.Join(fixture, "launch-calls"))
	var calls [][]string
	d := json.NewDecoder(bytes.NewReader(data))
	for {
		var args []string
		err := d.Decode(&args)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		calls = append(calls, args)
	}
	return calls
}

// Decode the emitted plist rather than testing source spelling or XML escapes.
func plistElement(t *testing.T, d *xml.Decoder, start xml.StartElement) any {
	t.Helper()
	switch start.Name.Local {
	case "true", "false":
		var s string
		d.DecodeElement(&s, &start)
		return start.Name.Local == "true"
	case "string", "integer", "key":
		var s string
		if err := d.DecodeElement(&s, &start); err != nil {
			t.Fatal(err)
		}
		return s
	case "array":
		var a []any
		for {
			tok, err := d.Token()
			if err != nil {
				t.Fatal(err)
			}
			switch v := tok.(type) {
			case xml.StartElement:
				a = append(a, plistElement(t, d, v))
			case xml.EndElement:
				return a
			}
		}
	case "dict":
		m := map[string]any{}
		key := ""
		for {
			tok, err := d.Token()
			if err != nil {
				t.Fatal(err)
			}
			switch v := tok.(type) {
			case xml.StartElement:
				if v.Name.Local == "key" {
					key = plistElement(t, d, v).(string)
				} else {
					m[key] = plistElement(t, d, v)
				}
			case xml.EndElement:
				return m
			}
		}
	}
	t.Fatalf("unsupported plist element %s", start.Name.Local)
	return nil
}
func decodePlist(t *testing.T, data []byte) map[string]any {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := d.Token()
		if err != nil {
			t.Fatal(err)
		}
		if s, ok := tok.(xml.StartElement); ok && s.Name.Local == "dict" {
			return plistElement(t, d, s).(map[string]any)
		}
	}
}
func TestServicePlist(t *testing.T) {
	c := testController(t)
	c.home = filepath.Join(c.home, "space & home")
	c.dir = filepath.Join(c.home, ".config/xray-pi")
	c.port = 12345
	exe := filepath.Join(c.home, ".local/share/dots/bin/dots-tools")
	data, err := c.servicePlist(exe)
	if err != nil {
		t.Fatal(err)
	}
	p := decodePlist(t, data)
	if p["Label"] != "local.dots.pi-proxy" || !reflect.DeepEqual(p["ProgramArguments"], []any{exe, "proxy", "pi", "--daemon"}) {
		t.Fatal("wrong service identity/arguments", p)
	}
	for _, key := range []string{"RunAtLoad", "KeepAlive", "AbandonProcessGroup"} {
		if p[key] != true {
			t.Fatal("missing lifecycle setting", key)
		}
	}
	if p["ThrottleInterval"] != "30" {
		t.Fatal("restart not throttled")
	}
	env := p["EnvironmentVariables"].(map[string]any)
	if len(env) != 3 || env["HOME"] != c.home || env["PI_PROXY_PORT"] != "12345" || !strings.Contains(env["PATH"].(string), "/opt/homebrew/bin") || !strings.Contains(env["PATH"].(string), "/usr/bin") {
		t.Fatal("unsafe or incomplete environment", env)
	}
	if _, err := c.servicePlist("relative/dots-tools"); err == nil {
		t.Fatal("relative executable accepted")
	}
	if _, err := c.servicePlist("/tmp/go-build123/b001/exe/dots-tools"); err == nil {
		t.Fatal("go run executable accepted")
	}
	if servicePlatform("linux") == nil || servicePlatform("darwin") != nil {
		t.Fatal("platform gate incorrect")
	}
}
func TestServiceInstallIsIdempotent(t *testing.T) {
	c, fixture, pid, cfg := startMonitored(t)
	echo := streamFixture(t, c)
	exe := filepath.Join(c.home, ".local/share/dots/bin/dots-tools")
	for i := 0; i < 2; i++ {
		if err := c.installServiceAt(exe); err != nil {
			t.Fatal(err)
		}
		sameRoute(t, c, pid, cfg)
		echo(strconv.Itoa(i))
	}
	domain := "gui/" + strconv.Itoa(os.Getuid())
	want := [][]string{{"print", domain + "/local.dots.pi-proxy"}, {"bootstrap", domain, c.servicePath()}, {"print", domain + "/local.dots.pi-proxy"}}
	if calls := serviceCalls(t, fixture); !reflect.DeepEqual(calls, want) {
		t.Fatalf("duplicate install restarted job: %#v", calls)
	}
	if err := c.installServiceAt(filepath.Join(c.home, "different", "dots-tools")); err == nil {
		t.Fatal("changed loaded definition silently accepted")
	}
	log, err := os.Stat(c.path("supervisor.log"))
	if err != nil || log.Mode().Perm() != 0600 {
		t.Fatal("supervisor log not private")
	}
}
func TestServiceUninstallLeavesXrayRunning(t *testing.T) {
	c, fixture, pid, cfg := startMonitored(t)
	echo := streamFixture(t, c)
	if err := c.installServiceAt(filepath.Join(c.home, ".local/share/dots/bin/dots-tools")); err != nil {
		t.Fatal(err)
	}
	if err := c.uninstallService(); err != nil {
		t.Fatal(err)
	}
	sameRoute(t, c, pid, cfg)
	echo("after uninstall")
	if _, err := os.Stat(c.servicePath()); !os.IsNotExist(err) {
		t.Fatal("plist retained")
	}
	calls := serviceCalls(t, fixture)
	last := calls[len(calls)-1]
	if len(last) != 2 || last[0] != "bootout" || !strings.HasSuffix(last[1], "/local.dots.pi-proxy") {
		t.Fatal("unloaded other service", calls)
	}
}
func TestServiceInstallFailureIsSafe(t *testing.T) {
	for _, operation := range []string{"print", "bootstrap"} {
		t.Run(operation, func(t *testing.T) {
			c, fixture, pid, cfg := startMonitored(t)
			os.WriteFile(filepath.Join(fixture, "launch-error"), []byte(operation), 0600)
			err := c.installServiceAt(filepath.Join(c.home, ".local/share/dots/bin/dots-tools"))
			if err == nil || strings.Contains(err.Error(), "fixture-secret") {
				t.Fatal("failed install succeeded or leaked diagnostics", err)
			}
			sameRoute(t, c, pid, cfg)
		})
	}
}
func TestServiceStopDisablesSupervisorFirst(t *testing.T) {
	c, fixture, pid, cfg := startMonitored(t)
	if err := c.installServiceAt(filepath.Join(c.home, ".local/share/dots/bin/dots-tools")); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(fixture, "launch-error"), []byte("bootout"), 0600)
	if _, err := c.dispatch([]string{"--stop"}); err == nil {
		t.Fatal("stop ignored service shutdown failure")
	}
	sameRoute(t, c, pid, cfg)
	os.Remove(filepath.Join(fixture, "launch-error"))
	if _, err := c.dispatch([]string{"--stop"}); err != nil {
		t.Fatal(err)
	}
	if pidRunning(pid) {
		t.Fatal("explicit stop failed to stop xray")
	}
}
