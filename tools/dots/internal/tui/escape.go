package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// terminalText removes terminal commands, including OSC/DCS payloads and
// unfinished sequences. Only application-owned SGR may survive in styled bodies.
func terminalText(value string, styled bool) string {
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); {
		start := i
		r, n := utf8.DecodeRuneInString(value[i:])
		i += n
		code := r
		if r == '\x1b' {
			if i == len(value) {
				break
			}
			r, n = utf8.DecodeRuneInString(value[i:])
			i += n
			switch r {
			case '[':
				code = '\u009b'
			case ']':
				code = '\u009d'
			case 'P', 'X', '^', '_':
				code = '\u0090'
			default:
				for r >= 0x20 && r <= 0x2f && i < len(value) {
					r, n = utf8.DecodeRuneInString(value[i:])
					i += n
				}
				continue
			}
		}
		switch code {
		case '\u009b':
			valid := true
			for i < len(value) {
				c := value[i]
				i++
				if c >= 0x40 && c <= 0x7e {
					if styled && valid && c == 'm' && value[start] == '\x1b' {
						out.WriteString(value[start:i])
					}
					break
				}
				if !(c >= '0' && c <= '9' || c == ';' || c == ':') {
					valid = false
				}
			}
		case '\u0090', '\u0098', '\u009d', '\u009e', '\u009f':
			for i < len(value) {
				c, n := utf8.DecodeRuneInString(value[i:])
				i += n
				if c == '\a' || c == '\u009c' {
					break
				}
				if c == '\x1b' && i < len(value) && value[i] == '\\' {
					i++
					break
				}
			}
		default:
			if !unicode.IsControl(r) || r == '\n' || r == '\t' {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}
