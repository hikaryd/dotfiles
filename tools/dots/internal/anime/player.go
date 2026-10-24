package anime

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"sort"
	"strings"
	"time"
	"unicode"

	"dots/native/internal/tui"
)

func chooseVideo(videos []Video, quality int) (Video, error) {
	if len(videos) == 0 {
		return Video{}, fmt.Errorf("видеопотоки пока недоступны")
	}
	sorted := append([]Video(nil), videos...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Quality < sorted[j].Quality })
	selected := sorted[0]
	for _, v := range sorted {
		if quality == 0 || v.Quality <= quality {
			selected = v
		}
	}
	return selected, nil
}

func playerArgs(video Video, extra []string, title string) ([]string, error) {
	u, err := url.Parse(video.URL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil || strings.IndexFunc(video.URL, unicode.IsControl) >= 0 {
		return nil, fmt.Errorf("некорректный адрес видеопотока")
	}
	args := append([]string(nil), extra...)
	args = append(args, "--no-terminal", "--input-terminal=no", "--force-media-title="+tui.Clean(title))
	keys := make([]string, 0, len(video.Headers))
	for key := range video.Headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	headers := make([]string, 0, len(keys))
	for _, key := range keys {
		value := video.Headers[key]
		if key == "" || strings.IndexFunc(key, func(r rune) bool {
			return !strings.ContainsRune("!#$%&'*+-.^_`|~", r) && !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9')
		}) >= 0 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return nil, fmt.Errorf("некорректный HTTP заголовок видеопотока")
		}
		header := strings.ReplaceAll(key+": "+value, "\\", "\\\\")
		headers = append(headers, strings.ReplaceAll(header, ",", "\\,"))
	}
	if len(headers) > 0 {
		args = append(args, "--http-header-fields="+strings.Join(headers, ","))
	}
	return append(args, "--", video.URL), nil
}

func playVideo(ctx context.Context, video Video, extra []string, title string, onStart func() error) error {
	args, err := playerArgs(video, extra, title)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "mpv", args...)
	cmd.Stdout = io.Discard
	stderr := &diagnosticBuffer{limitedBuffer: limitedBuffer{limit: 8192}}
	cmd.Stderr = stderr
	cmd.WaitDelay = time.Second
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("не удалось запустить mpv: %w", err)
	}
	if err = onStart(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("не удалось сохранить историю: %w", err)
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("mpv: %w\n%s", err, tui.CleanText(string(stderr.Bytes())))
	}
	return nil
}
