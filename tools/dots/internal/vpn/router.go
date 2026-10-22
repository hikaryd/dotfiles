package vpn

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"regexp"
	"strings"
	"time"
)

type router struct {
	ctx    context.Context
	client *http.Client
	base   string
	env    map[string]string
	out    io.Writer
}

func (r *router) v(k, d string) string { return value(r.env, k, d) }
func (r *router) raw(path string, data any) ([]byte, http.Header, int, error) {
	method := "GET"
	var body io.Reader
	if data != nil {
		b, e := json.Marshal(data)
		if e != nil {
			return nil, nil, 0, errors.New("HTTP RCI request failed")
		}
		body = bytes.NewReader(b)
		method = "POST"
	}
	req, e := http.NewRequestWithContext(r.ctx, method, r.base+path, body)
	if e != nil {
		return nil, nil, 0, errors.New("HTTP RCI request failed")
	}
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, e := r.client.Do(req)
	if e != nil {
		return nil, nil, 0, errors.New("HTTP RCI request failed")
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 4<<20+1))
	if e != nil || len(b) > 4<<20 {
		return nil, nil, 0, errors.New("HTTP RCI response failed")
	}
	return b, resp.Header, resp.StatusCode, nil
}
func (r *router) auth() error {
	_, h, status, e := r.raw("/auth", nil)
	if e != nil {
		return e
	}
	if status >= 200 && status < 300 {
		return nil
	}
	realm, ch := h.Get("X-NDM-Realm"), h.Get("X-NDM-Challenge")
	if realm == "" || ch == "" {
		return errors.New("HTTP RCI authentication failed")
	}
	digest := md5.Sum([]byte(r.v("ROUTER_USER", "admin") + ":" + realm + ":" + r.env["ROUTER_PASS"]))
	sha := sha256.Sum256([]byte(ch + hex.EncodeToString(digest[:])))
	_, _, status, e = r.raw("/auth", object{"login": r.v("ROUTER_USER", "admin"), "password": hex.EncodeToString(sha[:])})
	if e != nil || status < 200 || status >= 300 {
		return errors.New("HTTP RCI authentication failed")
	}
	return nil
}
func (r *router) get(path string, data any) (any, error) {
	b, _, code, e := r.raw(path, data)
	if e != nil {
		return nil, e
	}
	if code < 200 || code >= 300 {
		return nil, errors.New("HTTP RCI request failed")
	}
	var v any
	if json.Unmarshal(b, &v) != nil {
		return nil, errors.New("HTTP RCI invalid JSON response")
	}
	return v, nil
}
func (r *router) getInterface(name string) (object, error) {
	v, e := r.get("/rci/show", []object{{"interface": object{"name": name}}})
	if e != nil {
		return nil, e
	}
	a, ok := v.([]any)
	if !ok || len(a) != 1 || obj(a[0]) == nil {
		return nil, errors.New("Некорректный ответ HTTP RCI для интерфейса")
	}
	m, ok := obj(a[0])["interface"].(map[string]any)
	if !ok {
		return nil, errors.New("HTTP RCI не вернул интерфейс")
	}
	if len(m) > 0 && m["id"] != name && m["interface-name"] != name {
		return nil, errors.New("HTTP RCI не подтвердил имя интерфейса")
	}
	return m, nil
}
func (r *router) globals() ([]string, error) {
	v, e := r.get("/rci/show/interface", nil)
	if e != nil {
		return nil, e
	}
	m := obj(v)
	if len(m) == 0 {
		return nil, errors.New("HTTP RCI вернул некорректный инвентарь интерфейсов")
	}
	globals := []string{}
	for _, v := range m {
		i := obj(v)
		name := str(i["id"])
		if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_./-]*$`).MatchString(name) {
			return nil, errors.New("HTTP RCI вернул некорректный интерфейс в инвентаре")
		}
		if g, exists := i["global"]; exists {
			b, ok := g.(bool)
			if !ok {
				return nil, errors.New("HTTP RCI вернул некорректный интерфейс в инвентаре")
			}
			if b {
				globals = append(globals, name)
			}
		}
	}
	return unique(globals), nil
}
func (r *router) batch(cmds []string) ([][2]string, error) {
	keys := []string{r.env["ROUTER_KEY"], r.env["ROUTER_PASS"]}
	rx := regexp.MustCompile(`\bprivate-key\s+(\S+)`)
	data := []object{}
	for _, c := range cmds {
		data = append(data, object{"parse": c})
		if m := rx.FindStringSubmatch(c); m != nil {
			keys = append(keys, m[1])
		}
	}
	redact := func(s string) string {
		for _, k := range keys {
			if k != "" {
				s = strings.ReplaceAll(s, k, "[REDACTED]")
			}
		}
		return s
	}
	v, e := r.get("/rci/", data)
	if e != nil {
		return nil, e
	}
	a, ok := v.([]any)
	if !ok || len(a) != len(cmds) {
		return nil, errors.New("HTTP RCI вернул неполный ответ на команды")
	}
	res := [][2]string{}
	for i, c := range cmds {
		parsed := obj(obj(a[i])["parse"])
		if len(parsed) == 1 && parsed["prompt"] == "(config-wg-peer)" && regexp.MustCompile(`^wireguard peer \S+$`).MatchString(c) {
			res = append(res, [2]string{redact(c), "(config-wg-peer)"})
			continue
		}
		st, ok := parsed["status"].([]any)
		if !ok || len(st) == 0 {
			return nil, fmt.Errorf("HTTP RCI не вернул статус команды: %s", redact(c))
		}
		messages := []string{}
		hasError := false
		for _, s := range st {
			m := obj(s)
			status, ok := m["status"].(string)
			_, msgOK := m["message"].(string)
			_, msgExists := m["message"]
			if !ok || (msgExists && !msgOK) {
				return nil, fmt.Errorf("HTTP RCI не вернул статус команды: %s", redact(c))
			}
			msg := str(m["message"])
			if msg == "" {
				msg = status
			}
			messages = append(messages, msg)
			hasError = hasError || status == "error"
		}
		if (c == "no ip name-server 8.8.8.8" || c == "no ip name-server 77.88.8.8") && len(st) == 1 {
			m := obj(st[0])
			ip := strings.TrimPrefix(c, "no ip name-server ")
			if len(m) == 4 && m["status"] == "error" && m["code"] == "22544585" && m["ident"] == "Dns::Manager" && m["message"] == "no such server: "+ip+"." {
				res = append(res, [2]string{c, "уже отсутствует"})
				continue
			}
		}
		msg := redact(strings.Join(messages, "; "))
		if hasError {
			return nil, fmt.Errorf("Ошибка HTTP RCI: %s", msg)
		}
		res = append(res, [2]string{redact(c), msg})
	}
	return res, nil
}
func truncate(s string, n int) string { r := []rune(s); return string(r[:min(n, len(r))]) }
func (r *router) run(cmds []string, label string) error {
	fmt.Fprintf(r.out, "\n[ %s ]\n", label)
	res, e := r.batch(cmds)
	if e != nil {
		return e
	}
	for _, p := range res {
		fmt.Fprintf(r.out, "  · %-60s | %s\n", truncate(p[0], 60), truncate(p[1], 90))
	}
	return nil
}
func (r *router) identify() error {
	if e := r.auth(); e != nil {
		return e
	}
	name := ""
	if v, e := r.get("/rci/show/system", nil); e == nil {
		name = str(obj(v)["hostname"])
	}
	if name == "" {
		if v, e := r.get("/rci/show/version", nil); e == nil {
			name = str(obj(v)["device"])
			if name == "" {
				name = str(obj(v)["model"])
			}
		}
	}
	if name == "" {
		name = "router-" + r.v("ROUTER_HOST", "unknown")
	}
	slug := strings.Trim(regexp.MustCompile(`[^A-Za-z0-9_-]+`).ReplaceAllString(name, "-"), "-")
	if slug == "" {
		slug = "router"
	}
	pub := ""
	if si, e := r.getInterface(r.v("WG_IFACE", "Wireguard0")); e == nil {
		pub = str(obj(si["wireguard"])["public-key"])
	}
	fmt.Fprintf(r.out, "NAME=%s\nCURPUB=%s\n", slug, pub)
	return nil
}
func (r *router) install() error {
	for _, k := range []string{"SERVER_HOST", "SERVER_PUB", "ROUTER_KEY", "AWG_PARAMS"} {
		if r.env[k] == "" {
			return fmt.Errorf("missing required environment variable %s", k)
		}
	}
	if e := r.auth(); e != nil {
		return e
	}
	fmt.Fprintf(r.out, "Авторизация на %s — ок\n", r.base)
	lan := r.v("ROUTER_LAN_INTERFACE", "Home")
	iface := r.v("WG_IFACE", "Wireguard0")
	if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`).MatchString(lan) {
		return errors.New("Некорректное имя LAN-интерфейса")
	}
	si, e := r.getInterface(lan)
	if e != nil {
		return e
	}
	if len(si) == 0 {
		return fmt.Errorf("LAN-интерфейс %s не найден", lan)
	}
	globals, e := r.globals()
	if e != nil {
		return e
	}
	prefix := "interface " + iface
	serverPub := r.env["SERVER_PUB"]
	if e = r.run([]string{prefix, prefix + " description AWG-VPN", prefix + " wireguard asc " + r.env["AWG_PARAMS"], prefix + " security-level public", prefix + " ip address " + r.v("TUN_CLI", "10.8.2.2") + " " + r.v("TUN_MASK", "255.255.255.0"), prefix + " ip mtu " + r.v("MTU", "1280"), prefix + " wireguard private-key " + r.env["ROUTER_KEY"]}, "Интерфейс + ключ + AWG"); e != nil {
		return e
	}
	si, e = r.getInterface(iface)
	if e == nil {
		stale := []string{}
		peers, _ := obj(si["wireguard"])["peer"].([]any)
		for _, p := range peers {
			pk := str(obj(p)["public-key"])
			if pk != "" && pk != serverPub {
				stale = append(stale, prefix+" no wireguard peer "+pk)
			}
		}
		if len(stale) > 0 {
			e = r.run(stale, "Удаление чужих пиров")
		}
	}
	if e != nil {
		fmt.Fprintln(r.out, "peer cleanup skipped: HTTP RCI cleanup failed")
	}
	batches := []struct {
		cmds  []string
		label string
	}{{[]string{prefix, "wireguard peer " + serverPub, "endpoint " + r.env["SERVER_HOST"] + ":" + r.v("AWG_PORT", "51820"), "keepalive-interval 25", "allow-ips 0.0.0.0/0", "exit", "exit", prefix + " up"}, "Пир + endpoint + поднятие"}, {[]string{prefix + " ip global " + r.v("PRIORITY", "1000")}, "Default route через WG"}, {[]string{"ping-check profile WGCHECK", "host " + r.v("TUN_SRV", "10.8.2.1"), "update-interval 5", "max-fails 3", "timeout 3", "exit", prefix + " ping-check profile WGCHECK"}, "Failover (ping-check)"}}
	policy := []string{"ip policy AWG_LAN", "description AWG-LAN", "no permit auto", "permit global " + iface}
	for _, name := range globals {
		if name != iface {
			policy = append(policy, "no permit global "+name)
		}
	}
	policy = append(policy, "exit", "ip hotspot policy "+lan+" AWG_LAN")
	batches = append(batches, struct {
		cmds  []string
		label string
	}{policy, "LAN через WG (HTTP RCI)"}, struct {
		cmds  []string
		label string
	}{[]string{"no ip name-server 8.8.8.8", "no ip name-server 77.88.8.8", "ip name-server 1.1.1.1", "ip name-server 9.9.9.9", "no ip dhcp pool " + r.v("DHCP_POOL", "_WEBADMIN") + " dns-server", "ip dhcp pool " + r.v("DHCP_POOL", "_WEBADMIN") + " dns-server " + r.v("ROUTER_LAN_IP", r.env["ROUTER_HOST"])}, "DNS через туннель + DHCP"})
	for _, b := range batches {
		if e = r.run(b.cmds, b.label); e != nil {
			return e
		}
	}
	routes, e := r.get("/rci/show/ip/route", nil)
	if e == nil {
		a, ok := routes.([]any)
		if !ok {
			e = errors.New("invalid routes")
		}
		for _, v := range a {
			m := obj(v)
			d := str(m["destination"])
			if d == "8.8.8.8" || d == "8.8.8.8/32" || d == "77.88.8.8" || d == "77.88.8.8/32" {
				e = r.run([]string{"no ip route " + d + " " + str(m["interface"])}, "Удаление WAN-маршрута "+d)
				if e != nil {
					break
				}
			}
		}
	}
	if e != nil {
		fmt.Fprintln(r.out, "route cleanup skipped: HTTP RCI cleanup failed")
	}
	if e = r.run([]string{"system configuration save"}, "Сохранение конфигурации"); e != nil {
		return e
	}
	select {
	case <-r.ctx.Done():
		return errors.New("router setup interrupted")
	case <-time.After(5 * time.Second):
	}
	si, e = r.getInterface(iface)
	if e != nil {
		return e
	}
	peer := object{}
	if a, ok := obj(si["wireguard"])["peer"].([]any); ok && len(a) > 0 {
		peer = obj(a[0])
	}
	fmt.Fprintf(r.out, "\nИтог: link=%v connected=%v global=%v | peer online=%v handshake=%vs\n", si["link"], si["connected"], si["global"], peer["online"], peer["last-handshake"])
	return nil
}
func routerSetup(ctx context.Context, out io.Writer) error {
	values := map[string]string{}
	for _, s := range os.Environ() {
		k, v, _ := strings.Cut(s, "=")
		values[k] = v
	}
	if values["ROUTER_HOST"] == "" || values["ROUTER_PASS"] == "" {
		return errors.New("ROUTER_HOST and ROUTER_PASS are required")
	}
	jar, _ := cookiejar.New(nil)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	r := router{ctx, &http.Client{Timeout: 25 * time.Second, Jar: jar, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, "http://" + values["ROUTER_HOST"], values, out}
	defer transport.CloseIdleConnections()
	switch r.v("ACTION", "install") {
	case "identify":
		return r.identify()
	case "install":
		return r.install()
	default:
		return errors.New("неизвестный ACTION (install|identify)")
	}
}
