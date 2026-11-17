package agents

import (
	"fmt"
	"io"
	"os"
)

func Run(root string, args []string) int { return run(args, os.Stdin, os.Stdout, os.Stderr) }
func run(args []string, in io.Reader, out, errout io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errout, "Использование: dots-tools agents ai")
		return 2
	}
	switch args[0] {
	case "ai":
		return runAI(args[1:], in, out, errout)
	default:
		fmt.Fprintln(errout, "unknown agents command")
		return 2
	}
}
