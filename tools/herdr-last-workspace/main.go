// dots-last-workspace tracks and toggles workspace focus through Herdr's Unix socket.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const commandTimeout = 4 * time.Second

type config struct {
	socketPath string
	stateDir   string
	eventJSON  string
}

func configFromEnv() (config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return config{}, err
	}
	socketPath := os.Getenv("HERDR_SOCKET_PATH")
	if socketPath == "" {
		root := os.Getenv("XDG_CONFIG_HOME")
		if root == "" {
			root = filepath.Join(home, ".config")
		}
		root = filepath.Join(root, "herdr")
		if session := os.Getenv("HERDR_SESSION"); session != "" {
			root = filepath.Join(root, "sessions", session)
		}
		socketPath = filepath.Join(root, "herdr.sock")
	}
	stateDir := os.Getenv("HERDR_PLUGIN_STATE_DIR")
	if stateDir == "" {
		root := os.Getenv("XDG_STATE_HOME")
		if root == "" {
			root = filepath.Join(home, ".local", "state")
		}
		stateDir = filepath.Join(root, "herdr", "plugins", "dots.last-workspace")
	}
	return config{socketPath, stateDir, os.Getenv("HERDR_PLUGIN_EVENT_JSON")}, nil
}

// Keep the Python plugin's filename and JSON fields so switching implementations
// preserves history. Different Herdr sessions never share a focus state or lock.
func (c config) statePath() string {
	hash := sha256.Sum256([]byte(c.socketPath))
	return filepath.Join(c.stateDir, fmt.Sprintf("%x.json", hash[:8]))
}

type focusState struct {
	Current  string `json:"current"`
	Previous string `json:"previous"`
}

func readState(path string) (focusState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return focusState{}, nil
	}
	if err != nil {
		return focusState{}, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return focusState{}, nil // As before, a malformed history can be rebuilt.
	}
	var state focusState
	// Legacy nulls and non-string fields represent an unknown workspace.
	_ = json.Unmarshal(fields["current"], &state.Current)
	_ = json.Unmarshal(fields["previous"], &state.Previous)
	return state, nil
}

func writeState(path string, state focusState) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".focus-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	// Close even when encoding or syncing fails; rename only a complete file.
	err = json.NewEncoder(file).Encode(state)
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func lockState(ctx context.Context, path string) (*os.File, error) {
	lockPath := path[:len(path)-len(".json")] + ".lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			file.Close()
			return nil, fmt.Errorf("waiting for focus lock: %w", err)
		}
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return file, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			file.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Millisecond):
		}
	}
}

type socketClient struct {
	path string
}

func (c socketClient) call(ctx context.Context, method string, params any, result any) error {
	const id = "dots:last-workspace"
	request := struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{id, method, params}
	// Herdr 0.9.1 sleeps for 100ms if an accepted socket has no request bytes
	// ready. Prepare the entire line before connecting, then write it at once.
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("%s: encode request: %w", method, err)
	}
	payload = append(payload, '\n')
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.path)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return err
		}
	}
	if written, err := conn.Write(payload); err != nil {
		return fmt.Errorf("%s: send request: %w", method, err)
	} else if written != len(payload) {
		return fmt.Errorf("%s: send request: %w", method, io.ErrShortWrite)
	}
	var response struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	// Bound unexpected replies without imposing Scanner's small token limit.
	if err := json.NewDecoder(io.LimitReader(conn, 8<<20)).Decode(&response); err != nil {
		return fmt.Errorf("%s: read response: %w", method, err)
	}
	if response.ID != id {
		return fmt.Errorf("%s: response id mismatch", method)
	}
	if response.Error != nil {
		return fmt.Errorf("%s: %s: %s", method, response.Error.Code, response.Error.Message)
	}
	if len(response.Result) == 0 || string(response.Result) == "null" {
		return fmt.Errorf("%s: missing result", method)
	}
	if result != nil {
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("%s: invalid result: %w", method, err)
		}
	}
	return nil
}

// workspace.list contains everything we need and avoids loading panes, agents,
// and layouts from the much larger session.snapshot used by the old CLI helper.
func (c socketClient) workspaces(ctx context.Context) (string, map[string]bool, error) {
	var result struct {
		Type       string `json:"type"`
		Workspaces []struct {
			ID      string `json:"workspace_id"`
			Focused bool   `json:"focused"`
		} `json:"workspaces"`
	}
	if err := c.call(ctx, "workspace.list", struct{}{}, &result); err != nil {
		return "", nil, err
	}
	if result.Type != "workspace_list" {
		return "", nil, fmt.Errorf("workspace.list: unexpected result type %q", result.Type)
	}
	active := ""
	live := make(map[string]bool, len(result.Workspaces))
	for _, workspace := range result.Workspaces {
		if workspace.ID == "" {
			continue
		}
		live[workspace.ID] = true
		if workspace.Focused {
			active = workspace.ID
		}
	}
	return active, live, nil
}

func run(ctx context.Context, c config, action string) error {
	var target string
	switch action {
	case "prime", "toggle":
	case "focused":
		var event struct {
			Event string `json:"event"`
			Data  struct {
				WorkspaceID string `json:"workspace_id"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(c.eventJSON), &event); err != nil {
			return fmt.Errorf("invalid HERDR_PLUGIN_EVENT_JSON: %w", err)
		}
		if event.Event != "workspace_focused" && event.Event != "workspace.focused" {
			return nil
		}
		target = event.Data.WorkspaceID
	default:
		return errors.New("usage: dots-last-workspace prime|focused|toggle")
	}
	if err := os.MkdirAll(c.stateDir, 0o700); err != nil {
		return err
	}
	path := c.statePath()
	lock, err := lockState(ctx, path)
	if err != nil {
		return err
	}
	defer lock.Close() // Closing releases flock, including on any API failure.

	client := socketClient{c.socketPath}
	active, live, err := client.workspaces(ctx)
	if err != nil {
		return err
	}
	if !live[active] || (action == "focused" && target != active) {
		return nil // A newer focus event overtook this asynchronous hook.
	}
	state, err := readState(path)
	if err != nil {
		return err
	}
	before := state
	if state.Current != active {
		state.Previous = ""
		if live[state.Current] {
			state.Previous = state.Current
		}
		state.Current = active
	}
	if action == "toggle" {
		if live[state.Previous] && state.Previous != active {
			if err := client.call(ctx, "workspace.focus", map[string]string{"workspace_id": state.Previous}, nil); err != nil {
				return err // Never commit a toggle Herdr rejected.
			}
			state = focusState{Current: state.Previous, Previous: active}
		} else {
			state.Previous = "" // Discard history referring to a closed workspace.
		}
	}
	if state != before {
		return writeState(path, state)
	}
	return nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: dots-last-workspace prime|focused|toggle")
		os.Exit(1)
	}
	c, err := configFromEnv()
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
		defer cancel()
		err = run(ctx, c, os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Herdr last workspace:", err)
		os.Exit(1)
	}
}
