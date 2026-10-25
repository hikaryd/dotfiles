// Package setup implements the installer helpers used by dots-tools setup.
package setup

import (
	"fmt"
	"os"
	"path/filepath"
)

// Run dispatches a setup helper and returns its process exit status.
func Run(root string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: dots-tools setup COMMAND")
		return 2
	}
	if args[0] == "--help" || args[0] == "-h" || len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
		fmt.Println("usage: dots-tools setup jcode-apply|jcode-neo4j-init|jevgrep-apply|waterfox-apply|tinycast-hotkey|managed-links|json-keys|cleanup [args]")
		return 0
	}
	if args[0] == "json-keys" && len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: dots-tools setup json-keys filename")
		return 2
	}
	if args[0] != "json-keys" && args[0] != "managed-links" && args[0] != "cleanup" && len(args) != 1 {
		fmt.Fprintln(os.Stderr, "setup: unexpected arguments")
		return 2
	}
	if home() == "" {
		fmt.Fprintln(os.Stderr, "setup: cannot determine home directory")
		return 1
	}
	var err error
	switch args[0] {
	case "cleanup":
		err = cleanup(root, args[1:])
	case "jcode-apply":
		err = jcodeApply(root)
	case "jcode-neo4j-init":
		data := absolute(envDefault([]string{"JCODE_INTEGRATIONS_DIR"}, filepath.Join(envDefault([]string{"XDG_DATA_HOME"}, filepath.Join(home(), ".local/share")), "jcode-integrations")))
		_, err = jcodeNeo4jPassword(data)
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
