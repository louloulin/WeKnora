package formula

import (
	"fmt"
	"strings"
)

// NodeKind enumerates AST node types produced by the parser.
type NodeKind int

const (
	NodeLiteral NodeKind = iota
	NodeIdent
	NodeFieldRef
	NodeCall
	NodeBinary
	NodeUnary
	NodeList
	NodeIf
)

// Node is one AST node. The fields used depend on Kind:
//
//	Literal: Value
//	Ident:   Ident (function name when in call position)
//	FieldRef: Ident + optional chain (chain .Ident .Ident ...)
//	Call:    Ident (fn name) + Args ([]Node)
//	Binary:  Op + LHS + RHS
//	Unary:   Op + Operand
//	List:    Elems ([]Node)
//	If:      Cond + Then + Else
type Node struct {
	Kind   NodeKind
	Value  Value
	Ident  string
	Chain  []string // for FieldRef
	Args   []Node
	Op     string
	LHS    *Node
	RHS    *Node
	Operand *Node
	Elems  []Node
	Cond   *Node
	Then   *Node
	Else   *Node
}

// Parser is a recursive descent parser over the lexer output.
type Parser struct {
	tokens []Token
	pos    int
}

// NewParser constructs a Parser for the token stream.
func NewParser(tokens []Token) *Parser { return &Parser{tokens: tokens} }

// Parse parses the full expression. It accepts an expression and
// stops at EOF.
func (p *Parser) Parse() (Node, error) {
	if len(p.tokens) == 0 {
		return Node{}, ErrEmpty
	}
	// Skip a leading semicolon / EOF marker for robustness.
	if p.peek().Kind == TokenEOF {
		return Node{}, ErrEmpty
	}
	n, err := p.parseExpr()
	if err != nil {
		return n, err
	}
	if p.peek().Kind != TokenEOF {
		return n, fmt.Errorf("%w: %q at pos %d", ErrUnexpectedToken, p.peek().Text, p.peek().Pos)
	}
	return n, nil
}

// parseExpr parses an expression of precedence-tower shape:
//
//	expr   = ifExpr
//	ifExpr = "if" "(" expr "," expr "," expr ")" | logicalOr
//	logicalOr  = logicalAnd ("||" logicalAnd)*
//	logicalAnd = equality    ("&&" equality)*
//	equality   = compare     (("==" | "!=") compare)*
//	compare    = additive    (("<" | "<=" | ">" | ">=") additive)*
//	additive   = multiplicative (("+" | "-") multiplicative)*
//	multiplicative = unary (("*" | "/" | "%") unary)*
//	unary      = ("!" | "-") unary | postfix
//	postfix    = primary (chain | call)*
//	chain      = "." IDENT
//	call       = "(" arglist ")"
//	arglist    = (expr ("," expr)*)?
//	primary    = NUMBER | STRING | BOOL | NULL | IDENT | fieldRef | listLit | "(" expr ")"
func (p *Parser) parseExpr() (Node, error) { return p.parseIf() }

func (p *Parser) parseIf() (Node, error) {
	tok := p.peek()
	if tok.Kind == TokenIdent && strings.EqualFold(tok.Text, "if") {
		p.advance()
		if _, err := p.expect(TokenLParen); err != nil {
			return Node{}, err
		}
		cond, err := p.parseExpr()
		if err != nil {
			return Node{}, err
		}
		if _, err := p.expect(TokenComma); err != nil {
			return Node{}, err
		}
		then, err := p.parseExpr()
		if err != nil {
			return Node{}, err
		}
		if _, err := p.expect(TokenComma); err != nil {
			return Node{}, err
		}
		els, err := p.parseExpr()
		if err != nil {
			return Node{}, err
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return Node{}, err
		}
		return Node{Kind: NodeIf, Cond: &cond, Then: &then, Else: &els}, nil
	}
	return p.parseLogicalOr()
}

func (p *Parser) parseLogicalOr() (Node, error) {
	left, err := p.parseLogicalAnd()
	if err != nil {
		return left, err
	}
	for p.peek().Kind == TokenOr {
		p.advance()
		right, err := p.parseLogicalAnd()
		if err != nil {
			return right, err
		}
		currentLeft := left
		left = Node{Kind: NodeBinary, Op: "||", LHS: &currentLeft, RHS: &right}
	}
	return left, nil
}

func (p *Parser) parseLogicalAnd() (Node, error) {
	left, err := p.parseEquality()
	if err != nil {
		return left, err
	}
	for p.peek().Kind == TokenAnd {
		p.advance()
		right, err := p.parseEquality()
		if err != nil {
			return right, err
		}
		currentLeft := left
		left = Node{Kind: NodeBinary, Op: "&&", LHS: &currentLeft, RHS: &right}
	}
	return left, nil
}

func (p *Parser) parseEquality() (Node, error) {
	left, err := p.parseCompare()
	if err != nil {
		return left, err
	}
	for k := p.peek().Kind; k == TokenEq || k == TokenNeq; k = p.peek().Kind {
		op := p.peek().Text
		p.advance()
		right, err := p.parseCompare()
		if err != nil {
			return right, err
		}
		cur := left
	left = Node{Kind: NodeBinary, Op: op, LHS: &cur, RHS: &right}
	}
	return left, nil
}

func (p *Parser) parseCompare() (Node, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return left, err
	}
	for k := p.peek().Kind; k == TokenLt || k == TokenLte || k == TokenGt || k == TokenGte; k = p.peek().Kind {
		op := p.peek().Text
		p.advance()
		right, err := p.parseAdditive()
		if err != nil {
			return right, err
		}
		cur := left
	left = Node{Kind: NodeBinary, Op: op, LHS: &cur, RHS: &right}
	}
	return left, nil
}

func (p *Parser) parseAdditive() (Node, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return left, err
	}
	for k := p.peek().Kind; k == TokenPlus || k == TokenMinus; k = p.peek().Kind {
		op := p.peek().Text
		p.advance()
		right, err := p.parseMultiplicative()
		if err != nil {
			return right, err
		}
		cur := left
	left = Node{Kind: NodeBinary, Op: op, LHS: &cur, RHS: &right}
	}
	return left, nil
}

func (p *Parser) parseMultiplicative() (Node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return left, err
	}
	for k := p.peek().Kind; k == TokenStar || k == TokenSlash || k == TokenPercent; k = p.peek().Kind {
		op := p.peek().Text
		p.advance()
		right, err := p.parseUnary()
		if err != nil {
			return right, err
		}
		cur := left
	left = Node{Kind: NodeBinary, Op: op, LHS: &cur, RHS: &right}
	}
	return left, nil
}

func (p *Parser) parseUnary() (Node, error) {
	if p.peek().Kind == TokenNot {
		p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return operand, err
		}
		return Node{Kind: NodeUnary, Op: "!", Operand: &operand}, nil
	}
	if p.peek().Kind == TokenMinus {
		p.advance()
		operand, err := p.parseUnary()
		if err != nil {
			return operand, err
		}
		return Node{Kind: NodeUnary, Op: "-", Operand: &operand}, nil
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() (Node, error) {
	node, err := p.parsePrimary()
	if err != nil {
		return node, err
	}
	for {
		switch p.peek().Kind {
		case TokenDot:
			p.advance()
			name, err := p.expect(TokenIdent)
			if err != nil {
				return node, err
			}
			// X.Y becomes a "get" call: get(X, "Y").
			node = Node{
				Kind:  NodeCall,
				Ident: "get",
				Args:  []Node{node, {Kind: NodeLiteral, Value: Value{Kind: ValueString, Str: name.Text}}},
			}
		case TokenLParen:
			if node.Kind != NodeIdent {
				return node, fmt.Errorf("%w: call on non-identifier", ErrUnexpectedToken)
			}
			p.advance()
			args, err := p.parseArgList()
			if err != nil {
				return node, err
			}
			if _, err := p.expect(TokenRParen); err != nil {
				return node, err
			}
			node = Node{Kind: NodeCall, Ident: strings.ToLower(node.Ident), Args: args}
		default:
			return node, nil
		}
	}
}

func (p *Parser) parseArgList() ([]Node, error) {
	if p.peek().Kind == TokenRParen {
		return nil, nil
	}
	first, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	args := []Node{first}
	for p.peek().Kind == TokenComma {
		p.advance()
		next, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		args = append(args, next)
	}
	return args, nil
}

func (p *Parser) parsePrimary() (Node, error) {
	tok := p.peek()
	switch tok.Kind {
	case TokenNumber:
		p.advance()
		return Node{Kind: NodeLiteral, Value: Value{Kind: ValueNumber, Num: tok.Num}}, nil
	case TokenString:
		p.advance()
		return Node{Kind: NodeLiteral, Value: Value{Kind: ValueString, Str: tok.Str}}, nil
	case TokenBool:
		p.advance()
		return Node{Kind: NodeLiteral, Value: Value{Kind: ValueBool, Bool: tok.Bool}}, nil
	case TokenIdent:
		lower := strings.ToLower(tok.Text)
		if lower == "null" {
			p.advance()
			return Node{Kind: NodeLiteral, Value: Null}, nil
		}
		p.advance()
		return Node{Kind: NodeIdent, Ident: tok.Text}, nil
	case TokenFieldRef:
		p.advance()
		name, err := p.expect(TokenIdent)
		if err != nil {
			return Node{}, err
		}
		return Node{Kind: NodeFieldRef, Chain: []string{name.Text}}, nil
	case TokenLParen:
		p.advance()
		n, err := p.parseExpr()
		if err != nil {
			return n, err
		}
		if _, err := p.expect(TokenRParen); err != nil {
			return n, err
		}
		return n, nil
	case TokenLBracket:
		return p.parseListLiteral()
	}
	return Node{}, fmt.Errorf("%w: %q at pos %d", ErrUnexpectedToken, tok.Text, tok.Pos)
}

func (p *Parser) parseListLiteral() (Node, error) {
	p.advance() // [
	if p.peek().Kind == TokenRBracket {
		p.advance()
		return Node{Kind: NodeList, Elems: nil}, nil
	}
	first, err := p.parseExpr()
	if err != nil {
		return Node{}, err
	}
	elems := []Node{first}
	for p.peek().Kind == TokenComma {
		p.advance()
		next, err := p.parseExpr()
		if err != nil {
			return Node{}, err
		}
		elems = append(elems, next)
	}
	if _, err := p.expect(TokenRBracket); err != nil {
		return Node{}, err
	}
	return Node{Kind: NodeList, Elems: elems}, nil
}

func (p *Parser) peek() Token { return p.tokens[p.pos] }

func (p *Parser) advance() Token {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

func (p *Parser) expect(k TokenKind) (Token, error) {
	t := p.peek()
	if t.Kind != k {
		return t, fmt.Errorf("%w: want %s got %q at pos %d", ErrUnexpectedToken, k, t.Text, t.Pos)
	}
	p.advance()
	return t, nil
}
