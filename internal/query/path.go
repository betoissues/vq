package query

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Path is a --path filter: a prefix or, if it contains '*', '?' or '[', a
// glob over the whole vault-relative path. "." matches the whole vault.
type Path struct {
	all    bool // "." or "./": every path
	prefix string
	glob   []globToken // nil for a prefix
}

// ParsePath parses the argument of --path. The pattern is normalized to NFC.
func ParsePath(s string) (Path, error) {
	p := norm.NFC.String(trimPath(s))
	if p == "." || (p == "" && strings.Contains(s, ".")) {
		return Path{all: true}, nil
	}
	if p == "" {
		return Path{}, fmt.Errorf("--path %q: empty path", s)
	}
	if strings.ContainsAny(p, "*?[") {
		return Path{glob: compileGlob(p)}, nil
	}
	return Path{prefix: p}, nil
}

// trimPath removes leading "./" segments and leading and trailing slashes, so
// "./journal/" is "journal".
func trimPath(s string) string {
	for {
		s = strings.TrimLeft(s, "/")
		rest, ok := strings.CutPrefix(s, "./")
		if !ok {
			return strings.TrimRight(s, "/")
		}
		s = rest
	}
}

// Match reports whether the vault-relative path rel, normalized to NFC,
// matches.
func (p Path) Match(rel string) bool {
	if p.all {
		return true
	}
	rel = norm.NFC.String(rel)
	if p.glob != nil {
		return matchGlob(p.glob, rel)
	}
	return rel == p.prefix || strings.HasPrefix(rel, p.prefix+"/")
}

type globKind int

const (
	globLiteral globKind = iota
	globAny              // ?
	globStar             // *
	globClass            // [...]
)

type runeRange struct{ lo, hi rune }

type globToken struct {
	kind   globKind
	r      rune // globLiteral
	negate bool // globClass
	ranges []runeRange
}

// compileGlob parses a pattern with the semantics of Python's fnmatch: '*' and
// '?' also match '/', and an unclosed '[' is a literal character.
func compileGlob(pat string) []globToken {
	p := []rune(pat)
	var toks []globToken
	for i := 0; i < len(p); i++ {
		switch c := p[i]; c {
		case '*':
			if len(toks) == 0 || toks[len(toks)-1].kind != globStar {
				toks = append(toks, globToken{kind: globStar})
			}
		case '?':
			toks = append(toks, globToken{kind: globAny})
		case '[':
			j := i + 1
			if j < len(p) && p[j] == '!' {
				j++
			}
			if j < len(p) && p[j] == ']' {
				j++
			}
			for j < len(p) && p[j] != ']' {
				j++
			}
			if j >= len(p) {
				toks = append(toks, globToken{kind: globLiteral, r: c})
				continue
			}
			toks = append(toks, parseClass(p[i+1:j]))
			i = j
		default:
			toks = append(toks, globToken{kind: globLiteral, r: c})
		}
	}
	return toks
}

// parseClass parses the contents of a character class, without the brackets.
func parseClass(body []rune) globToken {
	t := globToken{kind: globClass}
	if len(body) > 0 && body[0] == '!' {
		t.negate = true
		body = body[1:]
	}
	for k := 0; k < len(body); {
		if k+2 < len(body) && body[k+1] == '-' {
			t.ranges = append(t.ranges, runeRange{body[k], body[k+2]})
			k += 3
		} else {
			t.ranges = append(t.ranges, runeRange{body[k], body[k]})
			k++
		}
	}
	return t
}

func (t globToken) matchRune(r rune) bool {
	switch t.kind {
	case globLiteral:
		return r == t.r
	case globAny:
		return true
	case globClass:
		in := false
		for _, rg := range t.ranges {
			if rg.lo <= r && r <= rg.hi {
				in = true
				break
			}
		}
		return in != t.negate
	}
	return false
}

// matchGlob matches the whole string s against the tokens, backtracking only
// to the last '*'.
func matchGlob(toks []globToken, s string) bool {
	ti, si := 0, 0
	starTok, starStr := -1, 0
	for si < len(s) || ti < len(toks) {
		if ti < len(toks) {
			t := toks[ti]
			if t.kind == globStar {
				starTok, starStr = ti, si
				ti++
				continue
			}
			if si < len(s) {
				r, size := utf8.DecodeRuneInString(s[si:])
				if t.matchRune(r) {
					ti++
					si += size
					continue
				}
			}
		}
		if starTok < 0 || starStr >= len(s) {
			return false
		}
		_, size := utf8.DecodeRuneInString(s[starStr:])
		starStr += size
		ti, si = starTok+1, starStr
	}
	return true
}
