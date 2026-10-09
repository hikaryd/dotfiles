package vpn

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const vlessURI = "vless://00000000-0000-4000-8000-000000000001@vless.example:443?type=tcp&security=reality&sni=cover.example&pbk=fake-public-key&sid=abcd&flow=xtls-rprx-vision#Germany%20-%20description"
const hyURI = "hysteria2://fake-password@hy2.example:8446/?sni=cover.example&insecure=0#Finland"

func mustNode(t *testing.T, s string, i int) object {
	t.Helper()
	n, e := parseNode(s, i)
	if e != nil {
		t.Fatal(e)
	}
	return n
}
func equal(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
func TestVlessReality(t *testing.T) {
	n := mustNode(t, vlessURI, 6)
	equal(t, n["tag"], "Germany #6")
	equal(t, n["server_port"], 443)
	equal(t, n["flow"], "xtls-rprx-vision")
	equal(t, obj(n["tls"])["reality"], object{"enabled": true, "public_key": "fake-public-key", "short_id": "abcd"})
	if n["transport"] != nil {
		t.Fatal(n)
	}
}
func TestVlessGRPC(t *testing.T) {
	n := mustNode(t, strings.Replace(vlessURI, "type=tcp", "type=grpc&serviceName=stream", 1), 2)
	equal(t, n["transport"], object{"type": "grpc", "service_name": "stream"})
}
func TestHyTLSNoLimits(t *testing.T) {
	n := mustNode(t, hyURI, 9)
	equal(t, n["tag"], "Finland #9")
	equal(t, n["server_port"], 8446)
	equal(t, n["password"], "fake-password")
	equal(t, n["tls"], object{"enabled": true, "server_name": "cover.example"})
	for _, k := range []string{"network", "up_mbps", "down_mbps", "transport", "uuid"} {
		if n[k] != nil {
			t.Fatal(k)
		}
	}
}
func TestHyIPv6EncodedAuth(t *testing.T) {
	n := mustNode(t, "hy2://fake%3Apass%2Bword%40%2F%3F%23%25@[2001:db8::1]:8446/?sni=cover%2Eexample&insecure=1#Finland%20Gaming", 9)
	equal(t, n["server"], "2001:db8::1")
	equal(t, n["password"], "fake:pass+word@/?#%")
	equal(t, obj(n["tls"])["insecure"], true)
	equal(t, n["tag"], "Finland Gaming #9")
}
func TestHyUserpassDefaultPort(t *testing.T) {
	n := mustNode(t, "hysteria2://fake-user:fake+password@hy2.example", 3)
	equal(t, n["password"], "fake-user:fake+password")
	equal(t, n["server_port"], 443)
	equal(t, n["tag"], "node3 #3")
	n = mustNode(t, "hy2://fake-password@[2001:db8::1]", 4)
	equal(t, n["server_port"], 443)
}
func TestHyObfuscation(t *testing.T) {
	n := mustNode(t, strings.Replace(hyURI, "insecure=0", "obfs=salamander&obfs-password=fake%2Bpass%26word", 1), 9)
	equal(t, n["obfs"], object{"type": "salamander", "password": "fake+pass&word"})
}
func TestHyMultiPort(t *testing.T) {
	for _, host := range []string{"hy2.example", "[2001:db8::1]"} {
		n := mustNode(t, "hy2://fake-password@"+host+":443,5000-6000,8446/", 9)
		equal(t, n["server_ports"], []string{"443", "5000:6000", "8446"})
		equal(t, n["server_port"], nil)
	}
}
func TestInvalidNodesRedacted(t *testing.T) {
	for _, s := range []string{"hy2://hy2.example:443", "hy2://@hy2.example:443", "hy2://fake-password@:443", "hy2://fake-password@[invalid]:443", "hy2://fake-password@hy2.example:65536", "hy2://fake-password@hy2.example:0", "hy2://fake-password@hy2.example:5000-4000", "hy2://fake-password@hy2.example:443,,8446", "hy2://fake-password@[2001:db8::1]8446", "hy2://fake-password@hy2.example:443/extra", "hy2://fake-password@hy2.example?insecure=fake-password", "hy2://fake-password@hy2.example?obfs=salamander", "hy2://fake-password@hy2.example?obfs=fake-password&obfs-password=fake-password", "hy2://fake-password@hy2.example?obfs-password=fake-password", "hy2://fake-password@hy2.example?pinSHA256=fake-password", "hy2://fake-password@hy2.example?ech=fake-password", "hy2://fake%FFpassword@hy2.example", strings.Replace(vlessURI, ":443?", ":99999?", 1)} {
		_, e := parseNode(s, 9)
		if e == nil {
			t.Fatalf("accepted invalid fixture %q", s)
		}
		if strings.Contains(e.Error(), "fake-password") || strings.Contains(e.Error(), s) {
			t.Fatal(e)
		}
	}
}
func TestSubscriptionRejectsUnknownSafely(t *testing.T) {
	var log bytes.Buffer
	n := parseSubscription(strings.Join([]string{"trojan://fake-password@secret.example:443#secret-name", strings.Replace(vlessURI, "type=tcp", "type=xhttp", 1), strings.Replace(vlessURI, "type=tcp", "type=fake-password", 1), hyURI}, "\n"), &log)
	equal(t, len(n), 1)
	equal(t, n[0]["tag"], "Finland #3")
	for _, s := range []string{"skip line 0: unsupported protocol", "skip line 1: xhttp transport is not supported", "skip line 2: unsupported VLESS transport"} {
		if !strings.Contains(log.String(), s) {
			t.Fatal(log.String())
		}
	}
	for _, s := range []string{"fake-password", "00000000", "secret.example"} {
		if strings.Contains(log.String(), s) {
			t.Fatal(log.String())
		}
	}
}
func preferredNodes() []object {
	return parseSubscription(strings.Join([]string{vlessURI, strings.Replace(hyURI, "#Finland", "#Hysteria%20Finland", 1), strings.Replace(strings.Replace(vlessURI, "vless.example", "estonia.example", 1), "#Germany", "#Estonia", 1), strings.Replace(strings.Replace(hyURI, "hy2.example", "other-hy2.example", 1), "#Finland", "#Hysteria%20Other", 1)}, "\n"), io.Discard)
}
func TestStablePreferenceBeforeCap(t *testing.T) {
	n := preferredNodes()
	before := string(jsonBytes(n))
	for _, c := range []struct {
		prefer string
		limit  int
		want   []string
	}{{"", 2, []string{"Germany #0", "Hysteria Finland #1"}}, {"hYsTeRiA", 2, []string{"Hysteria Finland #1", "Hysteria Other #3"}}, {"Hysteria", 4, []string{"Hysteria Finland #1", "Hysteria Other #3", "Germany #0", "Estonia #2"}}, {"no-match", 2, []string{"Germany #0", "Hysteria Finland #1"}}} {
		p, e := autoPool(n, "", "", c.limit, c.prefer)
		if e != nil {
			t.Fatal(e)
		}
		equal(t, p, c.want)
	}
	equal(t, string(jsonBytes(n)), before)
}
func TestPreferenceHonorsFilters(t *testing.T) {
	n := preferredNodes()
	p, e := autoPool(n, "", "Hysteria", 2, "Hysteria")
	if e != nil {
		t.Fatal(e)
	}
	equal(t, p, []string{"Germany #0", "Estonia #2"})
	p, e = autoPool(n, "Estonia", "", 2, "Hysteria")
	if e != nil {
		t.Fatal(e)
	}
	equal(t, p, []string{"Estonia #2"})
}
func TestDedupePrefersAliasKeepsManual(t *testing.T) {
	n := parseSubscription(vlessURI+"\n"+strings.Replace(vlessURI, "#Germany", "#Preferred", 1), io.Discard)
	p, e := autoPool(n, "", "", 2, "Preferred")
	if e != nil {
		t.Fatal(e)
	}
	equal(t, p, []string{"Preferred #1"})
	equal(t, len(n), 2)
}
func TestAutoFilterCapDedupe(t *testing.T) {
	n := parseSubscription(vlessURI+"\n"+hyURI+"\n"+vlessURI+"\n"+hyURI, io.Discard)
	before := string(jsonBytes(n))
	p, e := autoPool(n, "", "", 8, "")
	if e != nil {
		t.Fatal(e)
	}
	equal(t, p, []string{"Germany #0", "Finland #1"})
	p, e = autoPool(n, "", "Finland", 1, "")
	if e != nil {
		t.Fatal(e)
	}
	equal(t, p, []string{"Germany #0"})
	for _, c := range []struct {
		in, ex string
		max    int
	}{{"[", "", 1}, {"", ".*", 1}, {"", "", 0}} {
		if _, e = autoPool(n, c.in, c.ex, c.max, ""); e == nil {
			t.Fatal("accepted invalid pool")
		}
	}
	equal(t, string(jsonBytes(n)), before)
}
func generatorFixture(t *testing.T, source, extra string) (string, string) {
	t.Helper()
	fakeCommands(t)
	s := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "sing-box" {
			t.Error("missing source agent")
		}
		io.WriteString(w, source)
	}))
	t.Cleanup(s.Close)
	dir := t.TempDir()
	for name, data := range map[string]string{"config.json": "previous valid config", "secret.txt": "fake-panel-secret", "geoip-ru.srs": "fixture", "geosite-ads.srs": "fixture", "vpn.env": "SUB_URL=" + s.URL + "/fake-token\nAUTO_EXCLUDE=\n" + extra} {
		if e := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); e != nil {
			t.Fatal(e)
		}
	}
	return filepath.Join(dir, "vpn.env"), dir
}
func TestInvalidPreferenceLeavesConfigUntouched(t *testing.T) {
	conf, dir := generatorFixture(t, hyURI, "AUTO_MAX=1\nAUTO_PREFER=[\n")
	e := generate(context.Background(), conf, dir, io.Discard, io.Discard)
	if e == nil || !strings.Contains(e.Error(), "AUTO_PREFER") {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	equal(t, string(b), "previous valid config")
	if _, e := os.Stat(os.Getenv("VPN_TEST_TRACE")); !os.IsNotExist(e) {
		t.Fatal("validator invoked")
	}
}
func TestGeneratedSelectorKeepsAllNodes(t *testing.T) {
	conf, dir := generatorFixture(t, vlessURI+"\n"+hyURI+"\n"+vlessURI, "AUTO_EXCLUDE=Finland\nAUTO_MAX=1\n")
	var out bytes.Buffer
	if e := generate(context.Background(), conf, dir, &out, io.Discard); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	var c object
	json.Unmarshal(b, &c)
	obs := c["outbounds"].([]any)
	equal(t, obj(obs[0])["outbounds"], []any{"auto", "Germany #0", "Finland #1", "Germany #2"})
	equal(t, obj(obs[1])["outbounds"], []any{"Germany #0"})
	equal(t, obj(obs[3])["type"], "hysteria2")
	s, _ := os.Stat(filepath.Join(dir, "config.json"))
	equal(t, s.Mode().Perm(), os.FileMode(0600))
	if !strings.Contains(out.String(), "3 manual nodes, 1 auto nodes") {
		t.Fatal(out.String())
	}
}
func TestConfigCheckFailurePreservesAndRedacts(t *testing.T) {
	conf, dir := generatorFixture(t, hyURI, "AUTO_MAX=1\n")
	t.Setenv("VPN_TEST_CHECK_EXIT", "7")
	e := generate(context.Background(), conf, dir, io.Discard, io.Discard)
	if e == nil || !strings.Contains(e.Error(), "CONFIG CHECK FAILED") {
		t.Fatal(e)
	}
	if strings.Contains(e.Error(), "fake") {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.json"))
	equal(t, string(b), "previous valid config")
	matches, _ := filepath.Glob(filepath.Join(dir, ".config-*"))
	equal(t, len(matches), 0)
}

func TestConfigValidationAtomicFileSeam(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failure], func(t *testing.T) {
			fakeCommands(t)
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			os.WriteFile(path, []byte("old config"), 0600)
			nodes := parseSubscription(vlessURI+"\n"+hyURI+"\n"+vlessURI, io.Discard)
			auto, e := autoPool(nodes, "", "Finland", 1, "")
			if e != nil {
				t.Fatal(e)
			}
			cfg, e := buildConfig(nodes, auto, map[string]string{}, dir, "fake-panel-secret")
			if e != nil {
				t.Fatal(e)
			}
			if failure {
				t.Setenv("VPN_TEST_CHECK_EXIT", "7")
			}
			e = validateAndReplace(context.Background(), dir, cfg)
			if (e != nil) != failure {
				t.Fatal(e)
			}
			b, _ := os.ReadFile(path)
			if failure {
				equal(t, string(b), "old config")
				if strings.Contains(e.Error(), "fake") {
					t.Fatal(e)
				}
			} else {
				var got object
				json.Unmarshal(b, &got)
				outbounds := got["outbounds"].([]any)
				equal(t, obj(outbounds[0])["outbounds"], []any{"auto", "Germany #0", "Finland #1", "Germany #2"})
				equal(t, obj(outbounds[1])["outbounds"], []any{"Germany #0"})
				s, _ := os.Stat(path)
				equal(t, s.Mode().Perm(), os.FileMode(0600))
			}
			rows := traces(t)
			equal(t, len(rows), 1)
			equal(t, rows[0]["mode"], float64(0600))
			tmp := str(rows[0]["path"])
			if _, e = os.Stat(tmp); !os.IsNotExist(e) {
				t.Fatal("temporary config survives")
			}
		})
	}
}
func TestGeneratorDNSWhitelistAndPortValidation(t *testing.T) {
	nodes := parseSubscription(hyURI, io.Discard)
	for _, ad := range []string{"0", "1"} {
		c, e := buildConfig(nodes, []string{"Finland #0"}, map[string]string{"ADBLOCK": ad}, "/fixture", "fake-secret")
		if e != nil {
			t.Fatal(e)
		}
		rules := obj(c["dns"])["rules"].([]object)
		equal(t, rules[0]["domain"], []string{"aquadx.hydev.org"})
		if ad == "1" {
			equal(t, rules[1]["domain_suffix"], []string{"tmdb.org", "themoviedb.org", "b-cdn.net"})
			equal(t, rules[2]["action"], "reject")
		} else {
			equal(t, len(rules), 3)
		}
		route := obj(c["route"])
		r := route["rules"].([]object)
		equal(t, r[2]["inbound"], []string{"health-in"})
		equal(t, r[2]["outbound"], "select")
	}
	for _, conf := range []map[string]string{{"HEALTH_PORT": "7895"}, {"HEALTH_PORT": "0"}, {"CLASH_PORT": "65536"}} {
		if _, e := buildConfig(nodes, nil, conf, "/fixture", "fake"); e == nil {
			t.Fatal("invalid ports accepted")
		}
	}
	if _, e := buildConfig(nodes, nil, nil, "/fixture", ""); e == nil {
		t.Fatal("empty panel secret accepted")
	}
}

func TestVlessBlankQueryAndHysteriaLiteralPercent(t *testing.T) {
	n := mustNode(t, strings.Replace(vlessURI, "type=tcp", "type=grpc&type=&serviceName=literal%25service", 1), 0)
	equal(t, n["transport"], object{"type": "grpc", "service_name": "literal%service"})
	n = mustNode(t, "hy2://literal%password@hy2.example#Literal%name", 1)
	equal(t, n["password"], "literal%password")
	equal(t, n["tag"], "Literal%name #1")
}
