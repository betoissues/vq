package vault

import (
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// Note is a parsed note.
type Note struct {
	// Frontmatter is never nil; it is empty when missing or invalid.
	Frontmatter *Map
	// Tags are the frontmatter and inline tags, lowercased, deduplicated and
	// sorted by byte order.
	Tags []string
	// BadFrontmatter reports that the frontmatter was not a valid mapping.
	BadFrontmatter bool
	// TagsNotList reports that the tags key was not a list of scalars, or was
	// a scalar containing a comma or whitespace.
	TagsNotList bool
}

// Parse parses the contents of a note.
func Parse(data []byte) *Note {
	text := normalize(data)
	n := &Note{Frontmatter: NewMap()}
	yamlText, body, ok := splitFrontmatter(text)
	if ok {
		fm, err := parseYAML(yamlText)
		if err != nil {
			n.BadFrontmatter = true
		} else {
			n.Frontmatter = fm
		}
	}
	var fmTags []string
	if v, ok := n.Frontmatter.Get("tags"); ok {
		fmTags, n.TagsNotList = frontmatterTags(v)
	}
	n.Tags = mergeTags(fmTags, InlineTags(body))
	return n
}

// normalize decodes data as UTF-8, replacing invalid bytes with U+FFFD,
// normalizes line endings to \n and removes a leading BOM.
func normalize(data []byte) string {
	var s string
	if utf8.Valid(data) {
		s = string(data)
	} else {
		s = replaceInvalid(data)
	}
	if strings.ContainsRune(s, '\r') {
		s = strings.ReplaceAll(s, "\r\n", "\n")
		s = strings.ReplaceAll(s, "\r", "\n")
	}
	return strings.TrimPrefix(s, "\uFEFF")
}

func isDelimiter(line string) bool {
	return strings.TrimRight(line, " \t") == "---"
}

// splitFrontmatter returns the frontmatter text and the body. ok is false
// when the note has no frontmatter, in which case body is the whole text.
func splitFrontmatter(text string) (fm, body string, ok bool) {
	first, rest, found := strings.Cut(text, "\n")
	if !isDelimiter(first) || !found {
		return "", text, false
	}
	offset := 0
	for {
		line, after, more := strings.Cut(rest[offset:], "\n")
		if isDelimiter(line) {
			return rest[:offset], after, true
		}
		if !more {
			return "", text, false
		}
		offset += len(line) + 1
	}
}

// replaceInvalid copies data, writing U+FFFD for each byte that is not part
// of a valid UTF-8 sequence.
func replaceInvalid(data []byte) string {
	var b strings.Builder
	b.Grow(len(data) + len(data)/2)
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			b.WriteRune(utf8.RuneError)
		} else {
			b.Write(data[:size])
		}
		data = data[size:]
	}
	return b.String()
}

var errBadFrontmatter = errors.New("bad frontmatter")

// parseYAML parses frontmatter text, which must be a single mapping with
// unique scalar keys.
func parseYAML(text string) (*Map, error) {
	if strings.TrimSpace(text) == "" {
		return NewMap(), nil
	}
	dec := yaml.NewDecoder(strings.NewReader(text))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if err == io.EOF {
			// Only comments: no document at all.
			return NewMap(), nil
		}
		return nil, err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, errBadFrontmatter
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, errBadFrontmatter
	}
	root := resolve(doc.Content[0])
	if root.Kind != yaml.MappingNode {
		return nil, errBadFrontmatter
	}
	s := sizer{done: map[*yaml.Node]size{}, active: map[*yaml.Node]bool{}}
	if _, aliased, err := s.size(doc.Content[0]); err != nil || aliased > maxAliasNodes {
		return nil, errBadFrontmatter
	}
	c := converter{done: map[*yaml.Node]any{}, active: map[*yaml.Node]bool{}}
	v, err := c.convert(root)
	if err != nil {
		return nil, err
	}
	return v.(*Map), nil
}

func resolve(n *yaml.Node) *yaml.Node {
	for n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	return n
}

// converter turns yaml nodes into values. Nodes reached through several
// aliases are converted once; an alias cycle is an error.
type converter struct {
	done   map[*yaml.Node]any
	active map[*yaml.Node]bool
}

func (c *converter) convert(n *yaml.Node) (any, error) {
	n = resolve(n)
	if v, ok := c.done[n]; ok {
		return v, nil
	}
	if c.active[n] {
		return nil, errBadFrontmatter
	}
	c.active[n] = true
	defer delete(c.active, n)

	var v any
	switch n.Kind {
	case yaml.ScalarNode:
		if n.ShortTag() == "!!null" {
			v = nil
		} else {
			v = n.Value
		}
	case yaml.SequenceNode:
		list := make([]any, 0, len(n.Content))
		for _, item := range n.Content {
			iv, err := c.convert(item)
			if err != nil {
				return nil, err
			}
			list = append(list, iv)
		}
		v = list
	case yaml.MappingNode:
		m := NewMap()
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := resolve(n.Content[i])
			if k.Kind != yaml.ScalarNode {
				return nil, errBadFrontmatter
			}
			if _, dup := m.Get(k.Value); dup {
				return nil, errBadFrontmatter
			}
			iv, err := c.convert(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			m.Set(k.Value, iv)
		}
		v = m
	default:
		return nil, errBadFrontmatter
	}
	c.done[n] = v
	return v, nil
}

// maxAliasNodes is the largest number of nodes that aliases may expand to in
// one frontmatter. It guards against "billion laughs" notes, whose output
// would grow exponentially although the converted values are shared.
const maxAliasNodes = 10_000

// size is the expanded size of a node: all the nodes in its tree once aliases
// are replaced by their targets, and how many of those come from aliases.
type size struct {
	total, aliased int
}

// sizer computes expanded sizes. Counts saturate just above maxAliasNodes, so
// they cannot overflow; an alias cycle is an error.
type sizer struct {
	done   map[*yaml.Node]size
	active map[*yaml.Node]bool
}

func capped(n int) int {
	return min(n, maxAliasNodes+1)
}

// size returns the total and aliased node counts of n's expanded tree. Keys
// count as nodes. Each use of an alias adds the total size of its target.
func (s *sizer) size(n *yaml.Node) (total, aliased int, err error) {
	if sz, ok := s.done[n]; ok {
		return sz.total, sz.aliased, nil
	}
	if s.active[n] {
		return 0, 0, errBadFrontmatter
	}
	s.active[n] = true
	defer delete(s.active, n)

	if n.Kind == yaml.AliasNode {
		if n.Alias == nil {
			return 0, 0, errBadFrontmatter
		}
		t, _, err := s.size(n.Alias)
		if err != nil {
			return 0, 0, err
		}
		total, aliased = t, t
	} else {
		total = 1
		for _, c := range n.Content {
			t, a, err := s.size(c)
			if err != nil {
				return 0, 0, err
			}
			total, aliased = capped(total+t), capped(aliased+a)
		}
	}
	s.done[n] = size{total, aliased}
	return total, aliased, nil
}
