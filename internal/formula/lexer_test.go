package formula

import (
	"testing"
)

func TestLexer_Numbers(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"0", 0},
		{"42", 42},
		{"3.14", 3.14},
		{"1e3", 1000},
		{"1.5e2", 150},
	}
	for _, c := range cases {
		toks, err := NewLexer(c.in).Tokenize()
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if len(toks) < 1 || toks[0].Kind != TokenNumber {
			t.Fatalf("%s: not a number token", c.in)
		}
		if toks[0].Num != c.want {
			t.Errorf("%s: got %v want %v", c.in, toks[0].Num, c.want)
		}
	}
}

// Unary minus is handled at the parser layer: -7 tokenises as MINUS + NUMBER(7).
func TestLexer_UnaryMinusTokens(t *testing.T) {
	toks, err := NewLexer("-7").Tokenize()
	if err != nil {
		t.Fatal(err)
	}
	if toks[0].Kind != TokenMinus || toks[1].Kind != TokenNumber || toks[1].Num != 7 {
		t.Errorf("got %+v", toks)
	}
}

func TestLexer_Strings(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`"hello"`, "hello"},
		{`'world'`, "world"},
		{`"with \"quotes\""`, `with "quotes"`},
		{`'line\nbreak'`, "line\nbreak"},
		{`""`, ""},
	}
	for _, c := range cases {
		toks, err := NewLexer(c.in).Tokenize()
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if len(toks) < 1 || toks[0].Kind != TokenString {
			t.Fatalf("%s: not a string token", c.in)
		}
		if toks[0].Str != c.want {
			t.Errorf("%s: got %q want %q", c.in, toks[0].Str, c.want)
		}
	}
}

func TestLexer_Operators(t *testing.T) {
	toks, err := NewLexer("+ - * / % ( ) , . == != < <= > >= && || ! $").Tokenize()
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	want := []TokenKind{
		TokenPlus, TokenMinus, TokenStar, TokenSlash, TokenPercent,
		TokenLParen, TokenRParen, TokenComma, TokenDot,
		TokenEq, TokenNeq, TokenLt, TokenLte, TokenGt, TokenGte,
		TokenAnd, TokenOr, TokenNot, TokenFieldRef,
		TokenEOF,
	}
	if len(toks) != len(want) {
		t.Fatalf("got %d tokens want %d", len(toks), len(want))
	}
	for i, k := range want {
		if toks[i].Kind != k {
			t.Errorf("token %d: got %s want %s", i, toks[i].Kind, k)
		}
	}
}

func TestLexer_Idents(t *testing.T) {
	toks, err := NewLexer("sum count avg max if").Tokenize()
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if len(toks) != 6 {
		t.Fatalf("got %d tokens", len(toks))
	}
	for i := 0; i < 5; i++ {
		if toks[i].Kind != TokenIdent {
			t.Errorf("token %d kind = %s", i, toks[i].Kind)
		}
	}
	if toks[5].Kind != TokenEOF {
		t.Errorf("last token kind = %s", toks[5].Kind)
	}
}

func TestLexer_Bools(t *testing.T) {
	toks, err := NewLexer("true false TRUE False").Tokenize()
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if toks[0].Kind != TokenBool || !toks[0].Bool {
		t.Errorf("token 0: kind=%s bool=%v", toks[0].Kind, toks[0].Bool)
	}
	if toks[1].Kind != TokenBool || toks[1].Bool {
		t.Errorf("token 1: kind=%s bool=%v", toks[1].Kind, toks[1].Bool)
	}
	// Case-insensitive: TRUE / False should also tokenise as Bool.
	if toks[2].Kind != TokenBool || !toks[2].Bool {
		t.Errorf("token 2: kind=%s", toks[2].Kind)
	}
	if toks[3].Kind != TokenBool || toks[3].Bool {
		t.Errorf("token 3: kind=%s", toks[3].Kind)
	}
}

func TestLexer_FieldRef(t *testing.T) {
	toks, err := NewLexer("$price * 1.1").Tokenize()
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	if toks[0].Kind != TokenFieldRef {
		t.Errorf("token 0: kind=%s", toks[0].Kind)
	}
	if toks[1].Kind != TokenIdent || toks[1].Text != "price" {
		t.Errorf("token 1: %+v", toks[1])
	}
}

func TestLexer_Comments(t *testing.T) {
	toks, err := NewLexer("sum  // skip me\n price").Tokenize()
	if err != nil {
		t.Fatalf("tokenize: %v", err)
	}
	// Should be: sum, price, EOF
	if len(toks) != 3 {
		t.Fatalf("got %d tokens: %+v", len(toks), toks)
	}
	if toks[0].Kind != TokenIdent || toks[0].Text != "sum" {
		t.Errorf("token 0: %+v", toks[0])
	}
	if toks[1].Kind != TokenIdent || toks[1].Text != "price" {
		t.Errorf("token 1: %+v", toks[1])
	}
}

func TestLexer_Error(t *testing.T) {
	_, err := NewLexer("a @ b").Tokenize()
	if err == nil {
		t.Fatal("expected error on illegal character @")
	}
}

func TestLexer_UnterminatedString(t *testing.T) {
	_, err := NewLexer(`"abc`).Tokenize()
	if err == nil {
		t.Fatal("expected error on unterminated string")
	}
}
