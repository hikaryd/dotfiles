package proxy

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestSubscriptionSizeLimitPreservesCache(t *testing.T) {
	for _, compressed := range []bool{false, true} {
		t.Run(strconv.FormatBool(compressed), func(t *testing.T) {
			c := testController(t)
			s := fixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data := bytes.Repeat([]byte("x"), (8<<20)+1)
				if compressed {
					w.Header().Set("Content-Encoding", "gzip")
					z := gzip.NewWriter(w)
					defer z.Close()
					z.Write(data)
				} else {
					w.Write(data)
				}
			}))
			c.saveSubscription(s.URL)
			c.saveServers([]object{{"name": "existing"}})
			before, _ := os.ReadFile(c.path("servers.json"))
			if _, err := c.updateServers(false); err == nil {
				t.Fatal("oversized response accepted")
			}
			after, _ := os.ReadFile(c.path("servers.json"))
			if !bytes.Equal(before, after) {
				t.Fatal("cache changed")
			}
			// fetch itself must reject before decoding or parsing.
			if _, err := c.fetchSubscription(s.URL, false); err == nil {
				t.Fatal("download was unbounded")
			}
		})
	}
}

func TestServiceCollisionRefusesMutation(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		t.Run(strconv.FormatBool(loaded), func(t *testing.T) {
			c := testController(t)
			fixture := fakeCommands(t)
			os.MkdirAll(filepath.Dir(c.servicePath()), 0700)
			data := []byte("foreign plist")
			if loaded {
				data, _ = c.servicePlist(filepath.Join(c.home, "bin/dots-tools"))
				os.WriteFile(filepath.Join(fixture, "loaded"), []byte("foreign job"), 0600)
			}
			os.WriteFile(c.servicePath(), data, 0600)
			if err := c.installServiceAt(filepath.Join(c.home, "bin/dots-tools")); err == nil {
				t.Error("foreign service accepted on install")
			}
			if err := c.uninstallService(); err == nil {
				t.Error("foreign service accepted on uninstall")
			}
			after, _ := os.ReadFile(c.servicePath())
			if !bytes.Equal(after, data) {
				t.Error("foreign plist mutated")
			}
			for _, call := range serviceCalls(t, fixture) {
				if call[0] != "print" {
					t.Errorf("mutation: %v", call)
				}
			}
		})
	}
}
