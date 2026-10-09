package proxy

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func runSpotify(home string, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("spotify-proxy", flag.ContinueOnError)
	fs.SetOutput(errOut)
	port := fs.Int("port", 10810, "HTTP proxy port")
	check := fs.Bool("check", false, "check without modifying preferences")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(errOut, "spotify-proxy: unrecognized arguments")
		return 2
	}
	if *port < 1 || *port > 65535 {
		fmt.Fprintln(errOut, "spotify-proxy: proxy port must be between 1 and 65535")
		return 2
	}
	path := filepath.Join(home, "Library", "Application Support", "Spotify", "prefs")
	text, err := optionalText(path)
	if err != nil {
		fmt.Fprintln(errOut, "spotify-proxy: could not access preferences (OSError)")
		return 2
	}
	keys := []string{"network.proxy.mode", "network.proxy.addr"}
	values := []string{"2", fmt.Sprintf("\"127.0.0.1:%d@http\"", *port)}
	counts := [2]int{}
	matches := [2]bool{}
	var lines []string
	// splitlines semantics: ignore the trailing newline but preserve interior blanks.
	parts := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	if text == "" {
		parts = nil
	}
	for _, line := range parts {
		key, value, found := strings.Cut(line, "=")
		managed := false
		for i, k := range keys {
			if key == k {
				managed = true
				if found {
					counts[i]++
					matches[i] = value == values[i]
				}
			}
		}
		if !managed {
			lines = append(lines, line)
		}
	}
	configured := counts[0] == 1 && counts[1] == 1 && matches[0] && matches[1]
	if *check {
		if configured {
			return 0
		}
		return 1
	}
	if configured {
		return 0
	}
	for i, key := range keys {
		lines = append(lines, key+"="+values[i])
	}
	mode := os.FileMode(0600)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintln(errOut, "spotify-proxy: could not access preferences (OSError)")
		return 2
	}
	if err := atomicWrite(path, []byte(strings.Join(lines, "\n")+"\n"), mode); err != nil {
		fmt.Fprintln(errOut, "spotify-proxy: could not access preferences (OSError)")
		return 2
	}
	return 0
}
