package codexconfig

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// Decode with the full TOML validator, then use its syntax tree solely to retain
// insertion order. The live document itself is never reserialized.
type entry struct {
	key   string
	value any
}
type object []entry

func (o object) get(k string) (any, bool) {
	for _, e := range o {
		if e.key == k {
			return e.value, true
		}
	}
	return nil, false
}

type orderNode struct {
	keys     []string
	children map[string]*orderNode
	items    []*orderNode
}

func (o *orderNode) child(k string) *orderNode {
	if o.children == nil {
		o.children = map[string]*orderNode{}
	}
	n := o.children[k]
	if n == nil {
		n = &orderNode{}
		o.children[k] = n
		o.keys = append(o.keys, k)
	}
	return n
}
func keyParts(n *unstable.Node) []string {
	var p []string
	it := n.Key()
	for it.Next() {
		p = append(p, string(it.Node().Data))
	}
	return p
}
func orderPath(o *orderNode, p []string) *orderNode {
	for _, k := range p {
		if len(o.items) > 0 {
			o = o.items[len(o.items)-1]
		}
		o = o.child(k)
	}
	return o
}
func orderValue(o *orderNode, n *unstable.Node) {
	switch n.Kind {
	case unstable.InlineTable:
		it := n.Children()
		for it.Next() {
			kv := it.Node()
			orderValue(orderPath(o, keyParts(kv)), kv.Value())
		}
	case unstable.Array:
		it := n.Children()
		for it.Next() {
			c := &orderNode{}
			o.items = append(o.items, c)
			orderValue(c, it.Node())
		}
	}
}
func orderedDecode(text, label string) (object, error) {
	m, e := validate(text, label)
	if e != nil {
		return nil, e
	}
	root := &orderNode{}
	current := root
	p := unstable.Parser{}
	p.Reset([]byte(text))
	for p.NextExpression() {
		n := p.Expression()
		switch n.Kind {
		case unstable.Table, unstable.ArrayTable:
			current = orderPath(root, keyParts(n))
			if n.Kind == unstable.ArrayTable {
				c := &orderNode{}
				current.items = append(current.items, c)
				current = c
			} else if len(current.items) > 0 {
				current = current.items[len(current.items)-1]
			}
		case unstable.KeyValue:
			orderValue(orderPath(current, keyParts(n)), n.Value())
		}
	}
	if p.Error() != nil {
		return nil, fmt.Errorf("%s: invalid TOML syntax", label)
	}
	return orderedValue(m, root).(object), nil
}
func orderedValue(v any, o *orderNode) any {
	if o == nil {
		o = &orderNode{}
	}
	switch x := v.(type) {
	case map[string]any:
		result := object{}
		seen := map[string]bool{}
		for _, k := range o.keys {
			if child, ok := x[k]; ok {
				result = append(result, entry{k, orderedValue(child, o.children[k])})
				seen[k] = true
			}
		}
		var rest []string
		for k := range x {
			if !seen[k] {
				rest = append(rest, k)
			}
		}
		for _, k := range sorted(rest) {
			result = append(result, entry{k, orderedValue(x[k], nil)})
		}
		return result
	case []any:
		result := make([]any, len(x))
		for i, c := range x {
			var order *orderNode
			if i < len(o.items) {
				order = o.items[i]
			}
			result[i] = orderedValue(c, order)
		}
		return result
	default:
		return v
	}
}

var bareKeyRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func tomlKey(k string) string {
	if bareKeyRE.MatchString(k) {
		return k
	}
	return quote(k)
}
func header(p []string) string {
	parts := make([]string, len(p))
	for i, k := range p {
		parts[i] = tomlKey(k)
	}
	return "[" + strings.Join(parts, ".") + "]"
}
func tomlValue(v any) (string, error) {
	switch x := v.(type) {
	case bool:
		return strconv.FormatBool(x), nil
	case string:
		return quote(x), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case float64:
		if math.IsInf(x, 1) {
			return "inf", nil
		}
		if math.IsInf(x, -1) {
			return "-inf", nil
		}
		if math.IsNaN(x) {
			return "nan", nil
		}
		s := strconv.FormatFloat(x, 'g', -1, 64)
		if i := strings.IndexByte(s, 'e'); i >= 0 {
			ex, _ := strconv.Atoi(s[i+1:])
			if ex >= -4 && ex < 16 {
				s = strconv.FormatFloat(x, 'f', -1, 64)
			}
		}
		if !strings.ContainsAny(s, ".e") {
			s += ".0"
		}
		return s, nil
	case []any:
		parts := []string{}
		for _, c := range x {
			s, e := tomlValue(c)
			if e != nil {
				return "", e
			}
			parts = append(parts, s)
		}
		return "[" + strings.Join(parts, ", ") + "]", nil
	case object:
		parts := []string{}
		for _, c := range x {
			s, e := tomlValue(c.value)
			if e != nil {
				return "", e
			}
			parts = append(parts, tomlKey(c.key)+" = "+s)
		}
		return "{ " + strings.Join(parts, ", ") + " }", nil
	default:
		return "", fmt.Errorf("unsupported root TOML value: %T", v)
	}
}
func serializeTree(p []string, o object) ([]tableBlock, error) {
	var scalars []string
	var children []entry
	for _, e := range o {
		if _, ok := e.value.(object); ok {
			children = append(children, e)
		} else {
			v, err := tomlValue(e.value)
			if err != nil {
				return nil, err
			}
			scalars = append(scalars, tomlKey(e.key)+" = "+v)
		}
	}
	var blocks []tableBlock
	if len(scalars) > 0 || len(children) == 0 {
		blocks = append(blocks, tableBlock{p, append([]string{header(p)}, scalars...)})
	}
	for _, c := range children {
		child, e := serializeTree(appendPath(p, c.key), c.value.(object))
		if e != nil {
			return nil, e
		}
		blocks = append(blocks, child...)
	}
	return blocks, nil
}
