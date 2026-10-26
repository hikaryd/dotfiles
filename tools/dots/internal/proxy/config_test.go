package proxy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testController(t *testing.T) *controller {
	t.Helper()
	home := t.TempDir()
	c := &controller{home: home, dir: filepath.Join(home, ".config", "xray-pi"), port: 10810, out: &bytes.Buffer{}, errOut: &bytes.Buffer{}}
	if err := c.harden(); err != nil {
		t.Fatal(err)
	}
	return c
}
func TestVLESSAndConfig(t *testing.T) {
	s, err := parseVLESS("vless://fixture-id@[2001:db8::1]:443?type=ws&security=tls&sni=example.test&host=front.test&path=%2Fsocket&fp=chrome&alpn=h2%2Chttp%2F1.1&allowInsecure=yes&flow=fixture-flow#Name%20%E2%98%83")
	if err != nil {
		t.Fatal(err)
	}
	if s["name"] != "Name ☃" || s["uuid"] != "fixture-id" || s["address"] != "2001:db8::1" || s["path"] != "/socket" {
		t.Fatalf("parsed %#v", s)
	}
	want := object{"network": "ws", "security": "tls", "tlsSettings": object{"serverName": "example.test", "fingerprint": "chrome", "alpn": []string{"h2", "http/1.1"}, "allowInsecure": true}, "wsSettings": object{"path": "/socket", "headers": object{"Host": "front.test"}}}
	if got := streamSettings(s); !reflect.DeepEqual(got, want) {
		t.Fatalf("stream = %#v", got)
	}
	c := testController(t)
	cfg, err := c.config(s)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg["routing"], object{"rules": []any{}}) {
		t.Fatal("missing private routes must not create a catch-all direct rule")
	}
	inbound := cfg["inbounds"].([]any)[0].(object)
	if inbound["listen"] != "127.0.0.1" || inbound["protocol"] != "http" || inbound["port"] != 10810 {
		t.Fatalf("inbound %#v", inbound)
	}
	outbound := cfg["outbounds"].([]any)[0].(object)
	user := outbound["settings"].(object)["vnext"].([]any)[0].(object)["users"].([]any)[0].(object)
	if !reflect.DeepEqual(user, object{"id": "fixture-id", "encryption": "none", "flow": "fixture-flow"}) {
		t.Fatalf("user %#v", user)
	}
}
func TestPrivateRoutesThroughDirectNetwork(t *testing.T) {
	c := testController(t)
	if err := os.WriteFile(c.path("direct-domains.txt"), []byte("service.private.example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := c.config(object{"uuid": "fixture-id", "address": "example.test", "port": 443})
	if err != nil {
		t.Fatal(err)
	}
	want := object{"rules": []any{object{
		"type": "field", "domain": []string{"full:service.private.example"}, "outboundTag": "direct",
	}}}
	if !reflect.DeepEqual(cfg["routing"], want) {
		t.Fatalf("private hosts must bypass VLESS even when a client inherits stale NO_PROXY: routing = %#v", cfg["routing"])
	}
	outbounds := cfg["outbounds"].([]any)
	if outbounds[0].(object)["tag"] != "proxy" || outbounds[1].(object)["protocol"] != "freedom" || outbounds[1].(object)["tag"] != "direct" {
		t.Fatal("other traffic must retain the VLESS default and private hosts must use a direct outbound")
	}
}

func TestAllStreamVariants(t *testing.T) {
	tests := []struct {
		name string
		s    object
		want object
	}{
		{"default", object{}, object{"network": "tcp"}},
		{"empty", object{"type": "", "security": ""}, object{"network": "tcp"}},
		{"reality", object{"security": "reality", "sni": "s", "fp": "f", "pbk": "p", "sid": "i", "spx": "x"}, object{"network": "tcp", "security": "reality", "realitySettings": object{"serverName": "s", "fingerprint": "f", "publicKey": "p", "shortId": "i", "spiderX": "x"}}},
		{"grpc", object{"type": "grpc", "serviceName": "service", "mode": "multi"}, object{"network": "grpc", "grpcSettings": object{"serviceName": "service", "multiMode": true}}},
		{"grpc-single", object{"type": "grpc", "mode": "gun"}, object{"network": "grpc", "grpcSettings": object{"multiMode": false}}},
		{"tcp-http", object{"headerType": "http", "sni": "s"}, object{"network": "tcp", "tcpSettings": object{"header": object{"type": "http", "request": object{"path": []string{"/"}, "headers": object{"Host": []string{"s"}}}}}}},
		{"h2", object{"type": "h2", "path": "/p", "host": "h"}, object{"network": "h2", "httpSettings": object{"path": "/p", "host": []any{"h"}}}},
		{"upgrade", object{"type": "httpupgrade", "path": "/p", "host": "h"}, object{"network": "httpupgrade", "httpupgradeSettings": object{"path": "/p", "host": "h"}}},
		{"xhttp", object{"type": "xhttp", "path": "/p", "host": "h", "mode": "auto"}, object{"network": "xhttp", "xhttpSettings": object{"path": "/p", "host": "h", "mode": "auto"}}},
		{"split", object{"type": "splithttp", "path": "/p", "mode": "stream-up"}, object{"network": "splithttp", "xhttpSettings": object{"path": "/p", "mode": "stream-up"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := streamSettings(tt.s); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestPrivateDirectDomains(t *testing.T) {
	c := testController(t)
	if err := os.WriteFile(c.path("direct-domains.txt"), []byte("# Local only\nModels.Corp.Example\n\napi.corp.example\nmodels.corp.example\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := c.config(object{"uuid": "fixture-id", "address": "example.test", "port": 443})
	if err != nil {
		t.Fatal(err)
	}
	want := object{"rules": []any{object{
		"type": "field", "domain": []string{"full:models.corp.example", "full:api.corp.example"}, "outboundTag": "direct",
	}}}
	if !reflect.DeepEqual(cfg["routing"], want) {
		t.Fatalf("private exact hosts must route direct: %#v", cfg["routing"])
	}
	if cfg["outbounds"].([]any)[0].(object)["tag"] != "proxy" {
		t.Fatal("other traffic must retain the proxy default")
	}
}

func TestPrivateDirectDomainsRejectInvalidHosts(t *testing.T) {
	for _, host := range []string{"*.corp.example", "https://corp.example/v1", "corp.example:443", "bad host", "bad..host", "-bad.example", strings.Repeat("a", 64) + ".example"} {
		t.Run(host, func(t *testing.T) {
			c := testController(t)
			if err := os.WriteFile(c.path("direct-domains.txt"), []byte(host), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := c.config(object{"uuid": "fixture-id", "address": "example.test", "port": 443})
			if err == nil || strings.Contains(err.Error(), host) {
				t.Fatal("invalid private hosts must fail without disclosing their contents")
			}
		})
	}
}

func TestPrivateDirectDomainsReadFailure(t *testing.T) {
	c := testController(t)
	if err := os.Mkdir(c.path("direct-domains.txt"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := c.config(object{"uuid": "fixture-id", "address": "example.test", "port": 443}); err == nil {
		t.Fatal("unreadable private routing must not silently disappear")
	}
}
func TestDecodeSubscription(t *testing.T) {
	plain := "vless://fixture@host:443#one\nvless://fixture@host:80#two"
	enc := base64.StdEncoding.EncodeToString([]byte(plain))
	for _, raw := range []string{plain, enc, strings.TrimRight(enc, "="), " \n" + enc[:12] + "\n" + enc[12:] + " \t"} {
		if got := decodePayload([]byte(raw)); got != plain {
			t.Fatalf("decoded %q", got)
		}
	}
	if got := decodePayload([]byte("not base64!")); got != "not base64!" {
		t.Fatalf("fallback %q", got)
	}
}
func TestCLISetListSelectionAndPrivateModes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PI_PROXY_PORT", "10810")
	var out, errs bytes.Buffer
	invoke := func(args []string, input string) int {
		out.Reset()
		errs.Reset()
		return run("", append([]string{"pi"}, args...), strings.NewReader(input), &out, &errs)
	}
	if code := invoke([]string{"--set-vless"}, "vless://secret-fixture@example.test:443?security=reality#Fixture\n"); code != 0 {
		t.Fatalf("%d %s", code, &errs)
	}
	path := filepath.Join(home, ".config", "xray-pi", "servers.json")
	var data []object
	b, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(b, &data) != nil || len(data) != 1 {
		t.Fatalf("cache %s %v", b, err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	if code := invoke([]string{"--list"}, ""); code != 0 || !strings.Contains(out.String(), "Fixture") || strings.Contains(out.String(), "secret-fixture") {
		t.Fatalf("%d %s %s", code, &out, &errs)
	}
	for _, input := range []string{"abc\n", "0\n", "2\n", ""} {
		if code := invoke(nil, input); code != 1 {
			t.Fatalf("selection %q returned %d", input, code)
		}
	}
	if code := invoke([]string{"--set-vless"}, "vless://secret-fixture@example.test:invalid\n"); code != 1 || strings.Contains(out.String()+errs.String(), "secret-fixture") {
		t.Fatalf("bad secret output %d %s %s", code, &out, &errs)
	}
}
func TestAtomicWriteReplacesSymlinkAndIgnoresPredictableTemp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(target, []byte("unrelated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, ".config.tmp")); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "unrelated" {
		t.Fatal("modified symlink target")
	}
	st, _ := os.Lstat(path)
	if st.Mode()&os.ModeSymlink != 0 {
		t.Fatal("symlink not replaced")
	}
}
func TestVLESSPermissiveEscapesAndRepeatedParameters(t *testing.T) {
	s, err := parseVLESS("vless://fixture@host?type=&path=%2Fa;b%ZZ+space&host=a&host=b&host=#bad%ZZ+name")
	if err != nil {
		t.Fatal(err)
	}
	if s["name"] != "bad%ZZ+name" || s["path"] != "/a;b%ZZ space" || s["type"] != "tcp" || s["port"] != 443 || !reflect.DeepEqual(s["host"], []string{"a", "b"}) {
		t.Fatalf("parsed %#v", s)
	}
}
