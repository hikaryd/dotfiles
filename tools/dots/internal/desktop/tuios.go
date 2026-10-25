package desktop

import (
	"os"
	"path/filepath"
	"strings"
)

// The protocol stays Herdr-compatible, but outside a pane the daily backend is
// TUIOS. An explicit socket always wins, including the still-running old Herdr.
func muxSocketPath() string {
	if path := os.Getenv("HERDR_SOCKET_PATH"); path != "" {
		return path
	}
	if path := os.Getenv("TUIOS_SOCKET"); path != "" {
		return path + ".herdr"
	}
	data := envOr("XDG_DATA_HOME", filepath.Join(home(), ".local/share"))
	profile := envOr("DOTS_TUIOS_PROFILE", filepath.Join(data, "dots/tuios/profile"))
	server := os.Getenv("TUIOS_SERVER")
	if server == "" {
		if b, err := os.ReadFile(filepath.Join(profile, "server")); err == nil {
			server = strings.TrimSpace(string(b))
		}
	}
	if server == "" {
		server = "dots"
	}
	if !cleanText(server) || strings.ContainsAny(server, "/\\") || server == "." || server == ".." {
		return ""
	}
	path := filepath.Join(profile, "run/tuios/servers", server, "tuios.sock.herdr")
	if info, err := os.Stat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
		return path
	}
	return ""
}
