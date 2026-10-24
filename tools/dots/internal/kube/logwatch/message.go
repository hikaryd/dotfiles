package logwatch

import (
	"encoding/json"
	"strings"

	"github.com/mattn/go-runewidth"
)

func jsonMessage(text string) string {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(text), &fields) != nil {
		return text
	}
	for _, name := range []string{"message", "msg", "event"} {
		var message string
		if json.Unmarshal(fields[name], &message) == nil && message != "" {
			return message
		}
	}
	return text
}
func readableDetails(text string) string {
	var document any
	if json.Unmarshal([]byte(text), &document) == nil {
		if pretty, err := json.MarshalIndent(document, "", "  "); err == nil {
			return string(pretty)
		}
	}
	return text
}
func wrapCells(text string, width int) []string {
	width = max(1, width)
	result := []string{}
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		result = append(result, strings.Split(runewidth.Wrap(paragraph, width), "\n")...)
	}
	return result
}
