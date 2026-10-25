// Package vault walks an Obsidian-style vault and parses its notes.
package vault

// A frontmatter value is one of:
//
//   - nil: a YAML null
//   - string: a scalar, exactly as YAML yields it (never type-converted)
//   - []any: a sequence
//   - *Map: a mapping, in key order

// Map is a mapping with string keys that keeps key order.
type Map struct {
	keys []string
	vals map[string]any
}

// NewMap returns an empty map.
func NewMap() *Map {
	return &Map{vals: map[string]any{}}
}

// Set adds key with value v, or replaces its value if key exists.
func (m *Map) Set(key string, v any) {
	if _, ok := m.vals[key]; !ok {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = v
}

// Get returns the value of key and whether the key exists.
func (m *Map) Get(key string) (any, bool) {
	v, ok := m.vals[key]
	return v, ok
}

// Keys returns the keys in order.
func (m *Map) Keys() []string {
	return m.keys
}

// Len returns the number of keys.
func (m *Map) Len() int {
	return len(m.keys)
}
