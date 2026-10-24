package setup

import (
	"errors"
	"os"
)

func sameSource(p, q string) bool {
	a, e := resolve(p)
	if e != nil {
		return false
	}
	b, e := resolve(q)
	return e == nil && a == b
}
func regularOptional(p string) ([]byte, error) {
	i, e := os.Lstat(p)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if !i.Mode().IsRegular() {
		return nil, errors.New("expected a regular configuration file")
	}
	return os.ReadFile(p)
}
