package vpn

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if os.Getenv("VPN_TEST_CHILD") == "1" {
		os.Exit(helperCommand())
	}
	os.Exit(m.Run())
}
func helperCommand() int {
	role := filepath.Base(os.Args[0])
	args := os.Args[1:]
	if role == "dots-vpn" {
		return Run("", args)
	}
	trace := object{"command": role, "args": args, "pid": os.Getpid()}
	if role == "curl" {
		b, _ := io.ReadAll(os.Stdin)
		trace["stdin"] = string(b)
	}
	if role == "sing-box" || role == "xray" {
		if len(args) > 0 && args[0] == "version" {
			fmt.Println("Xray 26.3.27 fixture")
			return 0
		}
		if len(args) > 1 {
			path := args[len(args)-1]
			b, e := os.ReadFile(path)
			if e != nil {
				return 90
			}
			var cfg object
			if json.Unmarshal(b, &cfg) != nil {
				return 91
			}
			trace["config"] = cfg
			trace["path"] = path
			s, _ := os.Stat(path)
			d, _ := os.Stat(filepath.Dir(path))
			trace["mode"] = int(s.Mode().Perm())
			trace["dir_mode"] = int(d.Mode().Perm())
		}
	}
	f, e := os.OpenFile(os.Getenv("VPN_TEST_TRACE"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return 92
	}
	json.NewEncoder(f).Encode(trace)
	f.Close()
	switch role {
	case "curl":
		if d, _ := time.ParseDuration(os.Getenv("VPN_TEST_CURL_SLEEP")); d > 0 {
			time.Sleep(d)
		}
		if os.Getenv("VPN_TEST_CURL_METRICS") != "" {
			fmt.Print(os.Getenv("VPN_TEST_CURL_METRICS"))
		} else if slices.Contains(args, "%{json}") {
			fmt.Print(`{"http_code":200,"size_download":1000,"speed_download":2000000,"time_starttransfer":0.02,"time_total":0.5}`)
		} else {
			fmt.Print("200\t1000\t0.5\t2000\n")
		}
		n, _ := strconv.Atoi(os.Getenv("VPN_TEST_CURL_EXIT"))
		if strings.Contains(str(trace["stdin"]), "failure.example") {
			n = 28
		}
		return n
	case "sing-box", "xray":
		if slices.Contains(args, "check") || slices.Contains(args, "-test") {
			fmt.Fprintln(os.Stderr, "fake-password fake-panel-secret")
			n, _ := strconv.Atoi(os.Getenv("VPN_TEST_CHECK_EXIT"))
			return n
		}
		if os.Getenv("VPN_TEST_START_FAIL") == "1" {
			return 7
		}
		cfg := obj(trace["config"])
		in := obj(cfg["inbounds"].([]any)[0])
		port := in["listen_port"]
		if role == "xray" {
			port = in["port"]
		}
		l, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%.0f", port))
		if e != nil {
			return 93
		}
		defer l.Close()
		for {
			c, e := l.Accept()
			if e != nil {
				return 0
			}
			c.Close()
		}
	case "stubborn":
		signal.Ignore(syscall.SIGTERM)
		os.WriteFile(os.Getenv("VPN_TEST_READY"), []byte("ready"), 0600)
		for {
			time.Sleep(time.Second)
		}
	case "awg":
		fmt.Print("PRIVATE-PUBLIC-KEY\t10.8.2.2/32\nOTHER-KEY\t10.8.2.3/32\n")
		return 0
	case "modprobe":
		return 0
	case "systemctl":
		if len(args) > 0 && args[0] == "is-active" && os.Getenv("VPN_TEST_INACTIVE") == "1" {
			return 3
		}
		return 0
	case "ip":
		path := os.Getenv("VPN_TEST_ROUTES")
		b, _ := os.ReadFile(path)
		routes := []object{}
		json.Unmarshal(b, &routes)
		if len(args) > 0 && args[0] == "-j" {
			json.NewEncoder(os.Stdout).Encode(routes)
			return 0
		}
		if len(args) >= 4 && args[2] == "add" {
			if args[3] == os.Getenv("VPN_TEST_FAIL_ADD") {
				fmt.Fprintln(os.Stderr, "fake-private-output")
				return 7
			}
			routes = append(routes, hostRoute(args[3], routeMetric, "bbr", "awg0", "static"))
		} else if len(args) >= 4 && args[2] == "del" {
			found := false
			for i, r := range routes {
				if destination(r) == args[3] && owned(r) {
					routes = append(routes[:i], routes[i+1:]...)
					found = true
					break
				}
			}
			if !found {
				return 88
			}
		} else if len(args) >= 4 && args[2] == "show" {
			if os.Getenv("VPN_TEST_NO_LOCK") != "1" {
				fmt.Printf("%s dev awg0 proto static metric 42705 congctl lock bbr\n", args[4])
			}
			return 0
		}
		if os.WriteFile(path, jsonBytes(routes), 0600) != nil {
			return 89
		}
		return 0
	}
	return 87
}
func fakeCommands(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	self, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	for _, n := range []string{"sing-box", "curl", "xray", "stubborn", "awg", "ip", "modprobe", "systemctl"} {
		if e = os.Symlink(self, filepath.Join(dir, n)); e != nil {
			t.Fatal(e)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("VPN_TEST_CHILD", "1")
	t.Setenv("VPN_TEST_TRACE", filepath.Join(t.TempDir(), "trace.jsonl"))
	return dir
}
func traces(t *testing.T) []object {
	t.Helper()
	b, e := os.ReadFile(os.Getenv("VPN_TEST_TRACE"))
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		t.Fatal(e)
	}
	rows := []object{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var r object
		if json.Unmarshal([]byte(line), &r) != nil {
			t.Fatal("bad trace")
		}
		rows = append(rows, r)
	}
	return rows
}
func hostRoute(peer string, metric int, congestion, dev, protocol string) object {
	return object{"dst": peer, "metric": metric, "dev": dev, "protocol": protocol, "metrics": []any{object{"congestion": congestion}}}
}

func fixtureServer(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Skipf("local fixture socket restricted by sandbox: %v", e)
	}
	s := httptest.NewUnstartedServer(h)
	s.Listener = l
	s.Start()
	t.Cleanup(s.Close)
	return s
}
func requireSocket(t *testing.T) {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Skipf("local fixture socket restricted by sandbox: %v", e)
	}
	l.Close()
}
