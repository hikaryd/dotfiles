package palette

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestGhosttyIndentedColorsAreReplaced(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "config/ghostty/config")
	input := "font-size = 15\n  theme = Ayu\n\tbackground = 123456\n  foreground = 123456\n\tselection-background = 123456\n  selection-foreground = 123456\n\tcursor-color = 123456\n  unfocused-split-fill = 123456\n\tpalette = 0=#123456\n  palette = 4=#123456\n  font-family = Mono\n"
	mustWrite(t, path, []byte(input))
	var out, stderr bytes.Buffer
	if code := run(root, []string{"--tool", "ghostty"}, &out, &stderr); code != 0 {
		t.Fatalf("generation: %s", &stderr)
	}
	got := string(mustRead(t, path))
	var doc struct {
		Dark palette `toml:"dark"`
	}
	if err := toml.Unmarshal(mustRead(t, filepath.Join(root, "palette/ayu.toml")), &doc); err != nil {
		t.Fatal(err)
	}
	assertTerminalPalette(t, got, doc.Dark, true)
	if strings.Contains(got, "123456") || !strings.HasSuffix(got, "font-size = 15\n  font-family = Mono\n") {
		t.Fatalf("stale color or altered settings: %q", got)
	}
}

func TestRepositoryTmuxStyledBlock(t *testing.T) {
	repo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	root := fixture(t)
	for _, path := range []string{"palette/ayu.toml", "config/tmux/tmux.conf"} {
		mustWrite(t, filepath.Join(root, path), mustRead(t, filepath.Join(repo, path)))
	}
	want := `set -g status-style 'bg=default,fg=#a0a0a0'
set -g status-left-length 28
set -g status-right-length 36
set -g status-left '#[fg=#181818,bg=default]#[fg=#99ffe4,bg=#181818]  #[fg=#ffffff]#{=/18/...:session_name} #[fg=#181818,bg=default]  '
set -g status-right '#{?client_prefix,#[fg=#ffc799] PREFIX ,}#{?pane_in_mode,#[fg=#ffc799] COPY ,}#{?window_zoomed_flag,#[fg=#ffc799] ZOOM ,}#[fg=#a0a0a0] %H:%M '

set -g window-status-separator ' '
set -g window-status-style 'bg=default,fg=#a0a0a0'
set -g window-status-format ' #I #{=/16/...:window_name}#{?window_activity_flag, ·,}#{?window_bell_flag, !,} '
set -g window-status-current-format '#[fg=#232323,bg=default]#[fg=#ffc799,bg=#232323,bold] #I #[fg=#ffffff]#{=/16/...:window_name} #[fg=#232323,bg=default]'
set -g window-status-bell-style 'bg=default,fg=#ff8080'
set -g window-status-activity-style 'bg=default,fg=#ffc799'
set -g window-status-last-style 'bg=default,fg=#a0a0a0'

set -g mode-style 'bg=#232323,fg=#ffffff'
set -g copy-mode-match-style 'bg=#232323,fg=#ffc799'
set -g copy-mode-current-match-style 'bg=#ffc799,fg=#101010,bold'
set -g copy-mode-mark-style 'bg=#99ffe4,fg=#101010,bold'
set -g copy-mode-position-style 'bg=#232323,fg=#ffc799'
set -g copy-mode-position-format ' COPY · %position · %scroll_percentage '
set -g message-style 'bg=#232323,fg=#ffffff'
set -g message-command-style 'bg=#232323,fg=#ffc799'
set -g popup-style 'bg=#101010,fg=#ffffff'
set -g popup-border-style 'fg=#ffc799'
set -g popup-border-lines rounded
set -g menu-style 'bg=#101010,fg=#ffffff'
set -g menu-selected-style 'bg=#232323,fg=#ffc799,bold'
set -g menu-border-lines rounded
set -g clock-mode-colour '#ffc799'
set -g display-panes-colour '#a0a0a0'
set -g display-panes-active-colour '#ffc799'
set -g display-panes-time 1500`
	path := filepath.Join(root, "config/tmux/tmux.conf")
	before := mustRead(t, path)
	if !strings.Contains(string(before), want) {
		t.Fatal("repository tmux style block differs from Vesper")
	}
	var out, stderr bytes.Buffer
	if code := run(root, []string{"--tool", "tmux"}, &out, &stderr); code != 0 {
		t.Fatalf("generation: %s", &stderr)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("regeneration changed tmux styles or settings")
	}
}
