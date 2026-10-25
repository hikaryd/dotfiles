package integration_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestNeovimRmuxAdapterUsesNativeSocketAndRawClipboard(t *testing.T) {
	bin, err := exec.LookPath("nvim")
	if err != nil {
		t.Skip("nvim is not installed")
	}
	root, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	f := newFixture(t)
	f.env = append(f.env, "RMUX=/tmp/with,comma.sock,42,0", "TMUX=/wrong/socket,42,0", "HERDR_ENV=1")
	f.fake("rmux", fakeRule{Args: []string{"-S", "/tmp/with,comma.sock", "select-pane", "-L"}},
		fakeRule{Args: []string{"-S", "/tmp/with,comma.sock", "show-buffer", "-b", "buffer0"}, Output: "first\r\nlast\n"},
		fakeRule{Args: []string{"-S", "/tmp/with,comma.sock", "list-buffers", "-F", "#{buffer_name}"}, Output: "buffer0\n"})
	script := filepath.Join(f.base, "adapter.lua")
	f.write(script, `local root = `+strconv.Quote(root)+`
local wrapper = {execute = function() error('upstream tmux command was not replaced') end}
local moved = false
package.preload['tmux.wrapper.tmux'] = function() return wrapper end
package.preload.tmux = function() return {setup = function() end, move_left = function() moved = true end} end
local spec = dofile(root .. '/config/nvim/lua/plugins/tools/tmux.lua')
assert(spec.cond(), 'inherited Herdr context disabled rmux integration')
spec.config()
wrapper.execute({'select-pane', '-L'}, 'clipboard input\n')
assert(wrapper.get_buffer('buffer0') == 'first\r\nlast\n', 'clipboard bytes changed')
assert(wrapper.get_buffer_names()[1] == 'buffer0')
vim.g.mapleader = ' '
dofile(root .. '/config/nvim/lua/core/mappings.lua')
vim.fn.maparg(' h', 'n', false, true).callback()
assert(moved, 'split navigation used stale Herdr')
`, 0600)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--headless", "-u", "NONE", "-n", "-l", script)
	cmd.Env = f.env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Neovim adapter: %v: %s", err, out)
	}
	calls := f.calls()
	if len(calls) != 3 || calls[0].Input != "clipboard input\n" {
		t.Fatalf("adapter calls: %+v", calls)
	}
}
