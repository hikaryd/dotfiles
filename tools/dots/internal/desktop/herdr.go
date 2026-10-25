package desktop

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"dots/native/internal/mux"
)

// Explicit socket paths use one request per connection. Serialise before dialing:
// Herdr 0.9.1 otherwise incurs its 100ms empty-read polling delay.
func herdrCall(method string, params map[string]any, argv ...string) (map[string]any, error) {
	return herdrCallContext(context.Background(), method, params, argv...)
}
func herdrCallContext(ctx context.Context, method string, params map[string]any, argv ...string) (map[string]any, error) {
	var raw []byte
	if path := os.Getenv("HERDR_SOCKET_PATH"); path != "" {
		if params == nil {
			params = map[string]any{}
		}
		b, e := json.Marshal(map[string]any{"id": "dots-desktop", "method": method, "params": params})
		if e != nil {
			return nil, errors.New("Некорректный запрос Herdr")
		}
		b = append(b, '\n')
		dialer := net.Dialer{Timeout: 2 * time.Second}
		c, e := dialer.DialContext(ctx, "unix", path)
		if e != nil {
			return nil, errors.New("Herdr socket недоступен")
		}
		defer c.Close()
		stop := context.AfterFunc(ctx, func() { c.Close() })
		defer stop()
		if e = c.SetDeadline(time.Now().Add(2 * time.Second)); e != nil {
			return nil, errors.New("Herdr socket deadline failed")
		}
		if _, e = c.Write(b); e != nil {
			return nil, errors.New("Herdr request failed")
		}
		reader := bufio.NewReader(c)
		raw, e = readLineLimited(reader, 16*maxBytes)
		if e != nil {
			return nil, errors.New("Herdr response unavailable or too large")
		}
	} else {
		r, e := captureCommandContext(ctx, 2*time.Second, 16*maxBytes, "", nil, append([]string{"herdr"}, argv...)...)
		if e != nil {
			return nil, e
		}
		if r.code != 0 {
			return nil, errors.New("Herdr command failed")
		}
		s := r.out
		// report-metadata acknowledges success with its exit status in the CLI.
		if method == "workspace.report_metadata" && strings.TrimSpace(s) == "" {
			return map[string]any{}, nil
		}
		raw = []byte(s)
	}
	var response map[string]any
	if json.Unmarshal(raw, &response) != nil || response["error"] != nil || object(response["result"]) == nil {
		return nil, errors.New("Некорректный ответ Herdr")
	}
	return object(response["result"]), nil
}
func readLineLimited(r *bufio.Reader, limit int) ([]byte, error) {
	b := []byte{}
	for {
		part, e := r.ReadSlice('\n')
		if len(b)+len(part) > limit {
			return nil, errors.New("limit")
		}
		b = append(b, part...)
		if e == bufio.ErrBufferFull {
			continue
		}
		return b, e
	}
}
func herdrSnapshot() (map[string]any, error) { return herdrSnapshotContext(context.Background()) }
func herdrSnapshotContext(ctx context.Context) (map[string]any, error) {
	r, e := herdrCallContext(ctx, "session.snapshot", nil, "api", "snapshot")
	if e != nil {
		return nil, e
	}
	s := object(r["snapshot"])
	if s == nil || array(s["workspaces"]) == nil {
		return nil, errors.New("Некорректный снимок Herdr")
	}
	return s, nil
}
func inHerdr() bool {
	return !mux.IsRmux() && (os.Getenv("HERDR_ENV") != "" || os.Getenv("HERDR_SOCKET_PATH") != "")
}
func herdrFocus(id string) error {
	_, e := herdrCall("workspace.focus", map[string]any{"workspace_id": id}, "workspace", "focus", id)
	return e
}
func herdrCreate(path, label, workspace string) (map[string]any, error) {
	p := map[string]any{"cwd": path, "label": label, "focus": false}
	if workspace == "" {
		return herdrCall("workspace.create", p, "workspace", "create", "--cwd", path, "--label", label, "--no-focus")
	}
	p["workspace_id"] = workspace
	return herdrCall("tab.create", p, "tab", "create", "--workspace", workspace, "--cwd", path, "--label", label, "--no-focus")
}
func herdrRename(tab, label string) error {
	_, e := herdrCall("tab.rename", map[string]any{"tab_id": tab, "label": label}, "tab", "rename", tab, label)
	return e
}
func responseID(r map[string]any, field, key string) (string, error) {
	s := str(object(r[field])[key])
	if !cleanText(s) {
		return "", errors.New("Некорректный идентификатор Herdr")
	}
	return s, nil
}
