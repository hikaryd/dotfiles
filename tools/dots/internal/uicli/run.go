// Package uicli exposes the shared terminal UI to thin shell orchestrators.
// Selection stdout is data, never a decorated frame or a reconstructed label.
package uicli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"dots/native/internal/tui"
)

func Run(args []string) int {
	if len(args) > 0 && oneOf(args[0], "--help", "-h", "help") {
		fmt.Println("usage: dots-tools ui select|input|confirm|snapshot|message [--brand TEXT] [--title TEXT] [--secret]")
		return 0
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: dots-tools ui select|input|confirm|snapshot|message [--brand TEXT] [--title TEXT] [--mode TEXT] [--secret] [--value TEXT]")
		return 2
	}
	flags := flag.NewFlagSet("ui", flag.ContinueOnError)
	brand := flags.String("brand", "dots", "tool brand")
	title := flags.String("title", "", "screen title")
	mode := flags.String("mode", "LOCAL", "operation mode")
	level := flags.String("level", "info", "message level: info|success|warning|error")
	intro := flags.String("intro", "", "screen explanation")
	value := flags.String("value", "", "initial non-secret input")
	secret := flags.Bool("secret", false, "mask input")
	multi := flags.Bool("multi", false, "select multiple rows")
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 || !oneOf(args[0], "select", "input", "confirm", "snapshot", "message") || *secret && (args[0] != "input" || *value != "") || *multi && args[0] != "select" {
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	frame := tui.Frame{Brand: *brand, Title: *title, Mode: *mode, Intro: *intro, Tone: tui.Green}
	if *mode == "ACTION" {
		frame.Tone = tui.Yellow
	}
	var err error
	switch args[0] {
	case "select", "snapshot", "message":
		data, readErr := io.ReadAll(io.LimitReader(os.Stdin, 1024*1024+1))
		if readErr != nil || len(data) > 1024*1024 {
			fmt.Fprintln(os.Stderr, "ui: input exceeds 1 MiB or cannot be read")
			return 1
		}
		if args[0] == "message" {
			tone := tui.Accent
			switch *level {
			case "info":
			case "success":
				tone = tui.Green
			case "warning":
				tone = tui.Yellow
			case "error":
				tone = tui.Red
			default:
				return 2
			}
			if !tui.IsTerminal(os.Stdout) {
				_, err = os.Stdout.Write(data)
			} else {
				_, err = fmt.Fprint(os.Stdout, tui.Paint(tui.CleanText(string(data)), tone, tui.ColorEnabled(os.Stdout)))
			}
		} else if args[0] == "snapshot" {
			if !tui.IsTerminal(os.Stdout) {
				_, err = os.Stdout.Write(data)
			} else {
				frame.Width, _ = tui.Size(os.Stdout)
				frame.Color, frame.Body = tui.ColorEnabled(os.Stdout), tui.CleanText(string(data))
				_, err = fmt.Fprint(os.Stdout, frame.Render())
			}
		} else {
			rows := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			if len(data) == 0 {
				fmt.Fprintln(os.Stderr, "ui: no choices")
				return 1
			}
			var indexes []int
			indexes, err = tui.Select(ctx, frame, rows, "", *multi)
			if err == nil {
				for _, index := range indexes {
					if _, err = fmt.Fprintln(os.Stdout, rows[index]); err != nil {
						break
					}
				}
			}
		}
	case "input":
		var answer string
		if *secret {
			answer, err = tui.SecretInput(ctx, frame, "")
		} else {
			answer, err = tui.Input(ctx, frame, *value)
		}
		if err == nil {
			_, err = fmt.Fprintln(os.Stdout, answer)
		}
	case "confirm":
		err = tui.Confirm(ctx, frame)
	}
	if errors.Is(err, tui.ErrCanceled) || errors.Is(err, context.Canceled) {
		return 130
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ui:", err)
		return 1
	}
	return 0
}
func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
