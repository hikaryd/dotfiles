package vpn

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(code int, v any) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(jsonBytes(v)))}
}
func testRouter(t *testing.T, fn roundTripFunc) (*router, *bytes.Buffer) {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	out := &bytes.Buffer{}
	return &router{context.Background(), &http.Client{Transport: fn, Jar: jar}, "http://router.invalid", map[string]string{"ROUTER_HOST": "router.invalid", "ROUTER_PASS": "fake-password", "ROUTER_KEY": "fake-private-key"}, out}, out
}
func TestRouterChallengeAuthentication(t *testing.T) {
	calls := 0
	r, _ := testRouter(t, func(req *http.Request) (*http.Response, error) {
		calls++
		equal(t, req.URL.Path, "/auth")
		if calls == 1 {
			equal(t, req.Method, "GET")
			r := response(401, nil)
			r.Header.Set("X-NDM-Realm", "fixture-realm")
			r.Header.Set("X-NDM-Challenge", "fixture-challenge")
			r.Header.Set("Set-Cookie", "challenge=fixture; Path=/")
			return r, nil
		}
		equal(t, req.Method, "POST")
		equal(t, req.Header.Get("Content-Type"), "application/json")
		c, e := req.Cookie("challenge")
		if e != nil || c.Value != "fixture" {
			t.Fatal("cookie missing")
		}
		var body object
		json.NewDecoder(req.Body).Decode(&body)
		equal(t, body["login"], "admin")
		md := md5.Sum([]byte("admin:fixture-realm:fake-password"))
		sha := sha256.Sum256([]byte("fixture-challenge" + hex.EncodeToString(md[:])))
		equal(t, body["password"], hex.EncodeToString(sha[:]))
		return response(200, object{}), nil
	})
	if e := r.auth(); e != nil {
		t.Fatal(e)
	}
	equal(t, calls, 2)
}
func TestRouterAuthMissingChallengeAndNetworkRedacted(t *testing.T) {
	for _, fail := range []bool{false, true} {
		r, _ := testRouter(t, func(req *http.Request) (*http.Response, error) {
			if fail {
				return nil, errors.New("fake-password private-url")
			}
			return response(401, object{}), nil
		})
		e := r.auth()
		if e == nil || strings.Contains(e.Error(), "fake") {
			t.Fatal(e)
		}
	}
}
func TestRouterBatchRedactionAndStrictStatus(t *testing.T) {
	for _, result := range []any{object{}, []any{}, []any{object{"parse": object{}}}, []any{object{"parse": object{"status": []any{object{"status": "ok", "message": nil}}}}}, []any{object{"parse": object{"status": []any{object{"status": "error", "message": "bad fake-private-key"}}}}}} {
		r, _ := testRouter(t, func(req *http.Request) (*http.Response, error) {
			equal(t, req.URL.Path, "/rci/")
			equal(t, req.Method, "POST")
			return response(200, result), nil
		})
		_, e := r.batch([]string{"interface Wireguard0 wireguard private-key fake-private-key"})
		if e == nil {
			t.Fatal("invalid response accepted")
		}
		if strings.Contains(e.Error(), "fake-private-key") {
			t.Fatal("key leaked")
		}
	}
	r, out := testRouter(t, func(req *http.Request) (*http.Response, error) {
		return response(200, []any{object{"parse": object{"status": []any{object{"status": "ok", "message": "installed fake-private-key"}}}}}), nil
	})
	if e := r.run([]string{"interface Wireguard0 wireguard private-key fake-private-key"}, "fixture"); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), "fake-private-key") || !strings.Contains(out.String(), "[REDACTED]") {
		t.Fatal(out.String())
	}
}
func TestRouterPeerPromptAndExactDNSIdempotence(t *testing.T) {
	for _, cmd := range []string{"wireguard peer fake-public-key", "no ip name-server 8.8.8.8", "no ip name-server 77.88.8.8"} {
		r, _ := testRouter(t, func(req *http.Request) (*http.Response, error) {
			parsed := object{"prompt": "(config-wg-peer)"}
			if strings.HasPrefix(cmd, "no ip") {
				parsed = object{"status": []object{{"status": "error", "code": "22544585", "ident": "Dns::Manager", "message": "no such server: " + strings.TrimPrefix(cmd, "no ip name-server ") + "."}}}
			}
			return response(200, []object{{"parse": parsed}}), nil
		})
		if _, e := r.batch([]string{cmd}); e != nil {
			t.Fatal(e)
		}
	}
	r, _ := testRouter(t, func(req *http.Request) (*http.Response, error) {
		return response(200, []object{{"parse": object{"prompt": "(config-wg-peer)"}}}), nil
	})
	if _, e := r.batch([]string{"interface Wireguard0"}); e == nil {
		t.Fatal("prompt accepted for unrelated command")
	}
}
func TestRouterInterfaceAliasAndInventoryValidation(t *testing.T) {
	r, _ := testRouter(t, func(req *http.Request) (*http.Response, error) {
		equal(t, req.URL.Path, "/rci/show")
		var b []object
		json.NewDecoder(req.Body).Decode(&b)
		equal(t, obj(b[0]["interface"])["name"], "Home")
		return response(200, []object{{"interface": object{"id": "Bridge0", "interface-name": "Home"}}}), nil
	})
	if _, e := r.getInterface("Home"); e != nil {
		t.Fatal(e)
	}
	for _, inventory := range []any{object{}, object{"x": object{"id": "bad name", "global": true}}, object{"x": object{"id": "Wireguard0", "global": "true"}}} {
		r, _ := testRouter(t, func(req *http.Request) (*http.Response, error) { return response(200, inventory), nil })
		if _, e := r.globals(); e == nil {
			t.Fatal("bad inventory accepted")
		}
	}
}
func TestRouterIdentifyFallbackSlug(t *testing.T) {
	r, out := testRouter(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/auth":
			return response(200, nil), nil
		case "/rci/show/system":
			return response(200, object{}), nil
		case "/rci/show/version":
			return response(200, object{"device": "Keenetic Ultra / Home"}), nil
		case "/rci/show":
			return response(200, []object{{"interface": object{"id": "Wireguard0", "wireguard": object{"public-key": "fake-public-key"}}}}), nil
		}
		return response(404, nil), nil
	})
	if e := r.identify(); e != nil {
		t.Fatal(e)
	}
	equal(t, out.String(), "NAME=Keenetic-Ultra-Home\nCURPUB=fake-public-key\n")
}
func TestRouterInstallPreflightBeforeMutation(t *testing.T) {
	writes := 0
	r, _ := testRouter(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/auth" {
			return response(200, nil), nil
		}
		if req.URL.Path == "/rci/show" {
			return response(200, []object{{"interface": object{}}}), nil
		}
		writes++
		return response(200, nil), nil
	})
	for _, k := range []string{"SERVER_HOST", "SERVER_PUB", "ROUTER_KEY", "AWG_PARAMS"} {
		r.env[k] = "fixture"
	}
	if e := r.install(); e == nil {
		t.Fatal("missing LAN accepted")
	}
	equal(t, writes, 0)
}
func TestRouterIdentifyHTTPServer(t *testing.T) {
	s := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth":
			io.WriteString(w, `{}`)
		case "/rci/show/system":
			io.WriteString(w, `{"hostname":"fixture-router"}`)
		case "/rci/show":
			io.WriteString(w, `[{"interface":{"id":"Wireguard0","wireguard":{"public-key":"fixture-public"}}}]`)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Setenv("ROUTER_HOST", strings.TrimPrefix(s.URL, "http://"))
	t.Setenv("ROUTER_PASS", "fake-password")
	t.Setenv("ACTION", "identify")
	t.Setenv("HTTP_PROXY", "http://unreachable.invalid:1")
	var out, errout bytes.Buffer
	equal(t, run([]string{"router-setup"}, &out, &errout), 0)
	equal(t, out.String(), "NAME=fixture-router\nCURPUB=fixture-public\n")
}

func TestRouterInstallPolicyPeersDNSAndSave(t *testing.T) {
	commands := []string{}
	r, _ := testRouter(t, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/auth":
			return response(200, nil), nil
		case "/rci/show":
			var body []object
			json.NewDecoder(req.Body).Decode(&body)
			name := str(obj(body[0]["interface"])["name"])
			if name == "Home" {
				return response(200, []object{{"interface": object{"id": "Bridge0", "interface-name": "Home"}}}), nil
			}
			return response(200, []object{{"interface": object{"id": "Wireguard0", "wireguard": object{"peer": []object{{"public-key": "stale-public-key"}, {"public-key": "server-public-key", "online": true}}}}}}), nil
		case "/rci/show/interface":
			return response(200, object{"one": object{"id": "ISP", "global": true}, "two": object{"id": "Wireguard0", "global": true}, "three": object{"id": "Bridge0", "global": false}}), nil
		case "/rci/show/ip/route":
			return response(200, []object{{"destination": "8.8.8.8/32", "interface": "ISP"}, {"destination": "1.2.3.4/32", "interface": "ISP"}}), nil
		case "/rci/":
			var body []object
			json.NewDecoder(req.Body).Decode(&body)
			results := []object{}
			for _, v := range body {
				c := str(v["parse"])
				commands = append(commands, c)
				p := object{"status": []object{{"status": "ok", "message": "fixture success"}}}
				if strings.HasPrefix(c, "wireguard peer ") {
					p = object{"prompt": "(config-wg-peer)"}
				}
				results = append(results, object{"parse": p})
			}
			return response(200, results), nil
		}
		return response(404, nil), nil
	})
	r.env["SERVER_HOST"] = "server.example"
	r.env["SERVER_PUB"] = "server-public-key"
	r.env["AWG_PARAMS"] = "4 10 100 20 30 40 50 60 70"
	if e := r.install(); e != nil {
		t.Fatal(e)
	}
	all := strings.Join(commands, "\n")
	for _, c := range []string{"interface Wireguard0 no wireguard peer stale-public-key", "wireguard peer server-public-key", "endpoint server.example:51820", "interface Wireguard0 ip global 1000", "no permit auto", "permit global Wireguard0", "no permit global ISP", "ip hotspot policy Home AWG_LAN", "ip dhcp pool _WEBADMIN dns-server router.invalid", "no ip route 8.8.8.8/32 ISP"} {
		if !strings.Contains(all, c) {
			t.Fatalf("missing command %s", c)
		}
	}
	equal(t, commands[len(commands)-1], "system configuration save")
	for _, c := range []string{"no permit global Wireguard0", "no permit global Bridge0", "no ip route 1.2.3.4", "no wireguard peer server-public-key"} {
		if strings.Contains(all, c) {
			t.Fatalf("unexpected command %s", c)
		}
	}
}
