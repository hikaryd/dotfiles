package tui

import (
	"fmt"
	"os"
	"strings"
)

// Painter updates only changed rows in the alternate screen. It never prints a
// newline on the bottom row (which would scroll the entire screen) and never
// clears a whole frame. One synchronized write prevents half-painted frames.
type Painter struct {
	output        *os.File
	previous      []string
	width, height int
}

func NewPainter(output *os.File) *Painter { return &Painter{output: output} }
func (p *Painter) Draw(frame Frame) {
	rendered := frame.Render()
	if !IsTerminal(p.output) {
		fmt.Fprint(p.output, Plain(rendered))
		return
	}
	lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
	if frame.Height > 0 && len(lines) > frame.Height {
		lines = lines[:frame.Height]
	}
	force := p.width != frame.Width || p.height != frame.Height
	var out strings.Builder
	for index := 0; index < max(len(lines), len(p.previous)); index++ {
		if frame.Height > 0 && index >= frame.Height {
			break
		}
		line := ""
		if index < len(lines) {
			line = lines[index]
		}
		if !force && index < len(p.previous) && p.previous[index] == line {
			continue
		}
		fmt.Fprintf(&out, "\x1b[%d;1H\x1b[0m%s\x1b[K", index+1, line)
	}
	if out.Len() > 0 {
		fmt.Fprint(p.output, "\x1b[?2026h", out.String(), "\x1b[?2026l")
	}
	p.previous = lines
	p.width, p.height = frame.Width, frame.Height
}
