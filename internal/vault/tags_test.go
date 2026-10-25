package vault

import (
	"slices"
	"testing"
)

func TestFrontmatterTags(t *testing.T) {
	tests := []struct {
		name string
		in   string
		tags []string
		warn bool
	}{
		{"list", "tags: [a, B, '#c', '##d']", []string{"a", "b", "c", "d"}, false},
		{"block list", "tags:\n  - x\n  - y/z", []string{"x", "y/z"}, false},
		{"null items", "tags: [a, ~, '']", []string{"a"}, false},
		{"scalar", "tags: Project", []string{"project"}, false},
		{"scalar with comma", "tags: a, b", []string{"a, b"}, true},
		{"scalar with space", "tags: a b", []string{"a b"}, true},
		{"null", "tags:", []string{}, false},
		{"empty string", "tags: ''", []string{}, false},
		{"only hashes", "tags: '##'", []string{}, false},
		{"map", "tags: {a: 1}", []string{}, true},
		{"non-scalar items", "tags: [a, [b], {c: 1}]", []string{"a"}, true},
		{"duplicates", "tags: [a, A, '#a']", []string{"a"}, false},
		{"digits only", "tags: [2024, x, '#2024', 2024-01, v2]", []string{"2024-01", "v2", "x"}, false},
		{"digits only scalar", "tags: 2024", []string{}, false},
		{"digits only quoted", "tags: ['007']", []string{}, false},
		{"unicode decimal digits", "tags: [٢٠٢٤, ²]", []string{"²"}, false},
		{"spec example", "tags: [2024, 2025-, x]", []string{"x"}, false},
		{"trailing dash and slash", "tags: [idea-, a/b/, c-/-, '#d/']", []string{"a/b", "c", "d", "idea"}, false},
		{"only dashes and slashes", "tags: ['-', /, '#-/']", []string{}, false},
		{"inner dash and slash kept", "tags: [a-b, c/d]", []string{"a-b", "c/d"}, false},
		{"trimmed scalar", "tags: Idea/", []string{"idea"}, false},
		{"trimmed dedup", "tags: [x, x-, '#x/']", []string{"x"}, false},
		{"other key", "Tags: [a]", []string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := Parse([]byte("---\n" + tt.in + "\n---\n"))
			if n.BadFrontmatter {
				t.Fatal("bad frontmatter")
			}
			if !slices.Equal(n.Tags, tt.tags) {
				t.Errorf("tags = %q, want %q", n.Tags, tt.tags)
			}
			if n.TagsNotList != tt.warn {
				t.Errorf("TagsNotList = %v, want %v", n.TagsNotList, tt.warn)
			}
		})
	}
}

func TestInlineTags(t *testing.T) {
	tests := []struct {
		name string
		body string
		tags []string
	}{
		{"simple", "a #one and #Two", []string{"one", "Two"}},
		{"line start", "#x\n#y", []string{"x", "y"}},
		{"nested", "#a/b/c", []string{"a/b/c"}},
		{"chars", "#a_b-c/d1", []string{"a_b-c/d1"}},
		{"start with underscore or number", "#_x #1a", []string{"_x", "1a"}},
		{"unicode", "#Cuál #日本", []string{"Cuál", "日本"}},
		{"punctuation ends tag", "(#x), #y. #z!", []string{"x", "y", "z"}},
		{"trailing dash and slash", "#idea- #a/ #b-/-", []string{"idea", "a", "b"}},
		{"digits only", "#2024 #2024-01 #v2", []string{"2024-01", "v2"}},
		{"digits after trim", "#123-", nil},
		{"unicode decimal digits", "#٢٠٢٤ #²", []string{"²"}},
		{"url fragment", "http://x.com/page#frag", nil},
		{"after letter", "a#b", nil},
		{"after number", "1#b", nil},
		{"after underscore", "_#b", nil},
		{"after ampersand", "&#123; &#x", nil},
		{"double hash", "##x", nil},
		{"heading", "# Title\n## Sub", nil},
		{"after dash", "-#x", []string{"x"}},
		{"after tag", "#a#b", []string{"a"}},
		{"after tag ending in dash", "#a-#b", []string{"a", "b"}},
		{"bad start", "#-x #/x #", nil},
		{"inline code", "`#no` #yes `x` #also", []string{"yes", "also"}},
		{"unclosed inline code", "`#yes", []string{"yes"}},
		{"inline code removed, not replaced", "a`x`#b", nil},
		{"double backtick span with backtick inside", "``#no`#no`` #yes", []string{"yes"}},
		{"closer must have same length", "`#no``#no` #yes", []string{"yes"}},
		{"longer run inside is content", "``#no```#no`` #yes", []string{"yes"}},
		{"unmatched run is literal", "``#yes` #also", []string{"yes", "also"}},
		{"unmatched run, later span", "``#a `#no` #b", []string{"a", "b"}},
		{"span inside line", "a ```#no``` #yes", []string{"yes"}},
		{"empty span", "``#yes", []string{"yes"}},
		{"span does not cross lines", "`#a\n#b`", []string{"a", "b"}},
		{"several spans", "`#no` #a ``#no`` #b `#no`", []string{"a", "b"}},
		{"escaped backtick cannot open", "\\`#a` #b", []string{"a", "b"}},
		{"escaped backtick, later span", "\\` `#no` #yes", []string{"yes"}},
		{"escape shortens run", "\\``#no` #yes", []string{"yes"}},
		{"backslash inside span escapes nothing", "`#no\\` #yes", []string{"yes"}},
		{"escaped backslash before backtick", "\\\\`#no` #yes", []string{"yes"}},
		{"backslash before other char", "\\a`#no` #yes", []string{"yes"}},
		{"escaped hash", "\\#no #yes", []string{"yes"}},
		{"escaped backslash before hash", "\\\\#yes", []string{"yes"}},
		{"three backslashes", "\\\\\\#no", nil},
		{"escaped hash then hash", "\\##no", nil},
		{"escaped ampersand then hash", "\\&#no", nil},
		{"escape ends a tag", "#a\\#b", []string{"a"}},
		{"backslash before letter", "\\a#no \\ #yes", []string{"yes"}},
		{"escaped hash at line start", "\\#no\n#yes", []string{"yes"}},
		{"fenced", "```\n#no\n```\n#yes", []string{"yes"}},
		{"fenced tilde with info", "~~~ md\n#no\n~~~\n#yes", []string{"yes"}},
		{"indented fence", "- item\n    ```\n    #no\n    ```\n#yes", []string{"yes"}},
		{"longer closing fence", "```\n#no\n`````\n#yes", []string{"yes"}},
		{"shorter fence does not close", "````\n```\n#no\n````\n#yes", []string{"yes"}},
		{"other char does not close", "```\n~~~\n#no\n```\n#yes", []string{"yes"}},
		{"closing fence with text does not close", "```\n``` x\n#no\n```\n#yes", []string{"yes"}},
		{"unclosed fence", "#yes\n```\n#no", []string{"yes"}},
		{"two backticks not a fence", "``\n#yes", []string{"yes"}},
		{"backtick in info string not a fence", "``` a`b\n#yes", []string{"yes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InlineTags(tt.body); !slices.Equal(got, tt.tags) {
				t.Errorf("InlineTags(%q) = %q, want %q", tt.body, got, tt.tags)
			}
		})
	}
}

func TestTagsUnion(t *testing.T) {
	n := Parse([]byte("---\ntags: [b, A]\n---\n#a #c #B\n"))
	if want := []string{"a", "b", "c"}; !slices.Equal(n.Tags, want) {
		t.Errorf("tags = %q, want %q", n.Tags, want)
	}
}

func TestInlineTagsCommentsAndLinks(t *testing.T) {
	tests := []struct {
		name string
		body string
		tags []string
	}{
		{"obsidian comment", "%% #no %% #yes", []string{"yes"}},
		{"html comment", "<!-- #no --> #yes", []string{"yes"}},
		{"multi-line obsidian comment", "#a\n%%\n#no\n%%\n#b", []string{"a", "b"}},
		{"multi-line html comment", "#a <!-- x\n#no\n--> #b", []string{"a", "b"}},
		{"unclosed obsidian comment", "#a %% #no\n#no", []string{"a"}},
		{"unclosed html comment", "#a <!-- #no\n#no", []string{"a"}},
		{"comment deleted, not replaced", "a%%x%%#b", nil},
		{"several comments", "%%#no%% #a <!--#no--> #b %%#no%%", []string{"a", "b"}},
		{"html closer does not close obsidian", "%% --> #no %% #yes", []string{"yes"}},
		{"obsidian closer inside html", "<!-- %% #no --> #yes %%#no%%", []string{"yes"}},
		{"comment markers in inline code", "`%%` #yes `%%`", []string{"yes"}},
		{"comment markers in fence", "```\n<!--\n```\n#yes", []string{"yes"}},
		{"code removed before comments", "%% `%%` #no %% #yes", []string{"yes"}},

		{"wikilink heading", "See [[#Intro]] #yes", []string{"yes"}},
		{"wikilink with display text", "[[Note#Part|#x]] #yes", []string{"yes"}},
		{"wikilink without hash", "[[Other note]] #yes", []string{"yes"}},
		{"embed", "![[img.png#frag]] #yes", []string{"yes"}},
		{"unclosed wikilink", "[[#a #b", []string{"a", "b"}},
		{"wikilink does not span lines", "[[#a\n#b]]", []string{"a", "b"}},
		{"markdown link destination", "[top](#summary) #yes", []string{"yes"}},
		{"markdown link text kept", "[see #x](#anchor)", []string{"x"}},
		{"image destination", "![#alt](pic.png#frag)", []string{"alt"}},
		{"unclosed destination", "[a](#b", []string{"b"}},
		{"destination does not span lines", "[a](\n#b)", []string{"b"}},
		{"bracket alone does not block", "[#a] (#b)", []string{"a", "b"}},
		{"several links", "[[#no]] [a](#no) #c [[x]] [d](e#no)", []string{"c"}},
		{"link in inline code", "`[[` #a `]]`", []string{"a"}},
		{"link in comment", "%% [[#no]] %% #yes", []string{"yes"}},
		{"link deleted, not replaced", "x[[y]]#b", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InlineTags(tt.body); !slices.Equal(got, tt.tags) {
				t.Errorf("InlineTags(%q) = %q, want %q", tt.body, got, tt.tags)
			}
		})
	}
}

func TestInlineTagsCombiningMarks(t *testing.T) {
	tests := []struct {
		name string
		body string
		tags []string
	}{
		{"decomposed accent", "#cafe\u0301 #ok", []string{"cafe\u0301", "ok"}},
		{"precomposed accent", "#caf\u00e9", []string{"caf\u00e9"}},
		{"mark in the middle", "#Cua\u0301l", []string{"Cua\u0301l"}},
		{"mark cannot start a tag", "#\u0301x", nil},
		{"several marks", "#a\u0301\u0308b", []string{"a\u0301\u0308b"}},
		{"decomposed letter before hash", "cafe\u0301#x", nil},
		{"precomposed letter before hash", "caf\u00e9#x", nil},
		{"mark alone before hash", "\u0301#x", nil},
		{"mark ending a tag blocks the next", "#a\u0301#b", []string{"a\u0301"}},
		{"escaped mark is not an escape", "\\\u0301#x", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InlineTags(tt.body); !slices.Equal(got, tt.tags) {
				t.Errorf("InlineTags(%q) = %q, want %q", tt.body, got, tt.tags)
			}
		})
	}
	// The decomposed form no longer collides with the plain one, and note
	// tags come out in NFC.
	n := Parse([]byte("#cafe\u0301 #cafe"))
	if want := []string{"cafe", "caf\u00e9"}; !slices.Equal(n.Tags, want) {
		t.Errorf("tags = %q, want %q", n.Tags, want)
	}
}

func TestNormalizeTag(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Idea", "idea"},
		{"Cafe\u0301", "caf\u00e9"},
		{"CAF\u00c9", "caf\u00e9"},
		{"CAFE\u0301", "caf\u00e9"},
		{"A\u030a", "\u00e5"},             // Å decomposed
		{"\u212b", "\u00e5"},              // ANGSTROM SIGN
		{"x\u0323\u0307", "\u1e8b\u0323"}, // x and dot above compose
		{"x\u0307\u0323", "\u1e8b\u0323"}, // other mark order, same NFC
		{"\u0130", "i"},                   // İ: Go lowercases to plain i
		{"\u1e9e", "\u00df"},              // capital sharp s
	}
	for _, tt := range tests {
		if got := NormalizeTag(tt.in); got != tt.want {
			t.Errorf("NormalizeTag(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestTagsNFC(t *testing.T) {
	tests := []struct {
		name string
		note string
		tags []string
	}{
		{"inline nfd", "#cafe\u0301", []string{"caf\u00e9"}},
		{"frontmatter nfd", "---\ntags: [cafe\u0301]\n---\n", []string{"caf\u00e9"}},
		{"nfd and nfc deduplicated", "---\ntags: [caf\u00e9]\n---\n#Cafe\u0301 #CAF\u00c9", []string{"caf\u00e9"}},
		{"subtag nfd", "#proyecto/acci\u00f3n #proyecto/accio\u0301n", []string{"proyecto/acci\u00f3n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Parse([]byte(tt.note)).Tags; !slices.Equal(got, tt.tags) {
				t.Errorf("tags = %q, want %q", got, tt.tags)
			}
		})
	}
}

func TestFrontmatterNotNormalized(t *testing.T) {
	n := Parse([]byte("---\ntitle: Cafe\u0301\ntags: [cafe\u0301]\n---\n"))
	want := "{\"title\":\"Cafe\u0301\",\"tags\":[\"cafe\u0301\"]}"
	if got := string(AppendJSON(nil, n.Frontmatter)); got != want {
		t.Errorf("frontmatter = %s, want %s", got, want)
	}
}
