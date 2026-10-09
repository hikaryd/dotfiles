package vpn

import (
	"errors"
	"fmt"
	"io"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

func parseNode(line string, idx int) (object, error) {
	scheme, rest, ok := strings.Cut(line, "://")
	if !ok {
		return nil, errors.New("unsupported protocol (supported: vless, hysteria2, hy2)")
	}
	switch strings.ToLower(scheme) {
	case "vless":
		return parseVless(rest, idx)
	case "hy2", "hysteria2":
		return parseHy2(rest, idx)
	}
	return nil, errors.New("unsupported protocol (supported: vless, hysteria2, hy2)")
}
func nameTag(frag string, idx int) string {
	name := fmt.Sprintf("node%d", idx)
	if frag != "" {
		name = strings.ToValidUTF8(uriUnquote(frag), "\uFFFD")
		name = strings.TrimSpace(strings.SplitN(name, " - ", 2)[0])
	}
	return fmt.Sprintf("%s #%d", name, idx)
}

// URI components are decoded once. Like urllib, an unmatched percent sign is
// literal; only Hysteria authentication requires strictly valid UTF-8.
func uriUnquote(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v, e := strconv.ParseUint(s[i+1:i+3], 16, 8); e == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
func queryPairs(query string, keepBlank bool) [][2]string {
	pairs := [][2]string{}
	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		if v == "" && !keepBlank {
			continue
		}
		decode := func(s string) string {
			return strings.ToValidUTF8(uriUnquote(strings.ReplaceAll(s, "+", " ")), "\uFFFD")
		}
		pairs = append(pairs, [2]string{decode(k), decode(v)})
	}
	return pairs
}
func splitURI(rest string, keepBlank bool) (auth, host, port, path, frag string, q map[string]string, err error) {
	bad := errors.New("malformed node URI")
	rest, frag, _ = strings.Cut(rest, "#")
	rest, query, _ := strings.Cut(rest, "?")
	q = map[string]string{}
	for _, p := range queryPairs(query, keepBlank) {
		q[p[0]] = p[1]
	}
	i := strings.LastIndex(rest, "@")
	if i < 1 {
		err = bad
		return
	}
	auth = rest[:i]
	address := rest[i+1:]
	address, path, _ = strings.Cut(address, "/")
	if path != "" {
		path = "/" + path
	}
	if strings.HasPrefix(address, "[") {
		i = strings.Index(address, "]")
		if i < 0 {
			err = bad
			return
		}
		host = address[1:i]
		if net.ParseIP(host) == nil {
			err = bad
			return
		}
		tail := address[i+1:]
		if tail != "" && !strings.HasPrefix(tail, ":") {
			err = bad
			return
		}
		port = strings.TrimPrefix(tail, ":")
	} else {
		host, port, _ = strings.Cut(address, ":")
		if strings.Contains(port, ":") {
			err = bad
			return
		}
	}
	if host == "" {
		err = bad
	}
	return
}
func parseVless(rest string, idx int) (object, error) {
	auth, host, port, _, frag, q, e := splitURI(rest, false)
	if e != nil {
		return nil, e
	}
	p, e := strconv.Atoi(port)
	if e != nil || p < 1 || p > 65535 {
		return nil, errors.New("missing or invalid VLESS endpoint/authentication")
	}
	typ := value(q, "type", "tcp")
	if typ == "xhttp" {
		return nil, errors.New("xhttp transport is not supported by sing-box")
	}
	if typ != "tcp" && typ != "grpc" && typ != "http" {
		return nil, errors.New("unsupported VLESS transport")
	}
	fp := value(q, "fp", "chrome")
	if !strings.Contains("|chrome|firefox|edge|safari|360|qq|ios|android|random|randomized|", "|"+fp+"|") {
		fp = "chrome"
	}
	tls := object{"enabled": true, "server_name": q["sni"], "utls": object{"enabled": true, "fingerprint": fp}, "reality": object{"enabled": true, "public_key": q["pbk"], "short_id": q["sid"]}}
	n := object{"type": "vless", "tag": nameTag(frag, idx), "server": host, "server_port": p, "uuid": auth, "tls": tls}
	if q["flow"] != "" {
		n["flow"] = q["flow"]
	}
	if q["insecure"] == "1" || q["insecure"] == "true" {
		tls["insecure"] = true
	}
	if typ == "grpc" {
		n["transport"] = object{"type": "grpc", "service_name": q["serviceName"]}
	}
	if typ == "http" {
		n["transport"] = object{"type": "http", "host": []string{value(q, "host", q["sni"])}}
	}
	return n, nil
}
func parseHy2(rest string, idx int) (object, error) {
	auth, host, port, path, frag, q, e := splitURI(rest, true)
	if e != nil {
		return nil, errors.New("malformed Hysteria2 URI")
	}
	password := uriUnquote(auth)
	if password == "" || !utf8.ValidString(password) {
		return nil, errors.New("missing Hysteria2 authentication")
	}
	if path != "" {
		return nil, errors.New("unsupported Hysteria2 URI path")
	}
	for _, k := range []string{"pinSHA256", "ech", "mport"} {
		if _, ok := q[k]; ok {
			return nil, errors.New("unsupported Hysteria2 URI parameter")
		}
	}
	insecure := strings.ToLower(value(q, "insecure", "0"))
	if insecure != "0" && insecure != "1" && insecure != "true" && insecure != "false" {
		return nil, errors.New("invalid Hysteria2 insecure flag")
	}
	if port == "" {
		port = "443"
	}
	ports := []string{}
	for _, part := range strings.Split(port, ",") {
		a, b, has := strings.Cut(part, "-")
		s, e := strconv.Atoi(a)
		end, ee := strconv.Atoi(b)
		if !has {
			end = s
			ee = nil
		}
		if !regexp.MustCompile(`^[0-9]+(?:-[0-9]+)?$`).MatchString(part) || e != nil || ee != nil || s < 1 || end < s || end > 65535 {
			return nil, errors.New("invalid Hysteria2 server port")
		}
		ports = append(ports, strings.ReplaceAll(part, "-", ":"))
	}
	tls := object{"enabled": true, "server_name": q["sni"]}
	n := object{"type": "hysteria2", "tag": nameTag(frag, idx), "server": strings.ToLower(host), "password": password, "tls": tls}
	if len(ports) == 1 && !strings.Contains(ports[0], ":") {
		p, _ := strconv.Atoi(ports[0])
		n["server_port"] = p
	} else {
		n["server_ports"] = ports
	}
	if insecure == "1" || insecure == "true" {
		tls["insecure"] = true
	}
	if q["obfs"] != "" {
		if q["obfs"] != "salamander" {
			return nil, errors.New("unsupported Hysteria2 obfuscation")
		}
		if q["obfs-password"] == "" {
			return nil, errors.New("missing Hysteria2 obfuscation password")
		}
		n["obfs"] = object{"type": "salamander", "password": q["obfs-password"]}
	} else if q["obfs-password"] != "" {
		return nil, errors.New("missing Hysteria2 obfuscation type")
	}
	return n, nil
}
func parseSubscription(source string, w io.Writer) []object {
	nodes := []object{}
	for i, line := range strings.Split(source, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		n, e := parseNode(line, i)
		if e != nil {
			fmt.Fprintf(w, "skip line %d: %s\n", i, e)
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes
}
func autoPool(nodes []object, include, exclude string, limit int, prefer string) ([]string, error) {
	if limit < 1 {
		return nil, errors.New("AUTO_MAX must be positive")
	}
	regex := make([]*regexp.Regexp, 3)
	for i, s := range []string{include, exclude, prefer} {
		if s != "" {
			r, e := regexp.Compile("(?i)" + s)
			if e != nil {
				return nil, errors.New("Invalid AUTO_INCLUDE/AUTO_EXCLUDE/AUTO_PREFER regex")
			}
			regex[i] = r
		}
	}
	candidates := []object{}
	for _, n := range nodes {
		tag := str(n["tag"])
		if regex[0] != nil && !regex[0].MatchString(tag) || regex[1] != nil && regex[1].MatchString(tag) {
			continue
		}
		candidates = append(candidates, n)
	}
	if regex[2] != nil {
		sort.SliceStable(candidates, func(i, j int) bool {
			return regex[2].MatchString(str(candidates[i]["tag"])) && !regex[2].MatchString(str(candidates[j]["tag"]))
		})
	}
	tags := []string{}
	seen := map[string]bool{}
	for _, n := range candidates {
		identity := object{}
		for k, v := range n {
			if k != "tag" {
				identity[k] = v
			}
		}
		key := string(jsonBytes(identity))
		if seen[key] {
			continue
		}
		seen[key] = true
		tags = append(tags, str(n["tag"]))
		if len(tags) == limit {
			break
		}
	}
	if len(tags) == 0 {
		return nil, errors.New("Auto pool is empty; keeping old config. Check AUTO_INCLUDE/AUTO_EXCLUDE")
	}
	return tags, nil
}
