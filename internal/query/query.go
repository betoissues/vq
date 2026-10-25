// Package query implements the --tag, --where and --path filters.
package query

import (
	"fmt"
	"strings"

	"github.com/betoissues/vq/internal/vault"
)

// Query is a set of filters combined with AND. Path patterns are ORed among
// themselves.
type Query struct {
	Tags   []Tag
	Wheres []Where
	Paths  []Path
}

// MatchPath reports whether a vault-relative path matches the --path
// patterns. It is true when there are none.
func (q *Query) MatchPath(rel string) bool {
	if len(q.Paths) == 0 {
		return true
	}
	for _, p := range q.Paths {
		if p.Match(rel) {
			return true
		}
	}
	return false
}

// MatchNote reports whether a note matches every --tag and --where filter.
func (q *Query) MatchNote(n *vault.Note) bool {
	for _, t := range q.Tags {
		if !t.Match(n) {
			return false
		}
	}
	for _, w := range q.Wheres {
		if !w.Match(n.Frontmatter) {
			return false
		}
	}
	return true
}

// Tag is a --tag filter.
type Tag struct {
	tag string
}

// ParseTag parses the argument of --tag.
func ParseTag(s string) (Tag, error) {
	t := vault.NormalizeTag(strings.TrimRight(strings.TrimLeft(s, "#"), "/"))
	if t == "" {
		return Tag{}, fmt.Errorf("--tag: empty tag")
	}
	return Tag{tag: t}, nil
}

// Match reports whether the note has the tag or one of its subtags.
func (t Tag) Match(n *vault.Note) bool {
	for _, nt := range n.Tags {
		if nt == t.tag || strings.HasPrefix(nt, t.tag+"/") {
			return true
		}
	}
	return false
}
