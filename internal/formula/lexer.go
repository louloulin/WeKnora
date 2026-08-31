package formula

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Lexer converts a formula source string into a stream of tokens.
// It is a single-pass scanner with no backtracking.
//
// Whitespace, line breaks, and C-style // line comments are skipped.
//
// Single-character tokens: ( ) , . + - * / %
// Two-character tokens: == != <= >= && ||
// Multi-character: identifiers, numbers (integer / float), strings
// (single or double quoted), and the field reference prefix "$".
type Lexer struct {
	src    string
	pos    int
	tokens []Token
}

// NewLexer returns a lexer for src.
func NewLexer(src string) *Lexer { return &Lexer{src: src} }

// Tokenize performs the full tokenization. Errors are recorded as
// TokenIllegal tokens so the parser can report the offending offset
// in a single pass.
func (l *Lexer) Tokenize() ([]Token, error) {
	for l.pos < len(l.src) {
		ch := l.peek()
		switch {
		case unicode.IsSpace(ch):
			l.advance()
		case ch == '/' && l.peekAt(1) == '/':
			l.skipLineComment()
		case ch == '(' || ch == ')' || ch == ',' || ch == '.' ||
			ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '%' ||
			ch == '[' || ch == ']':
			l.singleRune()
		case ch == '$':
			l.tokens = append(l.tokens, Token{Kind: TokenFieldRef, Text: "$", Pos: l.pos})
			l.advance()
		case ch == '=' && l.peekAt(1) == '=':
			l.tokens = append(l.tokens, Token{Kind: TokenEq, Text: "==", Pos: l.pos})
			l.advanceN(2)
		case ch == '!' && l.peekAt(1) == '=':
			l.tokens = append(l.tokens, Token{Kind: TokenNeq, Text: "!=", Pos: l.pos})
			l.advanceN(2)
		case ch == '<' && l.peekAt(1) == '=':
			l.tokens = append(l.tokens, Token{Kind: TokenLte, Text: "<=", Pos: l.pos})
			l.advanceN(2)
		case ch == '>' && l.peekAt(1) == '=':
			l.tokens = append(l.tokens, Token{Kind: TokenGte, Text: ">=", Pos: l.pos})
			l.advanceN(2)
		case ch == '&' && l.peekAt(1) == '&':
			l.tokens = append(l.tokens, Token{Kind: TokenAnd, Text: "&&", Pos: l.pos})
			l.advanceN(2)
		case ch == '|' && l.peekAt(1) == '|':
			l.tokens = append(l.tokens, Token{Kind: TokenOr, Text: "||", Pos: l.pos})
			l.advanceN(2)
		case ch == '<':
			l.tokens = append(l.tokens, Token{Kind: TokenLt, Text: "<", Pos: l.pos})
			l.advance()
		case ch == '>':
			l.tokens = append(l.tokens, Token{Kind: TokenGt, Text: ">", Pos: l.pos})
			l.advance()
		case ch == '!':
			l.tokens = append(l.tokens, Token{Kind: TokenNot, Text: "!", Pos: l.pos})
			l.advance()
		case ch == '"' || ch == '\'':
			if err := l.scanString(ch); err != nil {
				return nil, err
			}
		case isDigit(ch):
			l.scanNumber()
		case isIdentStart(ch):
			l.scanIdent()
		default:
			return nil, fmt.Errorf("formula: unexpected character %q at pos %d", ch, l.pos)
		}
	}
	l.tokens = append(l.tokens, Token{Kind: TokenEOF, Pos: l.pos})
	return l.tokens, nil
}

func (l *Lexer) peek() rune {
	if l.pos >= len(l.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.pos:])
	return r
}

func (l *Lexer) peekAt(offset int) rune {
	if l.pos+offset >= len(l.src) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(l.src[l.pos+offset:])
	return r
}

func (l *Lexer) advance() { l.advanceN(1) }

func (l *Lexer) advanceN(n int) {
	for i := 0; i < n && l.pos < len(l.src); i++ {
		_, size := utf8.DecodeRuneInString(l.src[l.pos:])
		l.pos += size
	}
}

func (l *Lexer) singleRune() {
	start := l.pos
	ch := l.peek()
	l.advance()
	var k TokenKind
	switch ch {
	case '(':
		k = TokenLParen
	case ')':
		k = TokenRParen
	case ',':
		k = TokenComma
	case '.':
		k = TokenDot
	case '+':
		k = TokenPlus
	case '-':
		k = TokenMinus
	case '*':
		k = TokenStar
	case '/':
		k = TokenSlash
	case '%':
		k = TokenPercent
	case '[':
		k = TokenLBracket
	case ']':
		k = TokenRBracket
	}
	l.tokens = append(l.tokens, Token{Kind: k, Text: string(ch), Pos: start})
}

func (l *Lexer) scanString(quote rune) error {
	start := l.pos
	l.advance() // opening quote
	var b strings.Builder
	for l.pos < len(l.src) {
		ch := l.peek()
		if ch == '\\' {
			l.advance()
			if l.pos >= len(l.src) {
				return fmt.Errorf("formula: unterminated escape at pos %d", start)
			}
			esc := l.peek()
			switch esc {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '\\':
				b.WriteByte('\\')
			case '"':
				b.WriteByte('"')
			case '\'':
				b.WriteByte('\'')
			default:
				b.WriteRune(esc)
			}
			l.advance()
			continue
		}
		if ch == quote {
			l.advance() // closing quote
			l.tokens = append(l.tokens, Token{
				Kind: TokenString,
				Text: b.String(),
				Str:  b.String(),
				Pos:  start,
			})
			return nil
		}
		b.WriteRune(ch)
		l.advance()
	}
	return fmt.Errorf("formula: unterminated string starting at pos %d", start)
}

func (l *Lexer) scanNumber() {
	start := l.pos
	var numStr strings.Builder
	sawDecimal := false
	sawExp := false
	for l.pos < len(l.src) {
		ch := l.peek()
		if isDigit(ch) {
			numStr.WriteRune(ch)
			l.advance()
			continue
		}
		if ch == '.' && !sawDecimal && !sawExp {
			sawDecimal = true
			numStr.WriteRune(ch)
			l.advance()
			continue
		}
		if (ch == 'e' || ch == 'E') && !sawExp {
			sawExp = true
			numStr.WriteRune(ch)
			l.advance()
			if l.peek() == '+' || l.peek() == '-' {
				numStr.WriteRune(l.peek())
				l.advance()
			}
			continue
		}
		break
	}
	val := parseFloat(numStr.String())
	l.tokens = append(l.tokens, Token{
		Kind: TokenNumber,
		Text: numStr.String(),
		Num:  val,
		Pos:  start,
	})
}

func (l *Lexer) scanIdent() {
	start := l.pos
	var b strings.Builder
	for l.pos < len(l.src) {
		ch := l.peek()
		if isIdentPart(ch) {
			b.WriteRune(ch)
			l.advance()
		} else {
			break
		}
	}
	text := b.String()
	lower := strings.ToLower(text)
	if lower == "true" || lower == "false" {
		l.tokens = append(l.tokens, Token{
			Kind: TokenBool,
			Text: text,
			Bool: lower == "true",
			Pos:  start,
		})
		return
	}
	l.tokens = append(l.tokens, Token{
		Kind: TokenIdent,
		Text: text,
		Pos:  start,
	})
}

func (l *Lexer) skipLineComment() {
	for l.pos < len(l.src) && l.peek() != '\n' {
		l.advance()
	}
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isIdentStart(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r >= 0x80
}

func isIdentPart(r rune) bool {
	return isIdentStart(r) || isDigit(r)
}

// parseFloat avoids pulling in strconv to keep this file dependency-free.
func parseFloat(s string) float64 {
	var v float64
	var frac float64 = 1
	state := 0 // 0=int, 1=frac, 2=exp
	var exp int
	var expNeg bool
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '.':
			if state != 0 {
				return v
			}
			state = 1
		case 'e', 'E':
			if state == 2 {
				return v
			}
			state = 2
		case '+':
			if state != 2 {
				return v
			}
		case '-':
			if state == 2 {
				expNeg = true
			} else {
				return v
			}
		default:
			if c < '0' || c > '9' {
				return v
			}
			d := float64(c - '0')
			switch state {
			case 0:
				v = v*10 + d
			case 1:
				frac *= 10
				v += d / frac
			case 2:
				exp = exp*10 + int(d)
			}
		}
	}
	if state == 2 && exp > 0 {
		mul := 1.0
		for i := 0; i < exp; i++ {
			mul *= 10
		}
		if expNeg {
			v /= mul
		} else {
			v *= mul
		}
	}
	return v
}
