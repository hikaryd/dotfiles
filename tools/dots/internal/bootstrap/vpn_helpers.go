package bootstrap

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

// RunVPNHelper handles installer commands after the vpn group or dots-vpn alias.
// Paths are caller paths, independent of the repository root.
func RunVPNHelper(_ string, args []string) int {
	usage := func() int {
		fmt.Fprintln(os.Stderr, "usage: dots-tools vpn {awg-params PATH|tuning-env [PATH]}")
		return 2
	}
	if len(args) == 0 {
		return usage()
	}
	var err error
	switch args[0] {
	case "awg-params":
		if len(args) != 2 || args[1] == "" {
			return usage()
		}
		err = createAWGParams(args[1])
	case "tuning-env":
		if len(args) > 2 {
			return usage()
		}
		path, present := os.LookupEnv("VPN_ENV")
		if !present {
			path = "/etc/sing-box/vpn.env"
		}
		if len(args) == 2 {
			path = args[1]
		}
		var output string
		output, err = vpnTuningEnv(path)
		if err == nil {
			_, err = fmt.Fprint(os.Stdout, output)
			if err != nil {
				err = errors.New("cannot write tuning environment")
			}
		}
	default:
		return usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// Use bounded rejection sampling to avoid modulo bias and unbounded retries.
func vpnRandomRange(low, high int) (int, error) {
	width := uint64(high - low + 1)
	limit := (uint64(1) << 32) / width * width
	for attempt := 0; attempt < 128; attempt++ {
		var buf [4]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return 0, errors.New("cannot generate AWG randomness")
		}
		n := uint64(binary.BigEndian.Uint32(buf[:]))
		if n < limit {
			return low + int(n%width), nil
		}
	}
	return 0, errors.New("AWG randomness retry limit exceeded")
}

func existingAWGParams(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("cannot inspect AWG parameters")
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("AWG parameters must be a regular file, not a symlink")
	}
	return true, nil
}

func createAWGParams(path string) error {
	if exists, err := existingAWGParams(path); exists || err != nil {
		return err
	}
	var output strings.Builder
	values := make(map[string]int)
	for _, field := range []struct {
		key       string
		low, high int
	}{
		{"Jc", 3, 8}, {"Jmin", 8, 60}, {"Jmax", 0, 120},
		{"S1", 15, 150}, {"S2", 15, 150},
		{"H1", 5, 2147483647}, {"H2", 5, 2147483647},
		{"H3", 5, 2147483647}, {"H4", 5, 2147483647},
	} {
		if field.key == "Jmax" {
			field.low = values["Jmin"] + 10
		}
		accepted := false
		for attempt := 0; attempt < 128; attempt++ {
			n, err := vpnRandomRange(field.low, field.high)
			if err != nil {
				return err
			}
			if field.key == "S2" && n == values["S1"]+56 {
				continue
			}
			if strings.HasPrefix(field.key, "H") && (n == values["H1"] || n == values["H2"] || n == values["H3"]) {
				continue
			}
			values[field.key] = n
			fmt.Fprintf(&output, "%s=%d\n", field.key, n)
			accepted = true
			break
		}
		if !accepted {
			return errors.New("AWG parameter retry limit exceeded")
		}
	}
	// Publish a complete, private file without ever replacing a concurrent file
	// or symlink. Rename would overwrite an entry created after the initial check.
	f, err := os.CreateTemp(filepath.Dir(path), ".awg-params-*")
	if err != nil {
		return errors.New("cannot create private AWG temporary file")
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0o600); err == nil {
		_, err = f.WriteString(output.String())
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("cannot write AWG parameters")
	}
	if err = os.Link(f.Name(), path); errors.Is(err, os.ErrExist) {
		if exists, checkErr := existingAWGParams(path); exists || checkErr != nil {
			return checkErr
		}
	}
	if err != nil {
		return errors.New("cannot publish AWG parameters")
	}
	return nil
}

func vpnTuningEnv(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("cannot read VPN tuning environment")
	}
	if !utf8.Valid(data) {
		return "", errors.New("VPN tuning environment must be UTF-8")
	}
	conf := make(map[string]string)
	// Match load_conf's universal newlines, whitespace and ordered quote stripping;
	// do not interpret export, comments after values, escapes or shell expressions.
	lines := strings.ReplaceAll(strings.ReplaceAll(string(data), "\r\n", "\n"), "\r", "\n")
	for _, line := range strings.Split(lines, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		switch key {
		case "HEALTH_PORT", "AUTO_INCLUDE", "AUTO_EXCLUDE", "AUTO_MAX", "AUTO_PREFER":
			conf[key] = strings.Trim(strings.Trim(strings.TrimSpace(value), `"`), "'")
		}
	}
	portText, present := conf["HEALTH_PORT"]
	if !present {
		portText = "7896"
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil || port < 1 || port > 65535 {
		return "", errors.New("HEALTH_PORT must be in 1..65535")
	}
	var output strings.Builder
	fmt.Fprintf(&output, "HEALTH_PORT=%d\n", port)
	for _, key := range []string{"AUTO_INCLUDE", "AUTO_EXCLUDE", "AUTO_MAX", "AUTO_PREFER"} {
		if value, present := conf[key]; present {
			fmt.Fprintf(&output, "%s=%s\n", key, value)
		}
	}
	return output.String(), nil
}
