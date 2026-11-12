package setup

import (
	"testing"
)

func TestInstructionBlockRejectsAmbiguousMarkers(t *testing.T) {
	const begin, end = "<!-- BEGIN DOTS SEARCH -->", "<!-- END DOTS SEARCH -->"
	for _, input := range []string{begin + "\npersonal\n" + begin + "\nold\n" + end, begin + "\nold\n" + end + "\n" + end, end, "inline " + begin + "\nold\n" + end, end + "\n" + begin} {
		if _, err := instructionBlock(input, "new", begin, end); err == nil {
			t.Errorf("accepted ambiguous markers: %q", input)
		}
	}
}

