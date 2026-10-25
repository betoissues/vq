package vault

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// frontmatterTags returns the tags in the value of the tags key, not yet
// lowercased. warn reports that the value should have been a list.
func frontmatterTags(v any) (tags []string, warn bool) {
	switch v := v.(type) {
	case nil:
	case string:
		if strings.ContainsFunc(v, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
			warn = true
		}
		tags = append(tags, v)
	case []any:
		for _, item := range v {
			switch item := item.(type) {
			case nil:
			case string:
				tags = append(tags, item)
			default:
				warn = true
			}
		}
	default:
		warn = true
	}
	return tags, warn
}

// NormalizeTag returns the form tags are compared and printed in: NFC, then
// lowercased. It is normalized to NFC again after lowercasing, which can
// leave a string that is not in NFC.
func NormalizeTag(t string) string {
	return norm.NFC.String(strings.ToLower(norm.NFC.String(t)))
}

// mergeTags strips leading '#' and trailing '-' and '/' from frontmatter
// tags, drops empty and digits-only ones, normalizes everything with
// NormalizeTag and returns the sorted, deduplicated union.
func mergeTags(fm, inline []string) []string {
	out := make([]string, 0, len(fm)+len(inline))
	for _, t := range fm {
		t = strings.TrimRight(strings.TrimLeft(t, "#"), "-/")
		if !digitsOnly(t) {
			out = append(out, NormalizeTag(t))
		}
	}
	for _, t := range inline {
		out = append(out, NormalizeTag(t))
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// InlineTags returns the inline tags in body, in order of appearance and not
// lowercased. Fenced code blocks and inline code are removed first, then
// comments, then link targets.
func InlineTags(body string) []string {
	lines := stripFences(body)
	for i, line := range lines {
		lines[i] = stripInlineCode(line)
	}
	text := stripComments(strings.Join(lines, "\n"))
	var tags []string
	for _, line := range strings.Split(text, "\n") {
		tags = appendLineTags(tags, stripLinks(line))
	}
	return tags
}

// stripComments deletes Obsidian comments (%% ... %%) and HTML comments
// (<!-- ... -->), which may span lines. An unclosed comment runs to the end.
func stripComments(s string) string {
	if !strings.Contains(s, "%%") && !strings.Contains(s, "<!--") {
		return s
	}
	var b strings.Builder
	for {
		open, closing := "%%", "%%"
		i := strings.Index(s, open)
		if h := strings.Index(s, "<!--"); h >= 0 && (i < 0 || h < i) {
			i, open, closing = h, "<!--", "-->"
		}
		if i < 0 {
			b.WriteString(s)
			break
		}
		b.WriteString(s[:i])
		rest := s[i+len(open):]
		end := strings.Index(rest, closing)
		if end < 0 {
			break
		}
		s = rest[end+len(closing):]
	}
	return b.String()
}

// stripLinks deletes, within one line, whole wikilinks ([[...]]) and the
// destinations of Markdown links (](...)), keeping a Markdown link's text.
func stripLinks(line string) string {
	if !strings.Contains(line, "[[") && !strings.Contains(line, "](") {
		return line
	}
	var b strings.Builder
	start := 0 // start of the text not yet copied
	for i := 0; i < len(line); {
		var open, closing string
		switch {
		case strings.HasPrefix(line[i:], "[["):
			open, closing = "[[", "]]"
		case strings.HasPrefix(line[i:], "]("):
			open, closing = "](", ")"
		default:
			i++
			continue
		}
		end := strings.Index(line[i+len(open):], closing)
		if end < 0 {
			i += len(open)
			continue
		}
		b.WriteString(line[start:i])
		i += len(open) + end + len(closing)
		start = i
	}
	b.WriteString(line[start:])
	return b.String()
}

// stripFences splits body into lines and returns those outside fenced code
// blocks.
func stripFences(body string) []string {
	var out []string
	var fenceChar byte
	var fenceLen int
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if fenceLen > 0 {
			if n := runLen(trimmed, fenceChar); n >= fenceLen && strings.Trim(trimmed[n:], " \t") == "" {
				fenceLen = 0
			}
			continue
		}
		if trimmed != "" && (trimmed[0] == '`' || trimmed[0] == '~') {
			c := trimmed[0]
			n := runLen(trimmed, c)
			// As in CommonMark, the info string of a backtick fence cannot
			// contain backticks.
			if n >= 3 && (c == '~' || !strings.ContainsRune(trimmed[n:], '`')) {
				fenceChar, fenceLen = c, n
				continue
			}
		}
		out = append(out, line)
	}
	return out
}

func runLen(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

// stripInlineCode deletes CommonMark code spans from a single line: a run of
// N backticks opens a span that ends at the next run of exactly N backticks.
// A run with no matching closer is literal text. Outside a span a backslash
// escapes the next ASCII punctuation character, as in CommonMark, so an
// escaped backtick cannot open a span (and \\` is an escaped backslash
// followed by a backtick); inside a span backslashes escape nothing.
func stripInlineCode(line string) string {
	if !strings.ContainsRune(line, '`') {
		return line
	}
	var b strings.Builder
	start := 0 // start of the text not yet copied
	for i := 0; i < len(line); {
		if line[i] == '\\' && i+1 < len(line) && isASCIIPunct(line[i+1]) {
			i += 2
			continue
		}
		if line[i] != '`' {
			i++
			continue
		}
		n := runLen(line[i:], '`')
		closing := -1
		for j := i + n; j < len(line); {
			if line[j] != '`' {
				j++
				continue
			}
			m := runLen(line[j:], '`')
			if m == n {
				closing = j
				break
			}
			j += m
		}
		if closing < 0 {
			i += n
			continue
		}
		b.WriteString(line[start:i])
		i = closing + n
		start = i
	}
	b.WriteString(line[start:])
	return b.String()
}

func isTagStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsNumber(r)
}

// isTagChar reports whether r can follow the first character of a tag.
// Combining marks are accepted so that decomposed text (an 'e' followed by
// U+0301) is not cut short.
func isTagChar(r rune) bool {
	return isTagStart(r) || unicode.IsMark(r) || r == '-' || r == '/'
}

// blocksTag reports whether a '#' preceded by r cannot start a tag.
func blocksTag(r rune) bool {
	return isTagStart(r) || unicode.IsMark(r) || r == '/' || r == '&' || r == '#'
}

func appendLineTags(tags []string, line string) []string {
	prev := utf8.RuneError
	start := true
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		if r == '\\' && i+1 < len(line) && isASCIIPunct(line[i+1]) {
			// A CommonMark escape: the next character is literal, so an
			// escaped '#' does not start a tag.
			prev, start = rune(line[i+1]), false
			i += 2
			continue
		}
		if r != '#' || (!start && blocksTag(prev)) {
			prev, start = r, false
			i += size
			continue
		}
		j := i + size
		first, fsize := utf8.DecodeRuneInString(line[j:])
		if j >= len(line) || !isTagStart(first) {
			prev, start = r, false
			i = j
			continue
		}
		j += fsize
		last := first
		for j < len(line) {
			c, csize := utf8.DecodeRuneInString(line[j:])
			if !isTagChar(c) {
				break
			}
			last = c
			j += csize
		}
		tag := strings.TrimRight(line[i+size:j], "-/")
		if !digitsOnly(tag) {
			tags = append(tags, tag)
		}
		prev, start = last, false
		i = j
	}
	return tags
}

// digitsOnly reports whether s is empty or made only of Unicode decimal
// digits. Such tags are ignored.
func digitsOnly(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsDigit(r) })
}

func isASCIIPunct(c byte) bool {
	return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0
}
