// Package setup implements the installer helpers used by dots-tools setup.
package setup

import (
	"fmt"
	"os"
)

// Run dispatches a setup helper and returns its process exit status.
func Run(root string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: dots-tools setup COMMAND")
		return 2
	}
	if args[0] == "--help" || args[0] == "-h" || len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		fmt.Println("usage: dots-tools setup pi-install|pi-apply|jevgrep-apply|waterfox-apply|tinycast-hotkey|managed-links|json-keys [args]")
		return 0
	}
	if args[0] == "json-keys" && len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: dots-tools setup json-keys filename")
		return 2
	}
	if args[0] != "json-keys" && args[0] != "managed-links" && len(args) != 1 {
		fmt.Fprintln(os.Stderr, "setup: unexpected arguments")
		return 2
	}
	if home() == "" {
		fmt.Fprintln(os.Stderr, "setup: cannot determine home directory")
		return 1
	}
	var err error
	switch args[0] {
	case "pi-apply":
		err = piApply(root)
	case "jevgrep-apply":
		err = jevgrepApply(root)
	case "waterfox-apply":
		err = waterfoxApply(root)
	case "tinycast-hotkey":
		err = tinycastHotkey()
	case "managed-links":
		err = managedLinks(root, args[1:])
	case "json-keys":
		err = jsonKeys(args[1:])
	default:
		fmt.Fprintln(os.Stderr, "unknown setup command")
		return 2
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", args[0], err)
		return 1
	}
	return 0
}
