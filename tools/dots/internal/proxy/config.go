package proxy

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"dots/native/internal/tui"
)

type object = map[string]any

func truthy(v any) bool {
	s := strings.ToLower(fmt.Sprint(v))
	return s == "1" || s == "true" || s == "yes"
}
func value(s object, k, fallback string) string {
	v, ok := s[k]
	if !ok || v == nil || v == "" {
		return fallback
	}
	return fmt.Sprint(v)
}
func copyFields(dst, src object, fields ...string) {
	for i := 0; i < len(fields); i += 2 {
		if v, ok := src[fields[i]]; ok && v != nil && v != "" {
			dst[fields[i+1]] = v
		}
	}
}

// VLESS subscriptions use urllib-style permissive unquoting: malformed
// percent escapes remain literal, and invalid UTF-8 decodes with replacement.
func unquoteComponent(s string, plus bool) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(n))
				i += 2
				continue
			}
		}
		if plus && s[i] == '+' {
			b.WriteByte(' ')
		} else {
			b.WriteByte(s[i])
		}
	}
	return strings.ToValidUTF8(b.String(), "\uFFFD")
}
func parseVLESS(link string) (object, error) {
	link = strings.TrimSpace(link)
	if !strings.HasPrefix(link, "vless://") {
		return nil, errors.New("Invalid VLESS URL. It must start with vless://")
	}
	base, name := link, "VLESS server"
	if i := strings.LastIndex(base, "#"); i >= 0 {
		n := unquoteComponent(base[i+1:], false)
		if n != "" {
			name = n
		}
		base = base[:i]
	}
	uuid, rest, ok := strings.Cut(strings.TrimPrefix(base, "vless://"), "@")
	if !ok || uuid == "" {
		return nil, errors.New("Invalid VLESS URL.")
	}
	hostPort, query, _ := strings.Cut(rest, "?")
	host, portText := hostPort, "443"
	if strings.HasPrefix(hostPort, "[") {
		end := strings.IndexByte(hostPort, ']')
		if end < 0 || len(hostPort) < end+3 || hostPort[end+1] != ':' {
			return nil, errors.New("Invalid VLESS URL.")
		}
		host, portText = hostPort[1:end], hostPort[end+2:]
	} else if i := strings.LastIndexByte(hostPort, ':'); i >= 0 {
		host, portText = hostPort[:i], hostPort[i+1:]
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 || host == "" {
		return nil, errors.New("Invalid VLESS URL.")
	}
	params := map[string][]string{}
	for _, field := range strings.Split(query, "&") {
		k, v, _ := strings.Cut(field, "=")
		if v != "" {
			key := unquoteComponent(k, true)
			params[key] = append(params[key], unquoteComponent(v, true))
		}
	}
	s := object{"name": name, "uuid": uuid, "address": host, "port": port, "raw_url": link}
	for _, k := range []string{"type", "security", "sni", "host", "path", "flow", "fp", "pbk", "sid", "spx", "alpn", "encryption", "headerType", "serviceName", "mode", "allowInsecure"} {
		def := ""
		switch k {
		case "type":
			def = "tcp"
		case "security", "encryption":
			def = "none"
		case "allowInsecure":
			def = "0"
		}
		vals := []string{}
		for _, v := range params[k] {
			if v != "" {
				vals = append(vals, v)
			}
		}
		switch len(vals) {
		case 0:
			s[k] = def
		case 1:
			s[k] = vals[0]
		default:
			s[k] = vals
		}
	}
	return s, nil
}
func streamSettings(s object) object {
	network, security := value(s, "type", "tcp"), value(s, "security", "none")
	stream := object{"network": network}
	switch security {
	case "tls":
		tls := object{}
		copyFields(tls, s, "sni", "serverName", "fp", "fingerprint")
		if alpn := value(s, "alpn", ""); alpn != "" {
			tls["alpn"] = strings.Split(alpn, ",")
		}
		if truthy(s["allowInsecure"]) {
			tls["allowInsecure"] = true
		}
		stream["security"], stream["tlsSettings"] = "tls", tls
	case "reality":
		reality := object{}
		copyFields(reality, s, "sni", "serverName", "fp", "fingerprint", "pbk", "publicKey", "sid", "shortId", "spx", "spiderX")
		stream["security"], stream["realitySettings"] = "reality", reality
	}
	settings := object{}
	switch network {
	case "ws":
		copyFields(settings, s, "path", "path")
		if host := value(s, "host", ""); host != "" {
			settings["headers"] = object{"Host": s["host"]}
		}
		stream["wsSettings"] = settings
	case "grpc":
		copyFields(settings, s, "serviceName", "serviceName")
		if mode := value(s, "mode", ""); mode != "" {
			settings["multiMode"] = mode == "multi"
		}
		stream["grpcSettings"] = settings
	case "tcp":
		if value(s, "headerType", "") == "http" {
			stream["tcpSettings"] = object{"header": object{"type": "http", "request": object{"path": []string{value(s, "path", "/")}, "headers": object{"Host": []string{value(s, "host", value(s, "sni", ""))}}}}}
		}
	case "h2":
		if host := value(s, "host", ""); host != "" {
			settings["host"] = []any{s["host"]}
		}
		copyFields(settings, s, "path", "path")
		stream["httpSettings"] = settings
	case "httpupgrade":
		copyFields(settings, s, "path", "path", "host", "host")
		stream["httpupgradeSettings"] = settings
	case "xhttp", "splithttp":
		copyFields(settings, s, "path", "path", "host", "host", "mode", "mode")
		stream["xhttpSettings"] = settings
	}
	return stream
}
func (c *controller) config(s object) (object, error) {
	port, err := strconv.Atoi(fmt.Sprint(s["port"]))
	if err != nil || port < 1 || port > 65535 || value(s, "uuid", "") == "" || value(s, "address", "") == "" {
		return nil, errors.New("Invalid cached VLESS server configuration.")
	}
	user := object{"id": s["uuid"], "encryption": value(s, "encryption", "none")}
	copyFields(user, s, "flow", "flow")
	return object{
		"log":      object{"loglevel": "warning"},
		"inbounds": []any{object{"port": c.port, "listen": "127.0.0.1", "protocol": "http", "tag": "http-in", "settings": object{"timeout": 300}}},
		// Clients launched from an old terminal or Herdr can inherit stale NO_PROXY.
		// Keep the internal MCP endpoint reachable through the local proxy too.
		"routing": object{"rules": []any{object{
			"type": "field", "domain": []string{"full:nebula-staging.x5.ru"}, "outboundTag": "direct",
		}}},
		"outbounds": []any{
			object{"protocol": "vless", "settings": object{"vnext": []any{object{"address": s["address"], "port": port, "users": []any{user}}}}, "streamSettings": streamSettings(s), "tag": "proxy"},
			object{"protocol": "freedom", "tag": "direct"}, object{"protocol": "blackhole", "tag": "block"},
		},
	}, nil
}
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), 0600)
}
func (c *controller) saveServers(servers []object) error {
	if err := writeJSON(c.path("servers.json"), servers); err != nil {
		return errors.New("Could not save VLESS server cache.")
	}
	return nil
}
func (c *controller) setSingle() error {
	link, err := c.prompt("VLESS config URL")
	if err != nil {
		return err
	}
	s, err := parseVLESS(link)
	if err != nil {
		return err
	}
	if err := c.saveServers([]object{s}); err != nil {
		return err
	}
	fmt.Fprintf(c.out, "Saved 1 VLESS server: %s\n", s["name"])
	return nil
}
func (c *controller) saveSubscription(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("Subscription URL cannot be empty.")
	}
	if err := atomicWrite(c.path("subscription_url.txt"), []byte(s+"\n"), 0600); err != nil {
		return errors.New("Could not save subscription URL.")
	}
	fmt.Fprintln(c.out, "Subscription URL saved.")
	return nil
}
func (c *controller) subscription() (string, error) {
	s, err := readText(c.path("subscription_url.txt"))
	if err == nil {
		return strings.TrimSpace(s), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("Could not read subscription URL.")
	}
	s, err = c.prompt("VLESS subscription URL")
	if err != nil {
		return "", err
	}
	if s == "" {
		return "", errors.New("URL cannot be empty.")
	}
	return s, c.saveSubscription(s)
}
func decodePayload(raw []byte) string {
	text := strings.TrimSpace(strings.ToValidUTF8(string(raw), ""))
	if strings.Contains(text, "vless://") {
		return text
	}
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, text)
	// Python's b64decode(validate=False) ignores non-alphabet characters.
	padded := compact + strings.Repeat("=", (4-len(compact)%4)%4)
	padded = strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '/' || r == '=' {
			return r
		}
		return -1
	}, padded)
	b, err := base64.StdEncoding.DecodeString(padded)
	if err != nil {
		return text
	}
	return strings.ToValidUTF8(string(b), "")
}
func (c *controller) updateServers(insecure bool) ([]object, error) {
	u, err := c.subscription()
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(c.out, "Fetching subscription...")
	links, err := c.fetchSubscription(u, insecure)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(c.out, "Found %d VLESS link(s).\n", len(links))
	servers := []object{}
	for _, link := range links {
		s, err := parseVLESS(link)
		if err != nil {
			fmt.Fprintln(c.errOut, "Warning: failed to parse VLESS URL (invalid configuration)")
			continue
		}
		servers = append(servers, s)
	}
	if err := c.saveServers(servers); err != nil {
		return nil, err
	}
	fmt.Fprintf(c.out, "Saved %d server(s) to cache: %s\n", len(servers), c.path("servers.json"))
	return servers, nil
}
func (c *controller) loadServers() ([]object, error) {
	data, err := os.ReadFile(c.path("servers.json"))
	if errors.Is(err, os.ErrNotExist) {
		return c.updateServers(false)
	}
	if err != nil {
		return nil, errors.New("Could not read VLESS server cache.")
	}
	var servers []object
	if json.Unmarshal(data, &servers) != nil {
		return nil, errors.New("Invalid VLESS server cache.")
	}
	return servers, nil
}
func (c *controller) display(servers []object) {
	fmt.Fprintf(c.out, "\n%3s  %-45s %-30s %5s  %-10s %-10s\n", "#", "Name", "Address", "Port", "Transport", "Security")
	fmt.Fprintln(c.out, strings.Repeat("-", 115))
	for i, s := range servers {
		address := value(s, "address", "")
		r := []rune(address)
		if len(r) > 30 {
			address = string(r[:27]) + "..."
		}
		fmt.Fprintf(c.out, "%3d  %-45s %-30s %5s  %-10s %-10s\n", i+1, value(s, "name", ""), address, value(s, "port", ""), value(s, "type", ""), value(s, "security", ""))
	}
}
func (c *controller) selectAndStart() error {
	servers, err := c.loadServers()
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		return errors.New("No servers found. Use codex-proxy --set-vless or codex-proxy --set-sub.")
	}
	if c.interactive() {
		rows := make([]string, len(servers))
		for i, server := range servers {
			rows[i] = fmt.Sprintf("%s\t%s\t%s\t%s\t%s", tui.Clean(value(server, "name", "")), tui.Clean(value(server, "address", "")), tui.Clean(value(server, "port", "")), tui.Clean(value(server, "type", "")), tui.Clean(value(server, "security", "")))
		}
		selected, err := tui.Select(c.ctx, tui.Frame{Brand: "dots proxy", Title: "Выбор сервера", Mode: "LOCAL CONFIG", Tone: tui.Yellow}, rows, "", false)
		if err != nil {
			return err
		}
		return c.start(servers[selected[0]])
	}
	c.display(servers)
	fmt.Fprintf(c.out, "\nSelect server (1-%d):\n", len(servers))
	s, err := c.prompt("> ")
	if err != nil {
		return errors.New("Invalid selection.")
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return errors.New("Invalid selection.")
	}
	if n < 1 || n > len(servers) {
		return fmt.Errorf("Choose between 1 and %d.", len(servers))
	}
	return c.start(servers[n-1])
}
