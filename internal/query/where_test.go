package query

import (
	"strings"
	"testing"

	"github.com/betoissues/vq/internal/vault"
)

func TestParseWhere(t *testing.T) {
	tests := []struct {
		expr  string
		key   string
		op    Op
		value string
		err   string // substring of the error, "" for none
	}{
		{"status", "status", OpPresent, "", ""},
		{"  status  ", "status", OpPresent, "", ""},
		{"!status", "status", OpAbsent, "", ""},
		{"! status", "status", OpAbsent, "", ""},
		{"status=done", "status", OpEq, "done", ""},
		{" status = done ", "status", OpEq, "done", ""},
		{"status!=done", "status", OpNe, "done", ""},
		{"title~^a.*b$", "title", OpMatch, "^a.*b$", ""},
		{"n>5", "n", OpGt, "5", ""},
		{"n<5", "n", OpLt, "5", ""},
		{"n>=5", "n", OpGe, "5", ""},
		{"n<=5", "n", OpLe, "5", ""},
		{"a=b=c", "a", OpEq, "b=c", ""},
		{"a==b", "a", OpEq, "=b", ""},
		{"a=", "a", OpEq, "", ""},
		{"my key=x", "my key", OpEq, "x", ""},
		{`t="hello world"`, "t", OpEq, "hello world", ""},
		{`t='it's'`, "t", OpEq, "it's", ""},
		{`t= " padded " `, "t", OpEq, " padded ", ""},
		{`t="a'`, "t", OpEq, `"a'`, ""},
		{`t="`, "t", OpEq, `"`, ""},
		{`t=""`, "t", OpEq, "", ""},
		{"", "", "", "", "empty key"},
		{"  ", "", "", "", "empty key"},
		{"!", "", "", "", "empty key"},
		{"=x", "", "", "", "empty key"},
		{" >x", "", "", "", "empty key"},
		{"a!b", "", "", "", "malformed"},
		{"!status=done", "", "", "", "did you mean 'status!=done'?"},
		{"!status>done", "", "", "", "cannot be combined"},
		{"!a!b", "", "", "", "malformed"},
		{"t~(", "", "", "", "invalid regular expression"},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			w, err := ParseWhere(tt.expr)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("err = %v, want %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if w.Key != tt.key || w.Op != tt.op || w.Value != tt.value {
				t.Errorf("got (%q %q %q), want (%q %q %q)", w.Key, w.Op, w.Value, tt.key, tt.op, tt.value)
			}
		})
	}
}

func frontmatter(t *testing.T, yaml string) *vault.Map {
	t.Helper()
	n := vault.Parse([]byte("---\n" + yaml + "\n---\n"))
	if n.BadFrontmatter {
		t.Fatalf("bad frontmatter: %q", yaml)
	}
	return n.Frontmatter
}

func TestWhereMatch(t *testing.T) {
	fm := `status: Done
empty: ''
null:
list: [a, B]
nulls: [~]
emptylist: []
emptymap: {}
map: {x: 1}
maps: [{x: 1}]
mixed: [{x: 1}, c]
date: 2025-03-10
title: "Hello <World>"
`
	tests := []struct {
		expr string
		want bool
	}{
		{"status", true},
		{"empty", false},
		{"null", false},
		{"missing", false},
		{"emptylist", false},
		{"emptymap", false},
		{"nulls", true},
		{"map", true},
		{"!status", false},
		{"!empty", true},
		{"!missing", true},
		{"!emptymap", true},
		{"!map", false},
		{"Status", false},

		{"status=done", true},
		{"status=DONE", true},
		{"status=don", false},
		{"list=b", true},
		{"list=c", false},
		{"nulls=", true},
		{"empty=", false},
		{"map=1", false},
		{"maps=1", false},
		{"mixed=c", true},
		{"missing=x", false},

		{"status!=open", true},
		{"status!=done", false},
		{"list!=a", false},
		{"list!=c", true},
		{"missing!=x", true},
		{"empty!=", true},
		{"map!=x", true},

		{"status~^d", true},
		{"status~^D.N", true},
		{"status~^x", false},
		{"list~^b$", true},
		{"title~<world>", true},
		{"missing~.*", false},
		{"empty~.*", false},
		{"map~.*", false},

		{"date>=2025-01", true},
		{"date>2025-03-10", false},
		{"date>=2025-03-10", true},
		{"date<2025-04", true},
		{"date<=2025-03-09", false},
		{"status>c", true},
		{"status<E", true},
		{"list>a", true},
		{"list<a", false},
		{"missing<z", false},
	}
	m := frontmatter(t, fm)
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			w, err := ParseWhere(tt.expr)
			if err != nil {
				t.Fatal(err)
			}
			if got := w.Match(m); got != tt.want {
				t.Errorf("Match = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWhereRegexOnOriginalValue(t *testing.T) {
	// The value is not lowercased before matching: (?i) handles case.
	m := frontmatter(t, "k: ÀB")
	w, err := ParseWhere("k~^àb$")
	if err != nil {
		t.Fatal(err)
	}
	if !w.Match(m) {
		t.Error("no match")
	}
}

func TestWhereNFC(t *testing.T) {
	nfd := frontmatter(t, "city: Bogota\u0301\nlist: [x, Mu\u0308nchen]\nd: e\u0301")
	nfc := frontmatter(t, "city: Bogot\u00e1\nlist: [x, M\u00fcnchen]\nd: \u00e9")
	tests := []struct {
		expr string
		want bool
	}{
		{"city=Bogot\u00e1", true},
		{"city=bogota\u0301", true},
		{"city=BOGOT\u00c1", true},
		{"city=Bogota", false},
		{"city!=Bogot\u00e1", false},
		{"city!=bogota\u0301", false},
		{"list=m\u00fcnchen", true},
		{"list!=mu\u0308nchen", false},
		{"city~t\u00e1$", true},
		{"city~ta\u0301$", true},
		{"city~^bogot.$", true}, // one character, not two
		{"city~a$", false},
		{"d>=\u00e9", true},
		{"d<=e\u0301", true},
		{"d>e", true},
		{"d<f", false}, // é sorts after f in byte order
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			w, err := ParseWhere(tt.expr)
			if err != nil {
				t.Fatal(err)
			}
			if got := w.Match(nfd); got != tt.want {
				t.Errorf("NFD note: Match = %v, want %v", got, tt.want)
			}
			if got := w.Match(nfc); got != tt.want {
				t.Errorf("NFC note: Match = %v, want %v", got, tt.want)
			}
		})
	}
	// The value keeps the form it was given in.
	w, _ := ParseWhere("city=Bogota\u0301")
	if w.Value != "Bogota\u0301" {
		t.Errorf("Value = %q", w.Value)
	}
}
