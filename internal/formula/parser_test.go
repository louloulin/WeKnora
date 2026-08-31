package formula

import "testing"

func parseOne(t *testing.T, src string) Node {
	t.Helper()
	toks, err := NewLexer(src).Tokenize()
	if err != nil {
		t.Fatalf("lexer: %v", err)
	}
	n, err := NewParser(toks).Parse()
	if err != nil {
		t.Fatalf("parser: %v", err)
	}
	return n
}

func TestParser_Literal(t *testing.T) {
	n := parseOne(t, "42")
	if n.Kind != NodeLiteral || n.Value.Kind != ValueNumber || n.Value.Num != 42 {
		t.Errorf("got %+v", n)
	}
}

func TestParser_BinaryPrecedence(t *testing.T) {
	// 1 + 2 * 3 should parse as 1 + (2 * 3)
	n := parseOne(t, "1 + 2 * 3")
	if n.Kind != NodeBinary || n.Op != "+" {
		t.Fatalf("root should be +, got %+v", n)
	}
	if n.LHS.Kind != NodeLiteral || n.LHS.Value.Num != 1 {
		t.Errorf("lhs: %+v", n.LHS)
	}
	if n.RHS.Kind != NodeBinary || n.RHS.Op != "*" {
		t.Errorf("rhs: %+v", n.RHS)
	}
}

func TestParser_UnaryMinus(t *testing.T) {
	n := parseOne(t, "-5")
	if n.Kind != NodeUnary || n.Op != "-" {
		t.Fatalf("not unary -, got %+v", n)
	}
	if n.Operand.Kind != NodeLiteral || n.Operand.Value.Num != 5 {
		t.Errorf("operand: %+v", n.Operand)
	}
}

func TestParser_FunctionCall(t *testing.T) {
	n := parseOne(t, `upper("hello")`)
	if n.Kind != NodeCall || n.Ident != "upper" {
		t.Fatalf("not call upper, got %+v", n)
	}
	if len(n.Args) != 1 || n.Args[0].Kind != NodeLiteral {
		t.Errorf("args: %+v", n.Args)
	}
}

func TestParser_IfExpr(t *testing.T) {
	n := parseOne(t, "if($x > 0, 1, -1)")
	if n.Kind != NodeIf {
		t.Fatalf("not if, got %+v", n)
	}
	if n.Then == nil || n.Else == nil {
		t.Fatalf("missing then/else")
	}
}

func TestParser_FieldRef(t *testing.T) {
	n := parseOne(t, "$price * 1.1")
	if n.Kind != NodeBinary || n.Op != "*" {
		t.Fatalf("root: %+v", n)
	}
	if n.LHS.Kind != NodeFieldRef || n.LHS.Chain[0] != "price" {
		t.Errorf("lhs not $price: %+v", n.LHS)
	}
}

func TestParser_DotChain(t *testing.T) {
	// $user.name should produce get($user, "name")
	n := parseOne(t, "$user.name")
	if n.Kind != NodeCall || n.Ident != "get" {
		t.Fatalf("root: %+v", n)
	}
}

func TestParser_List(t *testing.T) {
	n := parseOne(t, "[1, 2, 3]")
	if n.Kind != NodeList || len(n.Elems) != 3 {
		t.Fatalf("got %+v", n)
	}
}

func TestParser_EmptyList(t *testing.T) {
	n := parseOne(t, "[]")
	if n.Kind != NodeList || len(n.Elems) != 0 {
		t.Errorf("got %+v", n)
	}
}

func TestParser_Empty(t *testing.T) {
	_, err := NewParser(nil).Parse()
	if err != ErrEmpty {
		t.Errorf("err = %v, want ErrEmpty", err)
	}
}

func TestParser_NestedCalls(t *testing.T) {
	n := parseOne(t, `concat(upper($a), lower($b))`)
	if n.Kind != NodeCall || n.Ident != "concat" {
		t.Fatalf("root: %+v", n)
	}
	if n.Args[0].Kind != NodeCall || n.Args[0].Ident != "upper" {
		t.Errorf("arg 0: %+v", n.Args[0])
	}
}
