package opencode

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestServeWaitsForShutdown(t *testing.T) {
	for _, force := range []bool{false, true} {
		name := "drain"
		if force {
			name = "force_after_grace"
		}
		t.Run(name, func(t *testing.T) {
			c := testCLI(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.ctx = ctx
			started, release := make(chan struct{}, 1), make(chan struct{})
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				started <- struct{}{}
				<-release
				w.WriteHeader(http.StatusBadGateway)
			}))
			defer proxy.Close()
			var released sync.Once
			releaseProxy := func() { released.Do(func() { close(release) }) }
			defer releaseProxy()
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			cfg := testConfig()
			cfg.Port = listener.Addr().(*net.TCPAddr).Port
			cfg.ProxyURL = proxy.URL
			listener.Close()
			if err := saveConfig(c.config, cfg); err != nil {
				t.Fatal(err)
			}
			ended := make(chan error, 1)
			go func() { ended <- c.serve() }()
			deadline := time.Now().Add(3 * time.Second)
			for !localHealth(cfg) {
				if time.Now().After(deadline) {
					t.Fatal("API did not start")
				}
				time.Sleep(5 * time.Millisecond)
			}
			type result struct {
				code int
				err  error
			}
			response := make(chan result, 1)
			go func() {
				client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 10 * time.Second}
				defer client.CloseIdleConnections()
				res, err := client.Post("http://127.0.0.1:"+strconv.Itoa(cfg.Port)+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"big-pickle"}`))
				if err != nil {
					response <- result{err: err}
					return
				}
				defer res.Body.Close()
				_, err = io.Copy(io.Discard, res.Body)
				response <- result{code: res.StatusCode, err: err}
			}()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not reach test proxy")
			}
			cancel()
			select {
			case err := <-ended:
				t.Fatalf("serve returned before draining active request: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			if !force {
				releaseProxy()
			}
			select {
			case err := <-ended:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(6 * time.Second):
				t.Fatal("shutdown did not finish within its grace period")
			}
			select {
			case got := <-response:
				if force && got.err == nil {
					t.Fatal("stalled request was not forcibly disconnected")
				}
				if !force && (got.err != nil || got.code != http.StatusBadGateway) {
					t.Fatalf("active response did not drain: %+v", got)
				}
			case <-time.After(time.Second):
				t.Fatal("request still active after shutdown")
			}
		})
	}
}
