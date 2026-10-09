package setup

import (
	"bytes"
	"encoding/json"
	"errors"
)

func keysOfObject(b []byte) ([]string, error) {
	if _, e := object(b); e != nil {
		return nil, e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	_, _ = d.Token()
	var keys []string
	seen := map[string]bool{}
	for d.More() {
		k, e := d.Token()
		if e != nil {
			return nil, errors.New("invalid JSON object")
		}
		var v json.RawMessage
		if e = d.Decode(&v); e != nil {
			return nil, errors.New("invalid JSON object")
		}
		key := k.(string)
		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
	}
	return keys, nil
}
