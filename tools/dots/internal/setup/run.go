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
		fmt.Println("usage: dots-tools setup pi-install|pi-apply|abide-apply|abide-runtime-patch|codex-instructions-apply|codex-task-title-apply|waterfox-apply|tinycast-hotkey|claude-apply-json|claude-sync-json|codexbar-config|managed-links|json-keys [args]")
		return 0
	}
	if (args[0] == "abide-runtime-patch" || args[0] == "json-keys") && len(args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: dots-tools setup %s %s\n", args[0], map[string]string{"abide-runtime-patch": "package", "json-keys": "filename"}[args[0]])
		return 2
	}
	if args[0] != "abide-runtime-patch" && args[0] != "json-keys" && args[0] != "managed-links" && len(args) != 1 {
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
	case "abide-apply":
		err = abideApply(root)
	case "abide-runtime-patch":
		err = abideRuntime(args[1:])
	case "codex-instructions-apply":
		err = codexInstructions(root)
	case "codex-task-title-apply":
		err = taskTitleApply(root)
	case "waterfox-apply":
		err = waterfoxApply(root)
	case "tinycast-hotkey":
		err = tinycastHotkey()
	case "claude-apply-json":
		err = claudeApply(root)
	case "claude-sync-json":
		err = claudeSync(root)
	case "codexbar-config":
		err = codexbarConfig()
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
