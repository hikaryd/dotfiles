package rmuxstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestCaptureAnimatedTitlesPreserveTopologyGuards(t *testing.T) {
	for _, mode := range []string{"title", "id", "session", "index", "layout", "width", "height", "active", "unknown", "missing-name", "invalid-name", "null-name", "empty", "malformed", "pane"} {
		t.Run(mode, func(t *testing.T) {
			store := storeFor(t)
			if _, err := store.Save(fixture()); err != nil {
				t.Fatal(err)
			}
			before, err := store.read("latest")
			if err != nil {
				t.Fatal(err)
			}
			queries, paneQueries := 0, 0
			r := Runtime{Home: t.TempDir(), Store: store, processes: func() (map[int]Process, error) {
				return map[int]Process{100: {Parent: 1, Executable: "/bin/sh", Started: time.Unix(100, 0)}}, nil
			}}
			r.run = func(args ...string) (string, error) {
				switch args[0] {
				case "show-options":
					return r.token(), nil
				case "set-option":
					return "", nil
				case "list-sessions":
					return `[{"session_name":"work"}]`, nil
				case "list-windows":
					queries++
					row := map[string]any{"session_name": "work", "window_id": 1, "window_index": 2, "window_name": "editor", "window_layout": "b264,80x24,0,0,7", "window_width": 80, "window_height": 24, "window_active": true}
					if queries > 1 {
						switch mode {
						case "title":
							row["window_name"] = "editor - busy"
						case "id":
							row["window_id"] = 2
						case "session":
							row["session_name"] = "other"
						case "index":
							row["window_index"] = 3
						case "layout":
							row["window_layout"] = withChecksum("80x24,0,0,8")
						case "width":
							row["window_width"] = 81
						case "height":
							row["window_height"] = 25
						case "active":
							row["window_active"] = false
						case "unknown":
							row["future_metadata"] = true
						case "missing-name":
							delete(row, "window_name")
						case "invalid-name":
							row["window_name"] = 42
						case "null-name":
							row["window_name"] = nil
						case "empty":
							return "[]", nil
						case "malformed":
							return "{", nil
						}
					}
					data, err := json.Marshal([]map[string]any{row})
					return string(data), err
				case "list-panes":
					paneQueries++
					cwd := "/"
					if mode == "pane" && paneQueries > 1 {
						cwd = "/changed"
					}
					data, err := json.Marshal([]map[string]any{{"session_name": "work", "window_index": 2, "pane_index": 0, "pane_id": "%7", "pane_current_path": cwd, "pane_active": true}})
					return string(data), err
				case "display-message":
					if args[len(args)-1] == "#{pane_pid}" {
						return "100", nil
					}
					return "", nil
				}
				return "", errors.New("unexpected call")
			}
			_, err = r.Save()
			after, readErr := store.read("latest")
			if readErr != nil {
				t.Fatal(readErr)
			}
			if mode == "title" {
				if err != nil {
					t.Fatalf("live title animation blocked save: %v", err)
				}
				saved, err := store.Load()
				if err != nil {
					t.Fatal(err)
				}
				if saved.Sessions[0].Windows[0].Name != "editor" {
					t.Fatal("did not retain first captured title")
				}
			} else if err == nil || !bytes.Equal(before, after) {
				t.Fatalf("unsafe %s update committed: %v", mode, err)
			}
		})
	}
}
