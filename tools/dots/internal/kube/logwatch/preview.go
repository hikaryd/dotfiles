package logwatch

import (
	"time"

	"dots/native/internal/tui"
)

// Preview renders through the same frame builder as the live viewer, without
// kubectl, terminal ownership, environment configuration or any cluster access.
func Preview(width, height int, color bool) tui.Frame {
	pods := stringList{"api-7c9df-abc", "worker-6d8c2-ghi"}
	v := viewer{cfg: config{pods: pods, environment: "stage", context: "dots-stage", namespace: "example-app"}, lines: newHistory(5000, 32*1024*1024), width: width, height: height}
	at := time.Date(2026, 1, 1, 15, 4, 1, 0, time.UTC)
	v.addLine(logLine{pod: pods[0], container: "application", text: `{"level":"INFO","message":"request completed","request_id":"example","method":"GET","status":200,"duration_ms":12}`, at: at})
	v.addLine(logLine{pod: pods[1], container: "worker", text: "WARN retrying upstream request", at: at.Add(time.Second)})
	return v.frame(color)
}
