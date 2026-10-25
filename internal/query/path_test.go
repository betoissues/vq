package query

import "testing"

func TestParsePathErrors(t *testing.T) {
	for _, s := range []string{"", "/", "///"} {
		if _, err := ParsePath(s); err == nil {
			t.Errorf("ParsePath(%q): no error", s)
		}
	}
}

func TestPathMatch(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"notes", "notes/a.md", true},
		{"notes", "notes/sub/a.md", true},
		{"notes", "notes.md", false},
		{"notes", "notesx/a.md", false},
		{"notes", "other/notes/a.md", false},
		{"/notes/", "notes/a.md", true},
		{"./notes", "notes/a.md", true},
		{"./notes/", "notes/a.md", true},
		{"././notes", "notes/a.md", true},
		{".//notes", "notes/a.md", true},
		{"/./notes", "notes/a.md", true},
		{"./*.md", "x/a.md", true},
		{"notes/./a.md", "notes/a.md", false},
		{"../notes", "notes/a.md", false},
		{".notes", ".notes/a.md", true},
		{"./.notes", ".notes/a.md", true},
		{"notes/a.md", "notes/a.md", true},
		{"notes/a", "notes/a.md", false},
		{"Notes", "notes/a.md", false},

		{"*.md", "a.md", true},
		{"*.md", "x/y/a.md", true},
		{"notes/*", "notes/sub/a.md", true},
		{"*/drafts/*.md", "a/drafts/b.md", true},
		{"*/drafts/*.md", "drafts/b.md", false},
		{"a?c.md", "abc.md", true},
		{"a?c.md", "a/c.md", true},
		{"a?c.md", "ac.md", false},
		{"*.MD", "a.md", false},
		{"[ab].md", "a.md", true},
		{"[ab].md", "c.md", false},
		{"[!ab].md", "c.md", true},
		{"[!ab].md", "a.md", false},
		{"[a-c].md", "b.md", true},
		{"[a-c].md", "d.md", false},
		{"[a-].md", "-.md", true},
		{"[-a].md", "-.md", true},
		{"[]].md", "].md", true},
		{"[!]].md", "a.md", true},
		{"[!]].md", "].md", false},
		{"[c-a].md", "b.md", false},
		{"[^a].md", "^.md", true},
		{"[^a].md", "b.md", false},
		{"[/].md", "/.md", true},
		{"[.md", "[.md", true},
		{"a[.md", "a[.md", true},
		{"[].md", "[].md", true},
		{"*[", "x/[", true},
		{"[é]x", "éx", true},
		{"?x", "éx", true},
		{"**", "anything/at/all.md", true},
		{"a*b*c", "axxbyyc", true},
		{"a*b*c", "axxbyy", false},
		{"*a", "ba", true},
		{"*a", "ab", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+" "+tt.path, func(t *testing.T) {
			p, err := ParsePath(tt.pattern)
			if err != nil {
				t.Fatal(err)
			}
			if got := p.Match(tt.path); got != tt.want {
				t.Errorf("Match = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPathWholeVault(t *testing.T) {
	for _, s := range []string{".", "./", "././", ".//", "/./", "/.", "./."} {
		p, err := ParsePath(s)
		if err != nil {
			t.Errorf("ParsePath(%q): %v", s, err)
			continue
		}
		for _, rel := range []string{"a.md", "x/y/z.md", ".hidden.md"} {
			if !p.Match(rel) {
				t.Errorf("ParsePath(%q).Match(%q) = false", s, rel)
			}
		}
	}
	// Not the whole vault: these keep matching as prefixes or globs.
	for _, tt := range []struct{ pattern, path string }{
		{"..", "a.md"},
		{".a", "a.md"},
		{"./a", "b.md"},
		{"?", "ab.md"},
	} {
		p, err := ParsePath(tt.pattern)
		if err != nil {
			t.Fatal(err)
		}
		if p.Match(tt.path) {
			t.Errorf("ParsePath(%q).Match(%q) = true", tt.pattern, tt.path)
		}
	}
}

func TestTrimPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"journal", "journal"},
		{"./journal/", "journal"},
		{"././a/b/", "a/b"},
		{"//./a", "a"},
		{".", "."},
		{"..", ".."},
		{"./", ""},
		{".a", ".a"},
		{"a/./b", "a/./b"},
	}
	for _, tt := range tests {
		if got := trimPath(tt.in); got != tt.want {
			t.Errorf("trimPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPathNFC(t *testing.T) {
	tests := []struct {
		pattern, path string
		want          bool
	}{
		{"caf\u00e9", "cafe\u0301/a.md", true},
		{"cafe\u0301", "caf\u00e9/a.md", true},
		{"cafe\u0301", "cafe\u0301/a.md", true},
		{"./notas/cafe\u0301/", "notas/caf\u00e9/x.md", true},
		{"cafe", "cafe\u0301/a.md", false},
		{"caf?/*.md", "cafe\u0301/a.md", true}, // ? matches é as one character
		{"caf\u00e9/*.md", "cafe\u0301/a.md", true},
		{"[\u00e9]*", "e\u0301x.md", true},
		{"Caf\u00e9", "cafe\u0301/a.md", false}, // still case-sensitive
	}
	for _, tt := range tests {
		p, err := ParsePath(tt.pattern)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.Match(tt.path); got != tt.want {
			t.Errorf("ParsePath(%q).Match(%q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
		}
	}
}
