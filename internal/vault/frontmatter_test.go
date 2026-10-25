package vault

import (
	"slices"
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name string
		in   string
		json string // frontmatter as JSON
		bad  bool
		body string
	}{
		{"none", "hello\n", `{}`, false, "hello\n"},
		{"empty file", "", `{}`, false, ""},
		{"simple", "---\na: 1\n---\nbody\n", `{"a":"1"}`, false, "body\n"},
		{"trailing spaces", "--- \t\na: 1\n---  \nbody", `{"a":"1"}`, false, "body"},
		{"closing at eof", "---\na: 1\n---", `{"a":"1"}`, false, ""},
		{"no closing", "---\na: 1\n", `{}`, false, "---\na: 1\n"},
		{"only opening", "---", `{}`, false, "---"},
		{"empty", "---\n---\nx", `{}`, false, "x"},
		{"whitespace only", "---\n  \n\t\n---\n", `{}`, false, ""},
		{"comments only", "---\n# nothing\n---\n", `{}`, false, ""},
		{"crlf", "---\r\na: 1\r\n---\r\nb\r\nc\rd", `{"a":"1"}`, false, "b\nc\nd"},
		{"bom", "\uFEFF---\na: 1\n---\n", `{"a":"1"}`, false, ""},
		{"indented delimiter is not one", " ---\na: 1\n---\n", `{}`, false, " ---\na: 1\n---\n"},
		{"key order", "---\nz: 1\na: 2\nm: 3\n---\n", `{"z":"1","a":"2","m":"3"}`, false, ""},
		{"no type conversion", "---\nd: 2024-01-01\nn: 007\nb: true\nf: 1.50\n---\n",
			`{"d":"2024-01-01","n":"007","b":"true","f":"1.50"}`, false, ""},
		{"escapes", "---\na: \"x\\ty\"\nb: 'it''s'\n---\n", `{"a":"x\ty","b":"it's"}`, false, ""},
		{"block scalars", "---\nl: |\n  one\n  two\nf: >\n  one\n  two\n---\n", `{"l":"one\ntwo\n","f":"one two\n"}`, false, ""},
		{"nulls", "---\na: ~\nb: null\nc:\nd: !!null x\ne: \"null\"\nf: Null\n---\n",
			`{"a":null,"b":null,"c":null,"d":null,"e":"null","f":null}`, false, ""},
		{"list and map", "---\nl: [a, ~, 1]\nm:\n  x: 1\n  y: [2]\n---\n", `{"l":["a",null,"1"],"m":{"x":"1","y":["2"]}}`, false, ""},
		{"aliases", "---\nbase: &b {x: 1}\ncopy: *b\ns: &s v\nt: *s\n---\n", `{"base":{"x":"1"},"copy":{"x":"1"},"s":"v","t":"v"}`, false, ""},
		{"merge key not expanded", "---\nb: &b {x: 1}\nc:\n  <<: *b\n  y: 2\n---\n", `{"b":{"x":"1"},"c":{"<<":{"x":"1"},"y":"2"}}`, false, ""},
		{"html not escaped", "---\na: <b>&</b>\n---\n", `{"a":"<b>&</b>"}`, false, ""},
		{"syntax error", "---\na: [1\n---\nbody", `{}`, true, "body"},
		{"top-level list", "---\n- a\n- b\n---\n", `{}`, true, ""},
		{"top-level scalar", "---\njust text\n---\n", `{}`, true, ""},
		{"top-level null", "---\n~\n---\n", `{}`, true, ""},
		{"duplicate keys", "---\na: 1\na: 2\n---\n", `{}`, true, ""},
		{"duplicate keys after quoting", "---\na: 1\n\"a\": 2\n---\n", `{}`, true, ""},
		{"nested duplicate keys", "---\nm:\n  a: 1\n  a: 2\n---\n", `{}`, true, ""},
		{"non-scalar key", "---\n? [a]\n: 1\n---\n", `{}`, true, ""},
		{"two documents", "---\na: 1\n...\nb: 2\n---\n", `{}`, true, ""},
		{"alias cycle", "---\na: &x [*x]\n---\n", `{}`, true, ""},
		{"invalid utf-8", "---\na: x\xffy\n---\n", `{"a":"x�y"}`, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := normalize([]byte(tt.in))
			_, body, _ := splitFrontmatter(text)
			n := Parse([]byte(tt.in))
			if got := string(AppendJSON(nil, n.Frontmatter)); got != tt.json {
				t.Errorf("frontmatter = %s, want %s", got, tt.json)
			}
			if n.BadFrontmatter != tt.bad {
				t.Errorf("BadFrontmatter = %v, want %v", n.BadFrontmatter, tt.bad)
			}
			if body != tt.body {
				t.Errorf("body = %q, want %q", body, tt.body)
			}
		})
	}
}

func TestParseBodyAfterBadFrontmatter(t *testing.T) {
	n := Parse([]byte("---\n- a\n---\n#tag\n"))
	if !n.BadFrontmatter || !slices.Equal(n.Tags, []string{"tag"}) {
		t.Errorf("got bad=%v tags=%v", n.BadFrontmatter, n.Tags)
	}
}

func TestNormalizeInvalidBytes(t *testing.T) {
	if got := normalize([]byte("a\xff\xfeb")); got != "a��b" {
		t.Errorf("got %q", got)
	}
}

// aliasList returns a flow sequence of n uses of alias name.
func aliasList(name string, n int) string {
	return "[" + strings.TrimSuffix(strings.Repeat("*"+name+",", n), ",") + "]"
}

func TestAliasExpansionLimit(t *testing.T) {
	// 99 scalars plus the sequence itself: 100 nodes.
	hundred := "a: &a [" + strings.TrimSuffix(strings.Repeat("x,", 99), ",") + "]\n"
	laughs := "a: &a [x,x,x,x,x,x,x,x,x,x]\n"
	for _, name := range []string{"b", "c", "d", "e", "f", "g"} {
		prev := string(rune(name[0] - 1))
		laughs += name + ": &" + name + " " + aliasList(prev, 10) + "\n"
	}
	tests := []struct {
		name string
		yaml string
		bad  bool
	}{
		{"billion laughs", laughs, true},
		{"at the limit", hundred + "b: " + aliasList("a", 100) + "\n", false},
		{"one over the limit", hundred + "b: " + aliasList("a", 100) + "\ns: &s v\nc: *s\n", true},
		{"scalar aliases at the limit", "s: &s v\nl: " + aliasList("s", 10_000) + "\n", false},
		{"scalar aliases over the limit", "s: &s v\nl: " + aliasList("s", 10_001) + "\n", true},
		// Nested aliases count fully: b itself uses 10*10 aliased nodes and each
		// *b brings 1 + 10*10 more (100 + 98*101 = 9 998).
		{"nested aliases", "a: &a [x,x,x,x,x,x,x,x,x]\nb: &b " + aliasList("a", 10) + "\nc: " + aliasList("b", 98) + "\n", false},
		{"nested aliases over", "a: &a [x,x,x,x,x,x,x,x,x]\nb: &b " + aliasList("a", 10) + "\nc: " + aliasList("b", 99) + "\n", true},
		{"map alias counts keys", "m: &m {k: v}\nl: " + aliasList("m", 3334) + "\n", true},
		{"large note without aliases", "l: [" + strings.TrimSuffix(strings.Repeat("x,", 50_000), ",") + "]\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := Parse([]byte("---\n" + tt.yaml + "---\n"))
			if n.BadFrontmatter != tt.bad {
				t.Errorf("BadFrontmatter = %v, want %v", n.BadFrontmatter, tt.bad)
			}
			if tt.bad && n.Frontmatter.Len() != 0 {
				t.Errorf("frontmatter not empty")
			}
		})
	}
}

func TestReplaceInvalid(t *testing.T) {
	tests := []struct{ in, want string }{
		{"abc", "abc"},
		{"a\xffb", "a�b"},
		{"\xff\xfe", "��"},
		{"\xe2\x82", "��"},      // truncated sequence: one per byte
		{"é\xc3", "é�"},         // valid then truncated
		{"�\xff", "��"},         // a real U+FFFD is kept
		{"\xed\xa0\x80", "���"}, // surrogate
	}
	for _, tt := range tests {
		if got := replaceInvalid([]byte(tt.in)); got != tt.want {
			t.Errorf("replaceInvalid(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if got := normalize([]byte(tt.in)); got != tt.want {
			t.Errorf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
