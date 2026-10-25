package query

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/betoissues/vq/internal/vault"
	"golang.org/x/text/unicode/norm"
)

// Op is a --where operator.
type Op string

const (
	OpPresent Op = ""   // KEY
	OpAbsent  Op = "!"  // !KEY
	OpEq      Op = "="  // KEY=VALUE
	OpNe      Op = "!=" // KEY!=VALUE
	OpMatch   Op = "~"  // KEY~REGEX
	OpGt      Op = ">"
	OpLt      Op = "<"
	OpGe      Op = ">="
	OpLe      Op = "<="
)

// operators in the order they are tried, two-character ones first.
var operators = []Op{OpNe, OpGe, OpLe, OpEq, OpMatch, OpGt, OpLt}

const opChars = "!=~<>"

// Where is a --where filter.
type Where struct {
	Key   string
	Op    Op
	Value string
	lower string // Value in NFC, lowercased
	re    *regexp.Regexp
}

// ParseWhere parses the argument of --where.
func ParseWhere(expr string) (Where, error) {
	s := strings.TrimSpace(expr)
	if rest, ok := strings.CutPrefix(s, "!"); ok {
		if !strings.ContainsAny(rest, opChars) {
			key := strings.TrimSpace(rest)
			if key == "" {
				return Where{}, fmt.Errorf("--where %q: empty key", expr)
			}
			return Where{Key: key, Op: OpAbsent}, nil
		}
		if w, err := parseComparison(rest, expr); err == nil {
			if w.Op == OpEq {
				return Where{}, fmt.Errorf("--where %q: '!' cannot be combined with an operator; did you mean '%s!=%s'?", expr, w.Key, w.Value)
			}
			return Where{}, fmt.Errorf("--where %q: '!' cannot be combined with an operator", expr)
		}
		return Where{}, fmt.Errorf("--where %q: malformed expression", expr)
	}
	if !strings.ContainsAny(s, opChars) {
		if s == "" {
			return Where{}, fmt.Errorf("--where %q: empty key", expr)
		}
		return Where{Key: s, Op: OpPresent}, nil
	}
	return parseComparison(s, expr)
}

// parseComparison parses KEY OP VALUE. expr is the original argument, used in
// error messages.
func parseComparison(s, expr string) (Where, error) {
	i := strings.IndexAny(s, opChars)
	key := strings.TrimSpace(s[:i])
	if key == "" {
		return Where{}, fmt.Errorf("--where %q: empty key", expr)
	}
	rest := s[i:]
	var op Op
	for _, o := range operators {
		if strings.HasPrefix(rest, string(o)) {
			op = o
			break
		}
	}
	if op == "" {
		return Where{}, fmt.Errorf("--where %q: malformed expression", expr)
	}
	value := unquote(strings.TrimSpace(rest[len(op):]))
	w := Where{Key: key, Op: op, Value: value, lower: fold(value)}
	if op == OpMatch {
		re, err := regexp.Compile("(?i)" + norm.NFC.String(value))
		if err != nil {
			return Where{}, fmt.Errorf("--where %q: invalid regular expression: %v", expr, err)
		}
		w.re = re
	}
	return w, nil
}

// fold returns s in NFC, lowercased, for case-insensitive comparisons.
func fold(s string) string {
	return strings.ToLower(norm.NFC.String(s))
}

// unquote removes one matching pair of surrounding double or single quotes.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}

// present reports whether v counts as present: not null, not the empty string,
// not an empty list and not an empty map.
func present(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case *vault.Map:
		return v.Len() > 0
	}
	return false
}

// values returns the comparable values of a key: the string of a scalar, or
// the strings of the scalar items of a list (null items are the empty
// string).
func values(fm *vault.Map, key string) []string {
	v, ok := fm.Get(key)
	if !ok || !present(v) {
		return nil
	}
	switch v := v.(type) {
	case string:
		return []string{v}
	case []any:
		var out []string
		for _, item := range v {
			switch item := item.(type) {
			case nil:
				out = append(out, "")
			case string:
				out = append(out, item)
			}
		}
		return out
	}
	return nil
}

// Match reports whether the frontmatter satisfies the condition.
func (w Where) Match(fm *vault.Map) bool {
	switch w.Op {
	case OpPresent, OpAbsent:
		v, ok := fm.Get(w.Key)
		return (ok && present(v)) == (w.Op == OpPresent)
	case OpNe:
		for _, v := range values(fm, w.Key) {
			if fold(v) == w.lower {
				return false
			}
		}
		return true
	}
	for _, v := range values(fm, w.Key) {
		if w.compare(v) {
			return true
		}
	}
	return false
}

func (w Where) compare(v string) bool {
	if w.Op == OpMatch {
		return w.re.MatchString(norm.NFC.String(v))
	}
	lv := fold(v)
	switch w.Op {
	case OpEq:
		return lv == w.lower
	case OpGt:
		return lv > w.lower
	case OpLt:
		return lv < w.lower
	case OpGe:
		return lv >= w.lower
	case OpLe:
		return lv <= w.lower
	}
	return false
}
