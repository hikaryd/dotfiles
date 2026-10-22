// Package logstream bounds display records without stopping pipe drainage.
// Raw non-interactive output deliberately bypasses this presentation boundary.
package logstream

import (
	"bufio"
	"dots/native/internal/tui"
	"errors"
	"io"
	"strings"
	"unicode/utf8"
)

const MaxLineBytes = 8192
const truncated = " … [truncated]"

// Text removes terminal commands before application-owned styling is added.
func Text(s string) string {
	if len(s) > MaxLineBytes {
		s = s[:MaxLineBytes-len(truncated)]
		for !utf8.ValidString(s) && len(s) > 0 {
			s = s[:len(s)-1]
		}
		s += truncated
	}
	return strings.ReplaceAll(tui.Plain(s), "\t", " ")
}

// Scan drains every record, retaining only a bounded prefix. Unlike Scanner,
// a long line cannot leave a child blocked writing to an unread stdout pipe.
func Scan(input io.Reader, emit func(string)) error {
	reader := bufio.NewReaderSize(input, 16<<10)
	prefix := make([]byte, 0, MaxLineBytes)
	clipped := false
	for {
		part, err := reader.ReadSlice('\n')
		if len(part) > 0 && part[len(part)-1] == '\n' {
			part = part[:len(part)-1]
		}
		take := min(len(part), MaxLineBytes-len(prefix))
		prefix = append(prefix, part[:take]...)
		clipped = clipped || take < len(part)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if len(prefix) > 0 || err == nil {
			line := string(prefix)
			if clipped {
				line = line[:min(len(line), MaxLineBytes-len(truncated))] + truncated
			}
			emit(Text(line))
		}
		prefix = prefix[:0]
		clipped = false
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
