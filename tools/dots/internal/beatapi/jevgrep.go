package beatapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (c *cli) backupPath() string {
	return filepath.Join(filepath.Dir(c.config), "jevgrep-credentials.before-beatapi.json")
}
func (c *cli) connectJevgrep(cfg Config) error {
	binary, err := exec.LookPath("jg")
	if err != nil {
		return errors.New("beatapi: official jevgrep (jg) is unavailable; client unchanged")
	}
	path := filepath.Join(c.base, "jevgrep/credentials.json")
	if err := privateDir(filepath.Dir(path)); err != nil {
		return errors.New("beatapi: cannot secure jevgrep directory; client unchanged")
	}
	var before []byte
	existed := false
	if err := privateFile(path); err == nil {
		f, e := os.Open(path)
		if e != nil {
			return errors.New("beatapi: cannot read jevgrep credentials")
		}
		before, err = io.ReadAll(io.LimitReader(f, 1<<20+1))
		f.Close()
		if err != nil || len(before) > 1<<20 {
			return errors.New("beatapi: jevgrep credentials unreadable or too large")
		}
		existed = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("beatapi: unsafe jevgrep credentials; client unchanged")
	}
	baseURL := fmt.Sprintf("http://%s/v1", cfg.address())
	matches := func(b []byte) bool { return jevgrepMatches(b, cfg) }
	if matches(before) {
		return nil
	}
	if existed {
		backup := c.backupPath()
		f, e := os.OpenFile(backup, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e == nil {
			_, e = f.Write(before)
			if e == nil {
				e = f.Sync()
			}
			closeErr := f.Close()
			if e == nil {
				e = closeErr
			}
			if e != nil {
				os.Remove(backup)
				return errors.New("beatapi: cannot back up jevgrep credentials; client unchanged")
			}
		} else if !errors.Is(e, os.ErrExist) || privateFile(backup) != nil {
			return errors.New("beatapi: unsafe jevgrep backup; client unchanged")
		}
	}
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "auth", "--provider", "custom", "--base-url", baseURL, "--model", jevModel, "--stdin")
	cmd.Stdin = strings.NewReader("local\n")
	// Real provider keys never enter jg argv, environment or stdin.
	runErr := cmd.Run()
	after, readErr := os.ReadFile(path)
	if runErr == nil && readErr == nil && privateFile(path) == nil && matches(after) {
		return nil
	}
	var rollback error
	if existed {
		rollback = atomicPrivate(path, before)
	} else {
		if st, e := os.Lstat(path); e == nil {
			if st.Mode().IsRegular() {
				rollback = os.Remove(path)
			} else {
				rollback = errors.New("unsafe credentials file")
			}
		}
	}
	if rollback != nil {
		return errors.New("beatapi: jg auth failed and credentials restoration failed; recover the private backup before retrying")
	}
	return errors.New("beatapi: jg auth failed or wrote incompatible credentials; previous client configuration restored")
}
