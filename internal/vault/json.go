package vault

import (
	"bytes"
	"encoding/json"
)

// AppendJSON appends the compact JSON encoding of a frontmatter value to dst.
// Maps keep key order and HTML characters are not escaped.
func AppendJSON(dst []byte, v any) []byte {
	switch v := v.(type) {
	case nil:
		return append(dst, "null"...)
	case string:
		return AppendJSONString(dst, v)
	case []any:
		dst = append(dst, '[')
		for i, item := range v {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = AppendJSON(dst, item)
		}
		return append(dst, ']')
	case *Map:
		dst = append(dst, '{')
		for i, k := range v.Keys() {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = AppendJSONString(dst, k)
			dst = append(dst, ':')
			item, _ := v.Get(k)
			dst = AppendJSON(dst, item)
		}
		return append(dst, '}')
	}
	panic("vault: unexpected frontmatter value")
}

// AppendJSONString appends s as a JSON string to dst, without escaping HTML
// characters.
func AppendJSONString(dst []byte, s string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // cannot fail for a string
	return append(dst, bytes.TrimSuffix(buf.Bytes(), []byte("\n"))...)
}
