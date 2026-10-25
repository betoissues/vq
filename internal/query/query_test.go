package query

import (
	"testing"

	"github.com/betoissues/vq/internal/vault"
)

func TestTag(t *testing.T) {
	n := vault.Parse([]byte("---\ntags: [project/alpha, Idea]\n---\n"))
	tests := []struct {
		tag  string
		want bool
	}{
		{"project", true},
		{"#project", true},
		{"##PROJECT", true},
		{"project/alpha", true},
		{"project/alp", false},
		{"proj", false},
		{"idea", true},
		{"IDEA", true},
		{"alpha", false},
		{"project/", true},
		{"project//", true},
		{"#project/alpha/", true},
		{"project/alp/", false},
	}
	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			tag, err := ParseTag(tt.tag)
			if err != nil {
				t.Fatal(err)
			}
			if got := tag.Match(n); got != tt.want {
				t.Errorf("Match = %v, want %v", got, tt.want)
			}
		})
	}
	for _, s := range []string{"", "#", "##", "/", "#/", "#//"} {
		if _, err := ParseTag(s); err == nil {
			t.Errorf("ParseTag(%q): no error", s)
		}
	}
}

func TestQuery(t *testing.T) {
	n := vault.Parse([]byte("---\nstatus: open\ntags: [a]\n---\n#b\n"))
	mustTag := func(s string) Tag {
		tag, err := ParseTag(s)
		if err != nil {
			t.Fatal(err)
		}
		return tag
	}
	mustWhere := func(s string) Where {
		w, err := ParseWhere(s)
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	mustPath := func(s string) Path {
		p, err := ParsePath(s)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		name string
		q    Query
		rel  string
		want bool
	}{
		{"no filters", Query{}, "x.md", true},
		{"tags ANDed", Query{Tags: []Tag{mustTag("a"), mustTag("b")}}, "x.md", true},
		{"tags ANDed, one missing", Query{Tags: []Tag{mustTag("a"), mustTag("c")}}, "x.md", false},
		{"where ANDed", Query{Wheres: []Where{mustWhere("status=open"), mustWhere("!done")}}, "x.md", true},
		{"tag and where", Query{Tags: []Tag{mustTag("a")}, Wheres: []Where{mustWhere("status=done")}}, "x.md", false},
		{"paths ORed", Query{Paths: []Path{mustPath("a"), mustPath("b")}}, "b/x.md", true},
		{"paths ORed, none", Query{Paths: []Path{mustPath("a"), mustPath("b")}}, "c/x.md", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.q.MatchPath(tt.rel) && tt.q.MatchNote(n); got != tt.want {
				t.Errorf("match = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTagNFC(t *testing.T) {
	nfd := vault.Parse([]byte("#cafe\u0301/sub"))
	nfc := vault.Parse([]byte("#caf\u00e9/sub"))
	for _, s := range []string{"caf\u00e9", "cafe\u0301", "CAFE\u0301", "#CAF\u00c9/", "cafe\u0301/sub"} {
		tag, err := ParseTag(s)
		if err != nil {
			t.Fatal(err)
		}
		if !tag.Match(nfd) || !tag.Match(nfc) {
			t.Errorf("ParseTag(%q): nfd %v, nfc %v", s, tag.Match(nfd), tag.Match(nfc))
		}
	}
	tag, _ := ParseTag("cafe")
	if tag.Match(nfd) {
		t.Error("cafe matches café")
	}
}
