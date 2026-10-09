package codexconfig

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

const whitespace = `[\t\n\v\f\r\x{001c}-\x{001f}\x{0085}\p{Z}]`

var likelySecretRE = regexp.MustCompile(`(?:sk-[A-Za-z0-9_-]{12,}|gh[pousr]_[A-Za-z0-9]{20,}|Bearer` + whitespace + `+[A-Za-z0-9._~-]{12,})`)
var camelRE = regexp.MustCompile(`([a-z0-9])([A-Z])`)
var namePartsRE = regexp.MustCompile(`[-_.]+`)
var assignmentRE = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_.-]*)` + whitespace + `*[:=]` + whitespace + `*(.+?)` + whitespace + `*$`)
var sensitiveParts = []string{"auth", "authorization", "bearer", "cookie", "credential", "credentials", "key", "passwd", "password", "secret", "token"}
var queryNames = []string{"assertion", "code", "jwt", "oauth_code", "samlresponse", "session", "sig", "signature", "state", "ticket", "x-amz-signature", "x-goog-signature"}

func fullPlaceholder(s string) bool { return placeholderRE.FindString(s) == s && s != "" }
func trimWhitespace(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f) })
}
func sensitiveName(s string) bool {
	normalized := strings.ToLower(camelRE.ReplaceAllString(s, "${1}-${2}"))
	parts := namePartsRE.Split(normalized, -1)
	for _, p := range parts {
		if contains(sensitiveParts, p) {
			return true
		}
	}
	compact := strings.Join(parts, "")
	for _, marker := range []string{"apikey", "accesstoken", "authtoken", "bearertoken", "privatekey", "sessioncookie"} {
		if strings.Contains(compact, marker) {
			return true
		}
	}
	return false
}
func sensitiveField(p []string) bool {
	if len(p) == 0 {
		return false
	}
	fields := p
	if p[0] == "mcp_servers" || p[0] == "model_providers" {
		fields = p[min(2, len(p)):]
	} else if contains(managedPrefixes, p[0]) {
		fields = p[1:]
	}
	for _, f := range fields {
		if !strings.HasPrefix(f, "[") && f != "requires_openai_auth" && sensitiveName(f) {
			return true
		}
	}
	return false
}

var schemeRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

func percentDecode(s string, query bool) string {
	if query {
		s = strings.ReplaceAll(s, "+", " ")
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '%' && i+2 < len(s) {
			v, e := url.PathUnescape(s[i : i+3])
			if e == nil {
				b.WriteString(v)
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return strings.ToValidUTF8(b.String(), "\ufffd")
}
func urlCredentials(value string) bool {
	// urlsplit accepts malformed percent escapes; inspect them without losing
	// the rest of the URL, and decode parameter names before classifying them.
	value = strings.TrimLeftFunc(value, func(r rune) bool { return r <= 32 })
	value = strings.NewReplacer("\n", "", "\r", "", "\t", "").Replace(value)
	scheme := schemeRE.FindString(value)
	rest := strings.TrimPrefix(value, scheme)
	if !strings.HasPrefix(rest, "//") {
		return false
	}
	rest = rest[2:]
	end := strings.IndexAny(rest, "/?#")
	if end < 0 {
		end = len(rest)
	}
	host := rest[:end]
	rest = rest[end:]
	if host == "" {
		return false
	}
	// These Unicode compatibility characters normalize to URL delimiters.
	// Python's urlsplit rejects them in the authority before inspecting fields.
	// The fixture oracle covers every non-ASCII Unicode code point.
	if strings.ContainsAny(host, "\u2047\u2048\u2049\u2100\u2101\u2105\u2106\u2a74\ufe13\ufe16\ufe55\ufe56\ufe5f\ufe6b\uff03\uff0f\uff1a\uff1f\uff20") {
		return true
	}
	if strings.Contains(host, "[") || strings.Contains(host, "]") {
		start := strings.IndexByte(host, '[')
		end := strings.IndexByte(host, ']')
		if start < 0 || end < start {
			return true
		}
		ip := host[start+1 : end]
		base, _, _ := strings.Cut(ip, "%")
		if (net.ParseIP(base) == nil || !strings.Contains(base, ":")) && !regexp.MustCompile(`^v[0-9A-Fa-f]+\..+$`).MatchString(ip) {
			return true
		}
	}
	if scheme == "" {
		return false
	}
	if strings.Contains(host, "@") {
		return true
	}
	pathAndQuery, fragment, _ := strings.Cut(rest, "#")
	path, query, _ := strings.Cut(pathAndQuery, "?")
	for _, pairs := range []string{query, fragment} {
		for _, pair := range strings.Split(pairs, "&") {
			name, val, _ := strings.Cut(pair, "=")
			name = percentDecode(name, true)
			val = percentDecode(val, true)
			if (sensitiveName(name) || contains(queryNames, strings.ToLower(name))) && !fullPlaceholder(val) {
				return true
			}
		}
	}
	var segments []string
	for _, segment := range strings.Split(path, "/") {
		if segment != "" {
			segments = append(segments, percentDecode(segment, false))
		}
	}
	for i, s := range segments {
		if fullPlaceholder(s) {
			continue
		}
		sensitive := sensitiveName(s)
		for _, p := range namePartsRE.Split(strings.ToLower(s), -1) {
			if contains([]string{"sig", "signature", "signing"}, p) {
				sensitive = true
			}
		}
		if sensitive {
			next := ""
			if i+1 < len(segments) {
				next = segments[i+1]
			}
			if !fullPlaceholder(next) {
				return true
			}
		}
	}
	return false
}
func placeholderAssignment(v string) bool {
	v = trimWhitespace(v)
	if fullPlaceholder(v) {
		return true
	}
	m := assignmentRE.FindStringSubmatch(v)
	return m != nil && fullPlaceholder(trimWhitespace(m[2]))
}
func leakGate(text, home string) error {
	parsed, err := validate(text, "sanitized Codex snapshot")
	if err != nil {
		return err
	}
	var roots []string
	for _, k := range []string{"projects", "hooks", "plugins", "marketplaces", "features", "notice"} {
		if _, ok := parsed[k]; ok {
			roots = append(roots, k)
		}
	}
	if len(roots) > 0 {
		return fmt.Errorf("runtime-owned tables leaked into snapshot: %s", strings.Join(sorted(roots), ", "))
	}
	if tui, ok := parsed["tui"].(map[string]any); ok {
		var unexpected []string
		for k := range tui {
			if k != "status_line" {
				unexpected = append(unexpected, k)
			}
		}
		if len(unexpected) > 0 {
			return fmt.Errorf("runtime/unmanaged TUI keys leaked into snapshot: %s", strings.Join(sorted(unexpected), ", "))
		}
	}
	problems := map[string]bool{}
	problem := func(message string, p []string) { problems[message+" at "+strings.Join(p, ".")] = true }
	var walk func(any, []string)
	walk = func(v any, p []string) {
		switch x := v.(type) {
		case map[string]any:
			for k, c := range x {
				walk(c, appendPath(p, k))
			}
			return
		case []any:
			for i, c := range x {
				walk(c, appendPath(p, fmt.Sprintf("[%d]", i)))
			}
			return
		}
		s, ok := v.(string)
		if !ok {
			if len(p) > 0 {
				if !envReference(p) && sensitiveField(p) {
					problem("non-string secret-like key", p)
				}
				if contains(p, "http_headers") {
					problem("non-string static HTTP header", p)
				}
			}
			return
		}
		if home != "" && strings.Contains(s, home) {
			problem("absolute home path", p)
		}
		if likelySecretRE.MatchString(s) {
			problem("secret-looking value", p)
		}
		if len(p) == 0 {
			return
		}
		expected := exactSecret(p)
		switch {
		case expected != "" && s != "${"+expected+"}":
			problem("unmasked configured secret", p)
		case len(p) >= 4 && p[0] == "mcp_servers" && p[2] == "env" && !safeEnv(p) && !fullPlaceholder(s):
			problem("unmasked MCP environment value", p)
		case len(p) == 3 && p[0] == "shell_environment_policy" && p[1] == "set" && !fullPlaceholder(s):
			problem("unmasked shell environment value", p)
		case !envReference(p) && sensitiveField(p) && !fullPlaceholder(s):
			problem("unmasked secret-like key", p)
		case contains(p, "http_headers") && !fullPlaceholder(s):
			problem("static HTTP header value", p)
		case (p[len(p)-1] == "url" || p[len(p)-1] == "base_url") && urlCredentials(s):
			problem("credential-bearing URL", p)
		}
	}
	walk(parsed, nil)
	if servers, ok := parsed["mcp_servers"].(map[string]any); ok {
		for server, v := range servers {
			config, ok := v.(map[string]any)
			if !ok {
				continue
			}
			args, ok := config["args"].([]any)
			if !ok {
				continue
			}
			for i, v := range args {
				arg, ok := v.(string)
				if !ok {
					continue
				}
				name := ""
				var value any = ""
				next := func() any {
					if i+1 < len(args) {
						return args[i+1]
					}
					return ""
				}
				switch {
				case strings.HasPrefix(arg, "--"):
					var inline bool
					var s string
					name, s, inline = strings.Cut(arg[2:], "=")
					if inline {
						value = s
					} else {
						value = next()
					}
				case strings.HasPrefix(arg, "-H"):
					name = "header"
					if len(arg) > 2 {
						value = arg[2:]
					} else {
						value = next()
					}
				case strings.HasPrefix(arg, "-e"):
					name = "env"
					if len(arg) > 2 {
						value = arg[2:]
					} else {
						value = next()
					}
				case strings.HasPrefix(arg, "-"):
					name = strings.TrimLeft(arg, "-")
					value = next()
				}
				location := fmt.Sprintf("mcp_servers.%s.args[%d]", server, i)
				add := func(message string) { problems[message+" at "+location] = true }
				if urlCredentials(arg) {
					add("credential-bearing URL in MCP arguments")
				}
				s, isString := value.(string)
				if isString && urlCredentials(s) {
					add("credential-bearing MCP option value")
				}
				m := assignmentRE.FindStringSubmatch(arg)
				if m != nil && sensitiveName(m[1]) && !fullPlaceholder(trimWhitespace(m[2])) {
					add("secret-bearing assignment in MCP arguments")
				}
				normalized := strings.ReplaceAll(strings.ToLower(name), "_", "-")
				if contains([]string{"env", "environment", "header", "headers", "http-header"}, normalized) && (!isString || !placeholderAssignment(s)) {
					add("static header/environment MCP argument")
				}
				if name != "" && sensitiveName(name) && (!isString || !fullPlaceholder(s)) {
					add("secret-bearing MCP argument")
				}
			}
		}
	}
	if likelySecretRE.MatchString(text) {
		problems["secret-looking token present in generated snapshot text"] = true
	}
	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(setKeys(problems), "; "))
	}
	return nil
}
