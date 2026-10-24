package proxy

import (
	"bytes"
	"encoding/xml"
	"errors"
	"os"
)

// A missing plist is safe only for an unloaded job. Never overwrite or delete
// a foreign definition, even when it happens to use our launchd label.
func (c *controller) ownedService() ([]string, error) {
	st, err := os.Lstat(c.servicePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !st.Mode().IsRegular() {
		return nil, errors.New("Unsafe proxy LaunchAgent file; no service changed.")
	}
	raw, err := os.ReadFile(c.servicePath())
	if err != nil {
		return nil, err
	}
	var plist struct {
		Args []string `xml:"dict>array>string"`
	}
	if xml.Unmarshal(raw, &plist) != nil || len(plist.Args) != 4 {
		return nil, errors.New("Foreign proxy LaunchAgent file; no service changed.")
	}
	expected, err := c.servicePlist(plist.Args[0])
	if err != nil || !bytes.Equal(expected, raw) {
		return nil, errors.New("Proxy LaunchAgent definition differs; no service changed.")
	}
	return plist.Args, nil
}
