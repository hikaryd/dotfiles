// Package launchd verifies the identity of a loaded job before mutation.
package launchd

import (
	"slices"
	"strings"
)

// Matches compares top-level launchctl print fields, not a same-label plist
// which may have been replaced after the job was loaded.
func Matches(output []byte, path string, argv []string) bool {
	var loadedPath, program string
	var args []string
	depth, inArgs := 0, false
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if inArgs {
			if line == "}" {
				inArgs = false
			} else {
				args = append(args, line)
			}
			continue
		}
		if depth == 1 {
			switch {
			case strings.HasPrefix(line, "path = "):
				loadedPath = strings.TrimPrefix(line, "path = ")
			case strings.HasPrefix(line, "program = "):
				program = strings.TrimPrefix(line, "program = ")
			case line == "arguments = {":
				inArgs = true
				continue
			}
		}
		if strings.HasSuffix(line, "{") {
			depth++
		} else if line == "}" {
			depth--
		}
	}
	return len(argv) > 0 && depth == 0 && !inArgs && loadedPath == path && program == argv[0] && slices.Equal(args, argv)
}
