package desktop

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"dots/native/internal/tui"
)

var clipModes = []string{"json", "compact", "base64", "jwt", "time", "redact"}
var invalidClip = errors.New("Некорректные данные для выбранного преобразования; clipboard не изменён.")

func checkedText(s string) error {
	if !utf8.ValidString(s) {
		return errors.New("Ожидался корректный UTF-8.")
	}
	if len(s) > maxBytes {
		return errors.New("Лимит 1 MiB превышен.")
	}
	for _, r := range s {
		if (unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Cs, r)) && !strings.ContainsRune("\n\r\t", r) {
			return errors.New("Управляющие символы или некорректный Unicode: операция отменена.")
		}
	}
	return nil
}

type jsonPair struct {
	k string
	v any
}
type jsonObject []jsonPair

// Keep object order, reject duplicate keys and escaped lone surrogates, unlike
// encoding/json's default permissive object/string decoder.
func strictJSON(s string) (any, error) {
	if e := checkedText(s); e != nil {
		return nil, e
	}
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			continue
		}
		i++
		if i >= len(s) || s[i] != 'u' {
			continue
		}
		if i+4 >= len(s) {
			return nil, invalidClip
		}
		n, e := strconv.ParseUint(s[i+1:i+5], 16, 16)
		if e != nil {
			return nil, invalidClip
		}
		i += 4
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(s) || s[i+1:i+3] != "\\u" {
				return nil, invalidClip
			}
			low, e := strconv.ParseUint(s[i+3:i+7], 16, 16)
			if e != nil || low < 0xdc00 || low > 0xdfff {
				return nil, invalidClip
			}
			i += 6
		} else if n >= 0xdc00 && n <= 0xdfff {
			return nil, invalidClip
		}
	}
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	v, e := readJSONValue(d, 0)
	if e != nil {
		return nil, e
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, invalidClip
	}
	return v, nil
}
func readJSONValue(d *json.Decoder, depth int) (any, error) {
	if depth > 512 {
		return nil, invalidClip
	}
	t, e := d.Token()
	if e != nil {
		return nil, invalidClip
	}
	switch x := t.(type) {
	case json.Delim:
		switch x {
		case '{':
			o := jsonObject{}
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return nil, invalidClip
				}
				key, ok := k.(string)
				if !ok || seen[key] || checkedText(key) != nil {
					return nil, invalidClip
				}
				seen[key] = true
				v, e := readJSONValue(d, depth+1)
				if e != nil {
					return nil, e
				}
				o = append(o, jsonPair{key, v})
			}
			_, e = d.Token()
			return o, e
		case '[':
			a := []any{}
			for d.More() {
				v, e := readJSONValue(d, depth+1)
				if e != nil {
					return nil, e
				}
				a = append(a, v)
			}
			_, e = d.Token()
			return a, e
		}
		return nil, invalidClip
	case string:
		if checkedText(x) != nil {
			return nil, invalidClip
		}
	case json.Number:
		if strings.ContainsAny(string(x), ".eE") {
			n, e := x.Float64()
			if e != nil || math.IsInf(n, 0) {
				return nil, invalidClip
			}
			format := byte('e')
			if n == 0 || math.Abs(n) >= 1e-4 && math.Abs(n) < 1e16 {
				format = 'f'
			}
			normalized := strconv.FormatFloat(n, format, -1, 64)
			if !strings.ContainsAny(normalized, ".e") {
				normalized += ".0"
			}
			return json.Number(normalized), nil
		}
		integer, ok := new(big.Int).SetString(string(x), 10)
		if !ok {
			return nil, invalidClip
		}
		return json.Number(integer.String()), nil
	}
	return t, nil
}
func (o jsonObject) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, p := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := jsonBytes(p.k, false)
		v, e := jsonBytes(p.v, false)
		if e != nil {
			return nil, e
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
func redactValue(v any, keys []string) any {
	switch x := v.(type) {
	case jsonObject:
		for i, p := range x {
			redact := false
			for _, k := range keys {
				if foldKey(k) == foldKey(p.k) {
					redact = true
					break
				}
			}
			if redact {
				x[i].v = "[REDACTED]"
			} else {
				x[i].v = redactValue(p.v, keys)
			}
		}
	case []any:
		for i, p := range x {
			x[i] = redactValue(p, keys)
		}
	}
	return v
}
func jwtSegment(s string) (string, error) {
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(s) {
		return "", invalidClip
	}
	b, e := base64.RawURLEncoding.Strict().DecodeString(s)
	if e != nil || base64.RawURLEncoding.EncodeToString(b) != s {
		return "", invalidClip
	}
	return string(b), checkedText(string(b))
}
func transform(mode, source string, keys []string) (string, error) {
	if e := checkedText(source); e != nil {
		return "", e
	}
	var v any
	var e error
	switch mode {
	case "base64":
		s := strings.TrimSpace(source)
		b, err := base64.StdEncoding.Strict().DecodeString(s)
		if err != nil || base64.StdEncoding.EncodeToString(b) != s {
			return "", invalidClip
		}
		return string(b), checkedText(string(b))
	case "json", "compact", "redact":
		v, e = strictJSON(source)
		if e != nil {
			return "", invalidClip
		}
		if mode == "redact" {
			if len(keys) == 0 {
				keys = []string{"token", "secret", "password", "authorization", "api_key"}
			}
			v = redactValue(v, keys)
		}
	case "jwt":
		parts := strings.Split(strings.TrimSpace(source), ".")
		if len(parts) != 3 {
			return "", invalidClip
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(parts[2]) {
			return "", invalidClip
		}
		sig, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
		if err != nil || base64.RawURLEncoding.EncodeToString(sig) != parts[2] {
			return "", invalidClip
		}
		h, err := jwtSegment(parts[0])
		if err != nil {
			return "", invalidClip
		}
		p, err := jwtSegment(parts[1])
		if err != nil {
			return "", invalidClip
		}
		hv, err := strictJSON(h)
		if err != nil {
			return "", invalidClip
		}
		pv, err := strictJSON(p)
		if err != nil {
			return "", invalidClip
		}
		ho, ok := hv.(jsonObject)
		if !ok {
			return "", invalidClip
		}
		if _, ok = pv.(jsonObject); !ok {
			return "", invalidClip
		}
		algOK := false
		for _, x := range ho {
			if x.k == "alg" {
				a, ok := x.v.(string)
				algOK = ok && !strings.EqualFold(a, "none")
			}
		}
		if !algOK {
			return "", invalidClip
		}
		v = jsonObject{{"verification", "UNVERIFIED"}, {"header", hv}, {"payload", pv}}
	case "time":
		s := strings.TrimSpace(source)
		var moment time.Time
		var sec float64
		if regexp.MustCompile(`^-?\d+(?:\.\d{1,6})?$`).MatchString(s) {
			sec, e = strconv.ParseFloat(s, 64)
			if e != nil || math.IsInf(sec, 0) {
				return "", invalidClip
			}
			if math.Abs(sec) >= 100000000000 {
				sec /= 1000
			}
			whole, frac := math.Modf(sec)
			moment = time.Unix(int64(whole), int64(math.RoundToEven(frac*1e6))*1000).UTC()
		} else {
			if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,6})?(?:Z|[+-](?:[01]\d|2[0-3]):[0-5]\d)$`).MatchString(s) {
				return "", invalidClip
			}
			moment, e = time.Parse(time.RFC3339Nano, s)
			if e != nil {
				return "", invalidClip
			}
			moment = moment.UTC()
			sec = float64(moment.Unix()) + float64(moment.Nanosecond())/1e9
		}
		if moment.Year() < 1 || moment.Year() > 9999 {
			return "", invalidClip
		}
		iso := moment.Format("2006-01-02T15:04:05")
		if moment.Nanosecond() != 0 {
			iso += fmt.Sprintf(".%06d", moment.Nanosecond()/1000)
		}
		iso += "Z"
		seconds := strconv.FormatFloat(sec, 'f', -1, 64)
		if !strings.Contains(seconds, ".") {
			seconds += ".0"
		}
		v = jsonObject{{"iso8601", iso}, {"unix_seconds", json.Number(seconds)}, {"unix_milliseconds", int64(math.RoundToEven(sec * 1000))}}
	default:
		return "", invalidClip
	}
	b, e := jsonBytes(v, mode != "compact")
	if e != nil {
		return "", invalidClip
	}
	return string(b), checkedText(string(b))
}
func clipboardRead() ([]byte, error) {
	r, e := captureCommand(5*time.Second, maxBytes, "", nil, "pbpaste")
	if e != nil {
		return nil, e
	}
	if r.code != 0 {
		return nil, errors.New("Не удалось прочитать clipboard.")
	}
	return []byte(r.out), nil
}
func changeCount() (string, error) {
	r, e := captureCommand(5*time.Second, 100, "", nil, "osascript", "-l", "JavaScript", "-e", "ObjC.import('AppKit'); $.NSPasteboard.generalPasteboard.changeCount")
	s := strings.TrimSpace(r.out)
	if e != nil || r.code != 0 || !regexp.MustCompile(`^[0-9]{1,20}$`).MatchString(s) {
		return "", errors.New("Недоступен changeCount clipboard: запись отменена.")
	}
	return s, nil
}
func clipboardSnapshot(expected string) ([]byte, string, error) {
	n, e := changeCount()
	if e != nil {
		return nil, "", e
	}
	if expected != "" && expected != n {
		return nil, "", errors.New("Clipboard изменился после чтения: запись отменена.")
	}
	b, e := clipboardRead()
	if e != nil {
		return nil, "", e
	}
	after, e := changeCount()
	if e != nil {
		return nil, "", e
	}
	if after != n {
		return nil, "", errors.New("Clipboard изменился во время чтения: запись отменена.")
	}
	return b, n, nil
}
func preview(s string) string {
	r := []rune(s)
	suffix := ""
	if len(r) > 2000 {
		r = r[:2000]
		suffix = " … [preview truncated]"
	}
	q := strconv.QuoteToASCII(string(r))
	return q[1:len(q)-1] + suffix
}
func confirmClipboard(result string) bool {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	frame := desktopFrame("Перезаписать clipboard?")
	frame.Mode, frame.Tone = "ACTION", tui.Yellow
	frame.Intro = preview(result) + "\n\nЗапись только после явного yes; clipboard проверяется повторно."
	return tui.Confirm(ctx, frame) == nil
}
func clipRun(args []string) int {
	o, e := parse(args, "--stdout --write --stdin", "--keys")
	if e != nil || len(o.pos) > 1 || o.flag("--stdout") && o.flag("--write") {
		return usageError("clip")
	}
	mode := ""
	if len(o.pos) > 0 {
		mode = o.pos[0]
		if !has(mode, clipModes...) {
			return usageError("clip")
		}
	}
	var keys []string
	if k, ok := o.values["--keys"]; ok {
		for _, v := range strings.Split(k, ",") {
			v = strings.TrimSpace(v)
			if mode != "redact" || v == "" {
				return fail("dots clip", errors.New("--keys допустим только для redact и непустых имён."))
			}
			keys = append(keys, v)
		}
	}
	if mode == "" {
		i, e := selectLine(clipModes, "Clipboard transform")
		if e != nil {
			return fail("dots clip", e)
		}
		if i < 0 {
			return 130
		}
		mode = clipModes[i]
	}
	var original, source []byte
	generation := ""
	if o.flag("--stdout") {
		if !o.flag("--stdin") {
			original, e = clipboardRead()
		}
	} else {
		original, generation, e = clipboardSnapshot("")
	}
	if e != nil {
		return fail("dots clip", e)
	}
	source = original
	if o.flag("--stdin") {
		source, e = readLimited(os.Stdin, maxBytes)
	}
	if e != nil {
		return fail("dots clip", e)
	}
	result, e := transform(mode, string(source), keys)
	if e != nil {
		return fail("dots clip", e)
	}
	if o.flag("--stdout") {
		fmt.Println(result)
		return 0
	}
	if mode == "jwt" {
		fmt.Fprintln(os.Stderr, "JWT UNVERIFIED: подпись и доверие НЕ проверены.")
	}
	if mode == "redact" {
		fmt.Fprintln(os.Stderr, "Redact: только указанные ключи; это не гарантия удаления всех секретов.")
	}
	if !o.flag("--write") {
		if !confirmClipboard(result) {
			return 130
		}
	}
	current, _, e := clipboardSnapshot(generation)
	if e != nil {
		return fail("dots clip", e)
	}
	if !bytes.Equal(current, original) {
		return fail("dots clip", errors.New("Clipboard изменился после чтения: запись отменена."))
	}
	r, e := captureCommand(5*time.Second, 1024, result, nil, "pbcopy")
	if e != nil {
		return fail("dots clip", e)
	}
	if r.code != 0 {
		return fail("dots clip", errors.New("pbcopy сообщил об ошибке; состояние clipboard неизвестно."))
	}
	fmt.Fprintln(os.Stderr, "Clipboard обновлён.")
	return 0
}
