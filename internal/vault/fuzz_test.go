package vault

import (
	"encoding/json"
	"testing"
)

func FuzzParse(f *testing.F) {
	for _, seed := range []string{
		"",
		"plain #tag body",
		"---\ntitle: x\ntags: [a, '#b/c-']\n---\n#d `#e` %% #f %% [[#g]] [h](#i) \\#j",
		"---\na: &a [x, x]\nb: [*a, *a]\nm: {k: [1, ~, {z: y}]}\n---\n",
		"---\na: &x [*x]\n---\n",
		"---\n- list\n---\n```\n#no\n```\n<!-- #no",
		"\uFEFF---\r\na: \"\\t<&>\"\r\n---\r\n\xff#x",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		n := Parse(data)
		if n.Frontmatter == nil {
			t.Fatal("nil frontmatter")
		}
		if n.BadFrontmatter && n.Frontmatter.Len() != 0 {
			t.Fatal("bad frontmatter is not empty")
		}
		out := AppendJSON(nil, n.Frontmatter)
		if !json.Valid(out) {
			t.Fatalf("invalid JSON: %s", out)
		}
		for i, tag := range n.Tags {
			if tag == "" || digitsOnly(tag) {
				t.Fatalf("bad tag %q", tag)
			}
			if i > 0 && n.Tags[i-1] >= tag {
				t.Fatalf("tags not sorted and unique: %q", n.Tags)
			}
		}
	})
}
