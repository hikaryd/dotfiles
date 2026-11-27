package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/term"
)

var ErrNoTerminal = errors.New("interactive terminal required")
var ErrCanceled = errors.New("selection canceled")
var ErrAction = errors.New("additional screen action")

type Key struct{ Name, Text string }

// Keys сводит стрелки и Ctrl+N/P к down/up, распознаёт одиночный Esc, Unicode
// и bracketed paste. Вставленный текст не считается командой или сочетанием клавиш.
func Keys(ctx context.Context, input *os.File) <-chan Key {
	output := make(chan Key, 32)
	runes := make(chan rune, 64)
	go func() {
		defer close(runes)
		pending := []byte{}
		buffer := make([]byte, 4096)
		for ctx.Err() == nil {
			n, err := input.Read(buffer)
			// VMIN=0/VTIME=1 по таймауту даёт пустое чтение, os.File видит EOF.
			// Это не конец сессии терминала.
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
					case "A", "112;5u", "1079;5u":
						send(Key{Name: "up"})
					case "B", "110;5u", "1090;5u":
						send(Key{Name: "down"})
					case "27;5;112~":
						if second == '[' {
							send(Key{Name: "up"})
						}
					case "27;5;110~":
						if second == '[' {
							send(Key{Name: "down"})
						}
					case "101;5u", "1091;5u", "27;5;101~":
						send(Key{Name: "ctrl-e"})
					case "108;5u", "1076;5u", "27;5;108~":
						send(Key{Name: "ctrl-l"})
					case "111;5u", "1097;5u", "27;5;111~":
						send(Key{Name: "ctrl-o"})
					case "114;5u", "1082;5u", "27;5;114~":
						send(Key{Name: "ctrl-r"})
					case "120;5u", "1095;5u", "27;5;120~":
						send(Key{Name: "ctrl-x"})
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
				case 14:
					name = "down"
				case 15:
					name = "ctrl-o"
				case 16:
					name = "up"
				case 18:
					name = "ctrl-r"
				case 24:
					name = "ctrl-x"
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

var screenOwner struct {
	sync.Mutex
	output *os.File
	active bool
}

// HoldScreen держит прежний экран, пока грузится следующий.
func HoldScreen() func() {
	terminal, err := Open()
	if err != nil {
		return func() {}
	}
	screenOwner.Lock()
	if screenOwner.output != nil {
		screenOwner.Unlock()
		terminal.Close()
		return func() {}
	}
	screenOwner.output = terminal
	screenOwner.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			screenOwner.Lock()
			defer screenOwner.Unlock()
			if screenOwner.output != terminal {
				return
			}
			if screenOwner.active {
				io.WriteString(terminal, "\x1b[0m\x1b[?2004l\x1b[?25h\x1b[?1049l")
			}
			screenOwner.output, screenOwner.active = nil, false
			terminal.Close()
		})
	}
}

// ReleaseScreen выходит из alternate screen, даже если его открыл вызывающий (dots menu).
func ReleaseScreen() {
	screenOwner.Lock()
	defer screenOwner.Unlock()
	if screenOwner.output == nil {
		return
	}
	if screenOwner.active {
		io.WriteString(screenOwner.output, "\x1b[0m\x1b[?2004l\x1b[?25h\x1b[?1049l")
	}
	screenOwner.output.Close()
	screenOwner.output, screenOwner.active = nil, false
}

func Screen(output *os.File) func() {
	if !term.IsTerminal(int(output.Fd())) {
		return func() {}
	}
	screenOwner.Lock()
	if screenOwner.output != nil {
		if !screenOwner.active {
			io.WriteString(output, "\x1b[?1049h\x1b[?25l\x1b[?2004h")
			screenOwner.active = true
		}
		screenOwner.Unlock()
		return func() {}
	}
	screenOwner.Unlock()
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
