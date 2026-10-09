package vpn

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

const globalRoutes = `[
{"dst":"10.8.2.0/24","dev":"awg0","protocol":"kernel","scope":"link","prefsrc":"10.8.2.1","flags":[]},
{"dst":"10.8.2.2","dev":"awg0","protocol":"static","metric":42705,"flags":[],"metrics":[{"congestion":"bbr"}]},
{"dst":"10.8.2.3","dev":"awg0","protocol":"static","metric":42704,"flags":[],"metrics":[{"congestion":"bbr"}]},
{"dst":"10.8.2.4","dev":"awg0","protocol":"static","metric":20,"flags":[],"metrics":[{"congestion":"bbr"}]},
{"dst":"10.8.2.5","dev":"eth0","protocol":"static","metric":42705,"metrics":[{"congestion":"bbr"}]},
{"dst":"10.8.2.6","protocol":"static","metric":42705,"metrics":[{"congestion":"bbr"}]}]`

func tuningFixture(t *testing.T, routes []object) (tuning, string) {
	t.Helper()
	fakeCommands(t)
	d := t.TempDir()
	path := filepath.Join(d, "routes.json")
	t.Setenv("VPN_TEST_ROUTES", path)
	if routes == nil {
		routes = []object{}
	}
	os.WriteFile(path, jsonBytes(routes), 0600)
	return tuning{filepath.Join(d, "config.json"), filepath.Join(d, "dots-vpn"), filepath.Join(d, "unit.service"), func(n string, a ...string) (string, error) { return safeCommand(context.Background(), n, a...) }}, path
}
func readRoutesFixture(t *testing.T, path string) []object {
	t.Helper()
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var r []object
	if json.Unmarshal(b, &r) != nil {
		t.Fatal("bad routes")
	}
	return r
}
func mutated(rows []object) bool {
	for _, r := range rows {
		cmd := str(r["command"])
		a, _ := r["args"].([]any)
		if cmd == "modprobe" || cmd == "systemctl" || slices.Contains(a, any("add")) || slices.Contains(a, any("del")) {
			return true
		}
	}
	return false
}
func TestApplyRegisteredPeerPreservesOtherHomeAndGlobals(t *testing.T) {
	other := hostRoute("10.8.2.3/32", 42703, "cubic", "awg0", "static")
	tt, path := tuningFixture(t, []object{other, {"dst": "10.8.2.0/24", "dev": "awg0", "protocol": "kernel"}})
	tt.save([]string{"10.8.2.2/32"})
	if e := tt.apply(); e != nil {
		t.Fatal(e)
	}
	rs := readRoutesFixture(t, path)
	equal(t, len(rs), 3)
	equal(t, rs[0]["metric"], float64(42703))
	equal(t, destination(rs[2]), "10.8.2.2/32")
	if !owned(rs[2]) {
		t.Fatal(rs)
	}
	for _, r := range traces(t) {
		if slices.Contains([]string{"sysctl", "tc", "systemctl"}, str(r["command"])) {
			t.Fatal(r)
		}
		a := r["args"].([]any)
		if slices.Contains(a, any("add")) {
			equal(t, a[len(a)-3:], []any{"congctl", "lock", "bbr"})
		}
	}
}
func TestInvalidPeerBeforeMutation(t *testing.T) {
	for _, p := range []any{"10.8.2.99/32", "10.8.2.0/24", "::1/128", 2} {
		t.Run(strings.ReplaceAll(str(p), "/", "_"), func(t *testing.T) {
			tt, _ := tuningFixture(t, nil)
			os.WriteFile(tt.config, jsonBytes(object{"peers": []any{p}}), 0600)
			if e := tt.apply(); e == nil {
				t.Fatal("invalid accepted")
			}
			if mutated(traces(t)) {
				t.Fatal("mutated before validation")
			}
		})
	}
}
func TestForeignHostRouteNeverOverwritten(t *testing.T) {
	for _, r := range []object{hostRoute("10.8.2.2/32", 20, "bbr", "awg0", "static"), hostRoute("10.8.2.2/32", routeMetric, "bbr", "eth0", "static")} {
		t.Run(str(r["dev"]), func(t *testing.T) {
			tt, path := tuningFixture(t, []object{r})
			tt.save([]string{"10.8.2.2/32"})
			before, _ := os.ReadFile(path)
			if e := tt.apply(); e == nil {
				t.Fatal("foreign route accepted")
			}
			after, _ := os.ReadFile(path)
			equal(t, string(after), string(before))
			if mutated(traces(t)) {
				t.Fatal("foreign route touched")
			}
		})
	}
}
func TestCleanupAllOwnershipMarkersNoConfig(t *testing.T) {
	rs := []object{hostRoute("10.8.2.2/32", routeMetric, "bbr", "awg0", "static"), hostRoute("10.8.2.3/32", 42703, "bbr", "awg0", "static"), hostRoute("10.8.2.7/32", 42704, "bbr", "awg0", "static"), hostRoute("10.8.2.4/32", routeMetric, "cubic", "awg0", "static"), hostRoute("10.8.2.5/32", routeMetric, "bbr", "awg0", "boot"), hostRoute("10.8.2.6/32", routeMetric, "bbr", "eth0", "static")}
	tt, path := tuningFixture(t, rs)
	if e := tt.cleanup(); e != nil {
		t.Fatal(e)
	}
	left := readRoutesFixture(t, path)
	equal(t, len(left), len(rs)-1)
	for _, r := range left {
		if owned(r) {
			t.Fatal("owned remains")
		}
	}
}
func TestAddFailureRollsBackOnlyNewRoutes(t *testing.T) {
	previous := hostRoute("10.8.2.9/32", routeMetric, "bbr", "awg0", "static")
	tt, path := tuningFixture(t, []object{previous})
	tt.save([]string{"10.8.2.2/32", "10.8.2.3/32"})
	t.Setenv("VPN_TEST_FAIL_ADD", "10.8.2.3/32")
	if e := tt.apply(); e == nil {
		t.Fatal("missing failure")
	}
	rs := readRoutesFixture(t, path)
	equal(t, len(rs), 1)
	equal(t, destination(rs[0]), "10.8.2.9/32")
}
func TestReapplyIdempotentOnlyDeselectedRemoved(t *testing.T) {
	tt, path := tuningFixture(t, []object{hostRoute("10.8.2.2/32", routeMetric, "bbr", "awg0", "static"), hostRoute("10.8.2.3/32", routeMetric, "bbr", "awg0", "static")})
	tt.save([]string{"10.8.2.2/32"})
	for range 2 {
		if e := tt.apply(); e != nil {
			t.Fatal(e)
		}
	}
	rs := readRoutesFixture(t, path)
	equal(t, len(rs), 1)
	equal(t, destination(rs[0]), "10.8.2.2/32")
	for _, r := range traces(t) {
		if slices.Contains(r["args"].([]any), any("add")) {
			t.Fatal("unnecessary add")
		}
	}
}
func TestGlobalBareDestinationIdempotent(t *testing.T) {
	var rs []object
	json.Unmarshal([]byte(globalRoutes), &rs)
	tt, path := tuningFixture(t, rs)
	tt.save([]string{"10.8.2.2/32"})
	before := readRoutesFixture(t, path)
	for range 2 {
		if e := tt.apply(); e != nil {
			t.Fatal(e)
		}
	}
	equal(t, readRoutesFixture(t, path), before)
	for _, r := range traces(t) {
		a := r["args"].([]any)
		if slices.Contains(a, any("add")) || slices.Contains(a, any("del")) {
			t.Fatal("route changed")
		}
	}
}
func TestGlobalCleanupPreservesMissingDeviceAndForeign(t *testing.T) {
	var rs []object
	json.Unmarshal([]byte(globalRoutes), &rs)
	tt, path := tuningFixture(t, rs)
	if owned(rs[5]) {
		t.Fatal("missing device owned")
	}
	if e := tt.cleanup(); e != nil {
		t.Fatal(e)
	}
	left := readRoutesFixture(t, path)
	equal(t, len(left), 5)
	for _, r := range traces(t) {
		a := r["args"].([]any)
		if slices.Contains(a, any("del")) {
			equal(t, a[3], "10.8.2.2/32")
		}
	}
}
func TestInstallDefaultsPreservesOptInNativeUnit(t *testing.T) {
	tt, _ := tuningFixture(t, nil)
	if e := tt.install(nil); e != nil {
		t.Fatal(e)
	}
	p, e := tt.readPeers()
	if e != nil {
		t.Fatal(e)
	}
	equal(t, len(p), 0)
	if e = tt.install([]string{"10.8.2.2/32"}); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(tt.config)
	if e = tt.install(nil); e != nil {
		t.Fatal(e)
	}
	after, _ := os.ReadFile(tt.config)
	equal(t, string(after), string(before))
	s, _ := os.Stat(tt.config)
	equal(t, s.Mode().Perm(), os.FileMode(0600))
	unit, _ := os.ReadFile(tt.unit)
	for _, line := range []string{"BindsTo=" + tuningParent, "PartOf=" + tuningParent, "WantedBy=" + tuningParent, "ExecStart=/usr/local/libexec/dots-vpn tcp-tuning apply", "ExecStop=/usr/local/libexec/dots-vpn tcp-tuning cleanup"} {
		if !strings.Contains(string(unit), line) {
			t.Fatal("missing native unit contract")
		}
	}
	if strings.Contains(string(unit), "python") {
		t.Fatal("interpreter unit")
	}
	self, _ := os.Executable()
	want, _ := os.ReadFile(self)
	installed, _ := os.ReadFile(tt.helper)
	if !reflect.DeepEqual(installed, want) {
		t.Fatal("did not install self")
	}
	count := 0
	for _, r := range traces(t) {
		a := r["args"].([]any)
		if slices.Contains(a, any("restart")) {
			equal(t, a, []any{"restart", tuningService})
			count++
		}
	}
	equal(t, count, 3)
}
func TestInactiveAWGInstallWritesNothing(t *testing.T) {
	tt, _ := tuningFixture(t, nil)
	t.Setenv("VPN_TEST_INACTIVE", "1")
	if e := tt.install(nil); e == nil {
		t.Fatal("inactive accepted")
	}
	for _, path := range []string{tt.config, tt.helper, tt.unit} {
		if _, e := os.Stat(path); !os.IsNotExist(e) {
			t.Fatal("wrote on inactive")
		}
	}
}
func TestDisableClearsOptInAndOwnedRoute(t *testing.T) {
	tt, path := tuningFixture(t, []object{hostRoute("10.8.2.2/32", routeMetric, "bbr", "awg0", "static")})
	tt.save([]string{"10.8.2.2/32"})
	if e := tt.disable(); e != nil {
		t.Fatal(e)
	}
	equal(t, len(readRoutesFixture(t, path)), 0)
	p, e := tt.readPeers()
	if e != nil {
		t.Fatal(e)
	}
	equal(t, len(p), 0)
}
func TestFailedCommandOutputRedacted(t *testing.T) {
	fakeCommands(t)
	t.Setenv("VPN_TEST_CHECK_EXIT", "7")
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte(`{}`), 0600)
	_, e := safeCommand(context.Background(), "sing-box", "check", "-c", path)
	if e == nil || strings.Contains(e.Error(), "fake") || !strings.Contains(e.Error(), "exit 7") {
		t.Fatal(e)
	}
}
func TestMissingLockRollsBackAndLockWaitBounded(t *testing.T) {
	tt, path := tuningFixture(t, nil)
	tt.save([]string{"10.8.2.2/32"})
	t.Setenv("VPN_TEST_NO_LOCK", "1")
	if e := tt.apply(); e == nil {
		t.Fatal("missing lock accepted")
	}
	equal(t, len(readRoutesFixture(t, path)), 0)
	lock := filepath.Join(t.TempDir(), "lock")
	unlock, e := lockFile(context.Background(), lock)
	if e != nil {
		t.Fatal(e)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, e = lockFile(ctx, lock)
	if e == nil {
		t.Fatal("unbounded or unheld lock")
	}
}
func TestRollbackFailureReported(t *testing.T) {
	tt, _ := tuningFixture(t, nil)
	tt.save([]string{"10.8.2.2/32", "10.8.2.3/32"})
	original := tt.execute
	tt.execute = func(n string, a ...string) (string, error) {
		if slices.Contains(a, "del") {
			return "", errors.New("fixture deletion failed")
		}
		return original(n, a...)
	}
	t.Setenv("VPN_TEST_FAIL_ADD", "10.8.2.3/32")
	e := tt.apply()
	if e == nil || !strings.Contains(e.Error(), "rollback failed") {
		t.Fatal(e)
	}
}
