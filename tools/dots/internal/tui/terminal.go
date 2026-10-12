package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"
)

var ErrNoTerminal = errors.New("interactive terminal required")
var ErrCanceled = errors.New("selection canceled")
var ErrAction = errors.New("additional screen action")

type Key struct{ Name, Text string }

// Keys decodes arrows, a standalone Esc, Unicode and bracketed paste. Consumers
// never interpret pasted text as terminal commands or keyboard shortcuts.
func Keys(ctx context.Context, input *os.File) <-chan Key {
	output := make(chan Key, 32)
	runes := make(chan rune, 64)
	go func() {
		defer close(runes)
		pending := []byte{}
		buffer := make([]byte, 4096)
		for ctx.Err() == nil {
			n, err := input.Read(buffer)
			// VMIN=0/VTIME=1 returns a zero-byte tty read on timeout; os.File
			// reports it as EOF. It is not the end of this terminal session.
			if err == io.EOF {
				continue
			}
			if err != nil {
				return
			}
			if n == 0 {
				continue
			}
			pending = append(pending, buffer[:n]...)
			for len(pending) > 0 && utf8.FullRune(pending) {
				r, size := utf8.DecodeRune(pending)
				pending = pending[size:]
				select {
				case runes <- r:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	go func() {
		defer close(output)
		send := func(key Key) bool {
			select {
			case output <- key:
				return true
			case <-ctx.Done():
				return false
			}
		}
		next := func(timeout time.Duration) (rune, bool) {
			timer := time.NewTimer(timeout)
			defer timer.Stop()
			select {
			case r, ok := <-runes:
				return r, ok
			case <-timer.C:
				return 0, false
			case <-ctx.Done():
				return 0, false
			}
		}
		for {
			select {
			case <-ctx.Done():
				return
			case r, ok := <-runes:
				if !ok {
					return
				}
				if r == 27 {
					second, ok := next(35 * time.Millisecond)
					if !ok {
						if !send(Key{Name: "esc"}) {
							return
						}
						continue
					}
					if second != '[' && second != 'O' {
						if !send(Key{Name: "esc"}) {
							return
						}
						if unicode.IsPrint(second) {
							send(Key{Name: "text", Text: string(second)})
						}
						continue
					}
					var sequence strings.Builder
					for {
						last, ok := next(35 * time.Millisecond)
						if !ok {
							break
						}
						sequence.WriteRune(last)
						if last >= '@' && last <= '~' {
							break
						}
						if sequence.Len() > 32 {
							break
						}
					}
					switch sequence.String() {
					case "A":
						send(Key{Name: "up"})
					case "B":
						send(Key{Name: "down"})
					case "C":
						send(Key{Name: "right"})
					case "D":
						send(Key{Name: "left"})
					case "H":
						send(Key{Name: "home"})
					case "F":
						send(Key{Name: "end"})
					case "5~":
						send(Key{Name: "page-up"})
					case "6~":
						send(Key{Name: "page-down"})
					case "200~":
						var paste strings.Builder
						for {
							select {
							case <-ctx.Done():
								return
							case p, ok := <-runes:
								if !ok {
									return
								}
								paste.WriteRune(p)
								if strings.HasSuffix(paste.String(), "\x1b[201~") {
									text := strings.TrimSuffix(paste.String(), "\x1b[201~")
									if !send(Key{Name: "paste", Text: text}) {
										return
									}
									goto pasted
								}
								if paste.Len() > 16*1024*1024 {
									return
								}
							}
						}
					pasted:
					}
					continue
				}
				name := "text"
				switch r {
				case 1:
					name = "ctrl-a"
				case 3:
					name = "ctrl-c"
				case 4:
					name = "ctrl-d"
				case 5:
					name = "ctrl-e"
				case 8, 127:
					name = "backspace"
				case '\t':
					name = "tab"
				case '\r', '\n':
					name = "enter"
				case 12:
					name = "ctrl-l"
				case 15:
					name = "ctrl-o"
				}
				if name != "text" || unicode.IsPrint(r) {
					if !send(Key{Name: name, Text: string(r)}) {
						return
					}
				}
			}
		}
	}()
	return output
}
func Raw(input *os.File) (func(), error) {
	if !term.IsTerminal(int(input.Fd())) {
		return nil, ErrNoTerminal
	}
	state, err := term.MakeRaw(int(input.Fd()))
	if err != nil {
		return nil, err
	}
	if err := setReadTimeout(input); err != nil {
		_ = term.Restore(int(input.Fd()), state)
		return nil, err
	}
	return func() { _ = term.Restore(int(input.Fd()), state) }, nil
}
func Screen(output *os.File) func() {
	if !term.IsTerminal(int(output.Fd())) {
		return func() {}
	}
	io.WriteString(output, "\x1b[?1049h\x1b[?25l\x1b[?2004h")
	return func() { io.WriteString(output, "\x1b[0m\x1b[?2004l\x1b[?25h\x1b[?1049l") }
}
func Open() (*os.File, error) {
	file, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, ErrNoTerminal
	}
	if !term.IsTerminal(int(file.Fd())) {
		file.Close()
		return nil, ErrNoTerminal
	}
	return file, nil
}
