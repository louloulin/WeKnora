package scimsp

import (
	"fmt"
	"strings"
	"unicode"
)

// FilterKind discriminates the conjunction operators.
type FilterKind int

const (
	KindLeaf FilterKind = iota
	KindAnd
	KindOr
)

// Filter is a parsed SCIM filter expression. It is a binary tree:
// a leaf holds one comparison (attribute / op / value); an interior
// node holds the conjunction kind (and / or) and two children.
//
// RFC 7644 §3.4.2.2 grammar (the subset we accept):
//
//	filter      = attr op value ("and" filter)*
//	filter      =/ attr op value ("or" filter)*
//	attr        = 1*ALPHA *(ALPHA / "." / "-" / "_")
//	op          = "eq" / "ne" / "co" / "sw" / "pr"
//	value       = DQUOTE *value-char DQUOTE
type Filter struct {
	Kind  FilterKind
	Attr  string
	Op    string
	Value string
	Left  *Filter
	Right *Filter
}

// ParseFilter parses a SCIM 2.0 filter expression. Supports
// "and" / "or" (case-insensitive) plus eq, ne, co, sw, pr. Strings
// on the right-hand side must be double-quoted.
//
// Examples accepted:
//
//	userName eq "alice"
//	emails.value co "@corp.example.com" or active eq false
//	displayName pr
//
// Examples rejected: anything with parens (we do not implement
// grouping) or ge/gt/le/lt operators (not used by major IdPs).
func ParseFilter(s string) (*Filter, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	tokens, err := tokenize(s)
	if err != nil {
		return nil, err
	}
	p := &filterParser{tokens: tokens}
	f, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.tokens) {
		return nil, fmt.Errorf("%w: trailing tokens", ErrInvalidFilter)
	}
	return f, nil
}

type filterParser struct {
	tokens []string
	pos    int
}

func (p *filterParser) parseOr() (*Filter, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peekEq("or") {
		p.pos++
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &Filter{Kind: KindOr, Left: left, Right: right}
	}
	return left, nil
}

func (p *filterParser) parseAnd() (*Filter, error) {
	left, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	for p.peekEq("and") {
		p.pos++
		right, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		left = &Filter{Kind: KindAnd, Left: left, Right: right}
	}
	return left, nil
}

func (p *filterParser) parseAtom() (*Filter, error) {
	if p.pos >= len(p.tokens) {
		return nil, fmt.Errorf("%w: empty filter", ErrInvalidFilter)
	}
	attr := p.tokens[p.pos]
	p.pos++
	if p.pos >= len(p.tokens) {
		return nil, fmt.Errorf("%w: missing operator after %q", ErrInvalidFilter, attr)
	}
	op := strings.ToLower(p.tokens[p.pos])
	p.pos++
	switch op {
	case caseEq, caseNe, caseCo, caseSw:
		if p.pos >= len(p.tokens) {
			return nil, fmt.Errorf("%w: missing value after %s", ErrInvalidFilter, op)
		}
		v := p.tokens[p.pos]
		p.pos++
		return &Filter{Kind: KindLeaf, Attr: strings.ToLower(attr), Op: op, Value: v}, nil
	case casePr:
		return &Filter{Kind: KindLeaf, Attr: strings.ToLower(attr), Op: casePr}, nil
	default:
		if isConjunction(op) {
			return nil, fmt.Errorf("%w: unexpected %q", ErrInvalidFilter, op)
		}
		return nil, fmt.Errorf("%w: unsupported operator %q", ErrUnsupportedFilterOp, op)
	}
}

const (
	caseEq = "eq"
	caseNe = "ne"
	caseCo = "co"
	caseSw = "sw"
	casePr = "pr"
)

func (p *filterParser) peekEq(s string) bool {
	if p.pos >= len(p.tokens) {
		return false
	}
	return strings.EqualFold(p.tokens[p.pos], s)
}

func isConjunction(s string) bool {
	return s == "and" || s == "or"
}

// tokenize splits a filter string into attribute / operator / value
// tokens. Quoted values stay as a single token with internal
// spaces preserved; backslash escapes the next char.
func tokenize(s string) ([]string, error) {
	var out []string
	var i int
	for i < len(s) {
		ch := rune(s[i])
		switch {
		case unicode.IsSpace(ch):
			i++
		case ch == '"':
			j := i + 1
			var buf strings.Builder
			for j < len(s) {
				if s[j] == '\\' && j+1 < len(s) {
					buf.WriteByte(s[j+1])
					j += 2
					continue
				}
				if s[j] == '"' {
					break
				}
				buf.WriteByte(s[j])
				j++
			}
			if j >= len(s) {
				return nil, fmt.Errorf("%w: unterminated string", ErrInvalidFilter)
			}
			out = append(out, buf.String())
			i = j + 1
		default:
			j := i
			for j < len(s) && !unicode.IsSpace(rune(s[j])) && s[j] != '"' {
				j++
			}
			out = append(out, s[i:j])
			i = j
		}
	}
	return out, nil
}

// Match evaluates the filter against the supplied getter. The
// getter returns the attribute value (already lowercased) for a
// given dotted path, plus whether the attribute is present.
//
// The getter abstraction lets Match evaluate against either a
// User, a Group, or an arbitrary map without depending on the
// persistence model.
func (f *Filter) Match(get func(attr string) (value string, present bool)) bool {
	if f == nil {
		return true
	}
	switch f.Kind {
	case KindLeaf:
		return matchLeaf(f, get)
	case KindAnd:
		return f.Left.Match(get) && f.Right.Match(get)
	case KindOr:
		return f.Left.Match(get) || f.Right.Match(get)
	}
	return true
}

func matchLeaf(f *Filter, get func(string) (string, bool)) bool {
	if f.Op == casePr {
		_, present := get(f.Attr)
		return present
	}
	got, _ := get(f.Attr)
	switch f.Op {
	case caseEq:
		return got == f.Value
	case caseNe:
		return got != f.Value
	case caseCo:
		return strings.Contains(got, f.Value)
	case caseSw:
		return strings.HasPrefix(got, f.Value)
	}
	return false
}
