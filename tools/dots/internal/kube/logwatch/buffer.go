package logwatch

import (
	"strings"
	"unicode/utf8"
)

// History is bounded by both records and text bytes. Append/eviction are O(1)
// amortized; snapshots are taken only for rendering or a deliberate pause.
type history struct {
	items                          []logLine
	start, length, bytes, maxBytes int
	dropped                        uint64
}

func newHistory(records, maxBytes int) history {
	return history{items: make([]logLine, records), maxBytes: maxBytes}
}
func (h *history) push(line logLine) {
	const suffix = " … [display truncated; use logs-save for the raw record]"
	limit := min(h.maxBytes, 1024*1024) - len(suffix)
	if len(line.text) > limit {
		cut := limit
		for cut > 0 && !utf8.RuneStart(line.text[cut]) {
			cut--
		}
		line.text = line.text[:cut] + suffix
	}
	size := len(line.text)
	for h.length > 0 && (h.length == len(h.items) || h.bytes+size > h.maxBytes) {
		old := h.items[h.start]
		h.bytes -= len(old.text)
		h.items[h.start] = logLine{}
		h.start = (h.start + 1) % len(h.items)
		h.length--
		h.dropped++
	}
	index := (h.start + h.length) % len(h.items)
	h.items[index] = line
	h.length++
	h.bytes += size
}
func (h *history) snapshot() []logLine {
	result := make([]logLine, h.length)
	for i := range result {
		result[i] = h.items[(h.start+i)%len(h.items)]
	}
	return result
}
func displayBody(text string) string {
	// Keep structured metadata out of the reading path while retaining the raw
	// record for search, detail view and saving. Most JSON loggers expose one of
	// these message fields; decoding happens only for visible rows.
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		return jsonMessage(text)
	}
	return text
}
