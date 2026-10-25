package vault

import "testing"

func TestAppendJSON(t *testing.T) {
	m := NewMap()
	m.Set("z", "<a & b>")
	m.Set("a", nil)
	m.Set("l", []any{"x", nil, []any{}})
	inner := NewMap()
	inner.Set("k", "line\nbreak\t\"q\"")
	m.Set("m", inner)
	m.Set("e", NewMap())

	want := `{"z":"<a & b>","a":null,"l":["x",null,[]],"m":{"k":"line\nbreak\t\"q\""},"e":{}}`
	if got := string(AppendJSON(nil, m)); got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestMapSetReplaces(t *testing.T) {
	m := NewMap()
	m.Set("a", "1")
	m.Set("b", "2")
	m.Set("a", "3")
	if got := string(AppendJSON(nil, m)); got != `{"a":"3","b":"2"}` {
		t.Errorf("got %s", got)
	}
}
