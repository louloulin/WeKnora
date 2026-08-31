// Package formula is the v0.7.26 Build #31 (P0 gap G31) formula engine.
//
// It powers DatabaseFieldFormula / DatabaseFieldRollup columns in
// the multi-view database (#26). The engine is a small expression
// language with type-safe values, deterministic dependency
// extraction, and a built-in function library covering numbers,
// strings, dates, lists, and lookups.
//
// The engine is intentionally tiny — it is NOT a full programming
// language. Anything that needs a Turing-complete computation should
// use the Agent Studio instead.
package formula

import (
	"errors"
	"fmt"
	"time"
)

// TokenKind classifies the output of the lexer.
type TokenKind int

const (
	TokenEOF TokenKind = iota
	TokenIllegal

	TokenIdent // sum, count, upper, ...
	TokenNumber
	TokenString
	TokenBool

	TokenLParen
	TokenRParen
	TokenLBracket
	TokenRBracket
	TokenComma
	TokenDot

	TokenPlus
	TokenMinus
	TokenStar
	TokenSlash
	TokenPercent

	TokenEq
	TokenNeq
	TokenLt
	TokenLte
	TokenGt
	TokenGte

	TokenAnd
	TokenOr
	TokenNot

	// FieldRef is a placeholder for "$column" or "column".
	TokenFieldRef
)

func (k TokenKind) String() string {
	switch k {
	case TokenEOF:
		return "EOF"
	case TokenIllegal:
		return "ILLEGAL"
	case TokenIdent:
		return "IDENT"
	case TokenNumber:
		return "NUMBER"
	case TokenString:
		return "STRING"
	case TokenBool:
		return "BOOL"
	case TokenLParen:
		return "("
	case TokenRParen:
		return ")"
	case TokenLBracket:
		return "["
	case TokenRBracket:
		return "]"
	case TokenComma:
		return ","
	case TokenDot:
		return "."
	case TokenPlus:
		return "+"
	case TokenMinus:
		return "-"
	case TokenStar:
		return "*"
	case TokenSlash:
		return "/"
	case TokenPercent:
		return "%"
	case TokenEq:
		return "=="
	case TokenNeq:
		return "!="
	case TokenLt:
		return "<"
	case TokenLte:
		return "<="
	case TokenGt:
		return ">"
	case TokenGte:
		return ">="
	case TokenAnd:
		return "&&"
	case TokenOr:
		return "||"
	case TokenNot:
		return "!"
	case TokenFieldRef:
		return "$"
	}
	return "UNKNOWN"
}

// Token is a single lexed unit. Fields are mutually exclusive:
// Number -> Num, String -> Str, Ident -> Text.
type Token struct {
	Kind  TokenKind
	Text  string
	Num   float64
	Str   string
	Bool  bool
	Pos   int // byte offset of the token start
}

// Value is the result of evaluating an expression. Values are
// immutable; comparisons are type-strict.
type Value struct {
	// Kind is one of "number" | "string" | "bool" | "date" | "list" | "null".
	Kind string `json:"kind"`
	Num  float64
	Str  string
	Bool bool
	// Date is RFC3339 (preserves sub-second precision via Time).
	Date time.Time
	List []Value
}

// ValueKind constants for Value.Kind.
const (
	ValueNumber = "number"
	ValueString = "string"
	ValueBool   = "bool"
	ValueDate   = "date"
	ValueList   = "list"
	ValueNull   = "null"
)

// Null is a sentinel for missing values.
var Null = Value{Kind: ValueNull}

// Common errors.
var (
	ErrEmpty            = errors.New("formula: empty expression")
	ErrUnexpectedToken  = errors.New("formula: unexpected token")
	ErrUnknownFunction  = errors.New("formula: unknown function")
	ErrTypeMismatch     = errors.New("formula: type mismatch")
	ErrArityMismatch    = errors.New("formula: function arity mismatch")
	ErrDivisionByZero   = errors.New("formula: division by zero")
	ErrUnknownField     = errors.New("formula: unknown field")
	ErrCyclicDependency = errors.New("formula: cyclic dependency")
)

// AsString formats a Value for human display. Dates render as
// RFC3339Nano. Lists render as comma-separated string representations.
func (v Value) AsString() string {
	switch v.Kind {
	case ValueNull:
		return ""
	case ValueString:
		return v.Str
	case ValueNumber:
		// Integer-valued floats print without decimals.
		if v.Num == float64(int64(v.Num)) {
			return fmt.Sprintf("%d", int64(v.Num))
		}
		return fmt.Sprintf("%g", v.Num)
	case ValueBool:
		if v.Bool {
			return "true"
		}
		return "false"
	case ValueDate:
		if v.Date.IsZero() {
			return ""
		}
		return v.Date.Format(time.RFC3339Nano)
	case ValueList:
		out := ""
		for i, item := range v.List {
			if i > 0 {
				out += ","
			}
			out += item.AsString()
		}
		return out
	}
	return ""
}

// Equal performs type-strict equality.
func (v Value) Equal(other Value) bool {
	if v.Kind != other.Kind {
		return false
	}
	switch v.Kind {
	case ValueNull:
		return true
	case ValueNumber:
		return v.Num == other.Num
	case ValueString:
		return v.Str == other.Str
	case ValueBool:
		return v.Bool == other.Bool
	case ValueDate:
		return v.Date.Equal(other.Date)
	case ValueList:
		if len(v.List) != len(other.List) {
			return false
		}
		for i := range v.List {
			if !v.List[i].Equal(other.List[i]) {
				return false
			}
		}
		return true
	}
	return false
}
