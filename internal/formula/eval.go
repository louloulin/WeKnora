package formula

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// EvalContext carries the inputs the evaluator needs to compute a
// formula expression. Fields is keyed by column name (case
// insensitive). Now returns the current time so date() / now() /
// date_add can be deterministic in tests.
type EvalContext struct {
	Fields map[string]Value
	Now    func() time.Time
}

// NewContext returns a context with the given field map. Now
// defaults to time.Now.
func NewContext(fields map[string]Value) *EvalContext {
	if fields == nil {
		fields = map[string]Value{}
	}
	return &EvalContext{Fields: fields, Now: time.Now}
}

// Evaluate parses (if not already parsed) and evaluates src against
// ctx. The parse cache is intentionally not retained across calls
// — callers should compile once and Evaluate many times.
func Evaluate(src string, ctx *EvalContext) (Value, error) {
	tokens, err := NewLexer(src).Tokenize()
	if err != nil {
		return Null, err
	}
	node, err := NewParser(tokens).Parse()
	if err != nil {
		return Null, err
	}
	if ctx == nil {
		ctx = NewContext(nil)
	}
	return Eval(node, ctx)
}

// Eval evaluates an already-parsed AST.
func Eval(n Node, ctx *EvalContext) (Value, error) {
	switch n.Kind {
	case NodeLiteral:
		return n.Value, nil
	case NodeIdent:
		// Bare identifier: lookup as a field name. If absent, treat as null.
		key := strings.ToLower(n.Ident)
		if v, ok := ctx.Fields[key]; ok {
			return v, nil
		}
		return Null, nil
	case NodeFieldRef:
		if len(n.Chain) == 0 {
			return Null, nil
		}
		key := strings.ToLower(n.Chain[0])
		v, ok := ctx.Fields[key]
		if !ok {
			return Null, nil
		}
		// Walk chained keys: $a.b.c becomes ctx[a][b][c].
		cur := v
		for _, seg := range n.Chain[1:] {
			if cur.Kind != ValueList {
				return Null, nil
			}
			next, ok := listGetByKey(cur, seg)
			if !ok {
				return Null, nil
			}
			cur = next
		}
		return cur, nil
	case NodeUnary:
		return evalUnary(n, ctx)
	case NodeBinary:
		return evalBinary(n, ctx)
	case NodeCall:
		return evalCall(n, ctx)
	case NodeList:
		out := make([]Value, 0, len(n.Elems))
		for i := range n.Elems {
			v, err := Eval(n.Elems[i], ctx)
			if err != nil {
				return Null, err
			}
			out = append(out, v)
		}
		return Value{Kind: ValueList, List: out}, nil
	case NodeIf:
		c, err := Eval(*n.Cond, ctx)
		if err != nil {
			return Null, err
		}
		if truthy(c) {
			return Eval(*n.Then, ctx)
		}
		return Eval(*n.Else, ctx)
	}
	return Null, fmt.Errorf("formula: unknown node kind %d", n.Kind)
}

func evalUnary(n Node, ctx *EvalContext) (Value, error) {
	v, err := Eval(*n.Operand, ctx)
	if err != nil {
		return Null, err
	}
	switch n.Op {
	case "!":
		return Value{Kind: ValueBool, Bool: !truthy(v)}, nil
	case "-":
		if v.Kind != ValueNumber {
			return Null, fmt.Errorf("%w: unary - on %s", ErrTypeMismatch, v.Kind)
		}
		return Value{Kind: ValueNumber, Num: -v.Num}, nil
	}
	return Null, fmt.Errorf("formula: unknown unary op %q", n.Op)
}

func evalBinary(n Node, ctx *EvalContext) (Value, error) {
	lhs, err := Eval(*n.LHS, ctx)
	if err != nil {
		return Null, err
	}
	rhs, err := Eval(*n.RHS, ctx)
	if err != nil {
		return Null, err
	}
	switch n.Op {
	case "+":
		if lhs.Kind == ValueNull || rhs.Kind == ValueNull {
			return Null, nil
		}
		if lhs.Kind == ValueNumber && rhs.Kind == ValueNumber {
			return Value{Kind: ValueNumber, Num: lhs.Num + rhs.Num}, nil
		}
		if lhs.Kind == ValueString || rhs.Kind == ValueString {
			return Value{Kind: ValueString, Str: lhs.AsString() + rhs.AsString()}, nil
		}
		if lhs.Kind == ValueList || rhs.Kind == ValueList {
			l := lhs.Kind == ValueList
			r := rhs.Kind == ValueList
			if l && r {
				return Value{Kind: ValueList, List: append(append([]Value{}, lhs.List...), rhs.List...)}, nil
			}
			if l {
				return Value{Kind: ValueList, List: append(lhs.List, rhs)}, nil
			}
			return Value{Kind: ValueList, List: append(rhs.List, lhs)}, nil
		}
		if lhs.Kind == ValueDate && rhs.Kind == ValueNumber {
			return Value{Kind: ValueDate, Date: lhs.Date.Add(time.Duration(rhs.Num) * 24 * time.Hour)}, nil
		}
		return Null, fmt.Errorf("%w: + on %s and %s", ErrTypeMismatch, lhs.Kind, rhs.Kind)
	case "-":
		if lhs.Kind == ValueNumber && rhs.Kind == ValueNumber {
			return Value{Kind: ValueNumber, Num: lhs.Num - rhs.Num}, nil
		}
		if lhs.Kind == ValueDate && rhs.Kind == ValueNumber {
			return Value{Kind: ValueDate, Date: lhs.Date.Add(-time.Duration(rhs.Num) * 24 * time.Hour)}, nil
		}
		if lhs.Kind == ValueDate && rhs.Kind == ValueDate {
			days := lhs.Date.Sub(rhs.Date).Hours() / 24
			return Value{Kind: ValueNumber, Num: days}, nil
		}
		return Null, fmt.Errorf("%w: - on %s and %s", ErrTypeMismatch, lhs.Kind, rhs.Kind)
	case "*":
		if lhs.Kind == ValueNumber && rhs.Kind == ValueNumber {
			return Value{Kind: ValueNumber, Num: lhs.Num * rhs.Num}, nil
		}
		return Null, fmt.Errorf("%w: * on %s and %s", ErrTypeMismatch, lhs.Kind, rhs.Kind)
	case "/":
		if lhs.Kind == ValueNumber && rhs.Kind == ValueNumber {
			if rhs.Num == 0 {
				return Null, ErrDivisionByZero
			}
			return Value{Kind: ValueNumber, Num: lhs.Num / rhs.Num}, nil
		}
		return Null, fmt.Errorf("%w: / on %s and %s", ErrTypeMismatch, lhs.Kind, rhs.Kind)
	case "%":
		if lhs.Kind == ValueNumber && rhs.Kind == ValueNumber {
			if rhs.Num == 0 {
				return Null, ErrDivisionByZero
			}
			return Value{Kind: ValueNumber, Num: math.Mod(lhs.Num, rhs.Num)}, nil
		}
		return Null, fmt.Errorf("%w: %% on %s and %s", ErrTypeMismatch, lhs.Kind, rhs.Kind)
	case "==":
		return Value{Kind: ValueBool, Bool: lhs.Equal(rhs)}, nil
	case "!=":
		return Value{Kind: ValueBool, Bool: !lhs.Equal(rhs)}, nil
	case "<", "<=", ">", ">=":
		return cmpOp(n.Op, lhs, rhs)
	case "&&":
		return Value{Kind: ValueBool, Bool: truthy(lhs) && truthy(rhs)}, nil
	case "||":
		return Value{Kind: ValueBool, Bool: truthy(lhs) || truthy(rhs)}, nil
	}
	return Null, fmt.Errorf("formula: unknown binary op %q", n.Op)
}

func cmpOp(op string, lhs, rhs Value) (Value, error) {
	// Type-strict numeric comparisons.
	if lhs.Kind == ValueNumber && rhs.Kind == ValueNumber {
		switch op {
		case "<":
			return Value{Kind: ValueBool, Bool: lhs.Num < rhs.Num}, nil
		case "<=":
			return Value{Kind: ValueBool, Bool: lhs.Num <= rhs.Num}, nil
		case ">":
			return Value{Kind: ValueBool, Bool: lhs.Num > rhs.Num}, nil
		case ">=":
			return Value{Kind: ValueBool, Bool: lhs.Num >= rhs.Num}, nil
		}
	}
	if lhs.Kind == ValueString && rhs.Kind == ValueString {
		switch op {
		case "<":
			return Value{Kind: ValueBool, Bool: lhs.Str < rhs.Str}, nil
		case "<=":
			return Value{Kind: ValueBool, Bool: lhs.Str <= rhs.Str}, nil
		case ">":
			return Value{Kind: ValueBool, Bool: lhs.Str > rhs.Str}, nil
		case ">=":
			return Value{Kind: ValueBool, Bool: lhs.Str >= rhs.Str}, nil
		}
	}
	if lhs.Kind == ValueDate && rhs.Kind == ValueDate {
		switch op {
		case "<":
			return Value{Kind: ValueBool, Bool: lhs.Date.Before(rhs.Date)}, nil
		case "<=":
			return Value{Kind: ValueBool, Bool: !rhs.Date.Before(lhs.Date)}, nil
		case ">":
			return Value{Kind: ValueBool, Bool: lhs.Date.After(rhs.Date)}, nil
		case ">=":
			return Value{Kind: ValueBool, Bool: !lhs.Date.Before(rhs.Date)}, nil
		}
	}
	return Null, fmt.Errorf("%w: %s on %s and %s", ErrTypeMismatch, op, lhs.Kind, rhs.Kind)
}

func evalCall(n Node, ctx *EvalContext) (Value, error) {
	fn, ok := builtins[n.Ident]
	if !ok {
		return Null, fmt.Errorf("%w: %s", ErrUnknownFunction, n.Ident)
	}
	args := make([]Value, len(n.Args))
	for i := range n.Args {
		v, err := Eval(n.Args[i], ctx)
		if err != nil {
			return Null, err
		}
		args[i] = v
	}
	if fn.min != -1 && len(args) < fn.min {
		return Null, fmt.Errorf("%w: %s expects >= %d, got %d", ErrArityMismatch, n.Ident, fn.min, len(args))
	}
	if fn.max != -1 && len(args) > fn.max {
		return Null, fmt.Errorf("%w: %s expects <= %d, got %d", ErrArityMismatch, n.Ident, fn.max, len(args))
	}
	return fn.body(args, ctx)
}

type builtinSpec struct {
	min, max int
	body     func([]Value, *EvalContext) (Value, error)
}

var builtins = map[string]builtinSpec{
	"now":        {0, 0, builtinNow},
	"today":      {0, 0, builtinNow},
	"date":       {1, 1, builtinDate},
	"date_add":   {2, 3, builtinDateAdd},
	"date_diff":  {3, 3, builtinDateDiff},
	"year":       {1, 1, builtinYear},
	"month":      {1, 1, builtinMonth},
	"day":        {1, 1, builtinDay},
	"upper":      {1, 1, builtinUpper},
	"lower":      {1, 1, builtinLower},
	"trim":       {1, 1, builtinTrim},
	"concat":     {1, -1, builtinConcat},
	"len":        {1, 1, builtinLen},
	"contains":   {2, 2, builtinContains},
	"starts_with": {2, 2, builtinStartsWith},
	"ends_with":  {2, 2, builtinEndsWith},
	"if":         {3, 3, builtinIf},
	"coalesce":   {1, -1, builtinCoalesce},
	"round":      {2, 2, builtinRound},
	"floor":      {1, 1, builtinFloor},
	"ceil":       {1, 1, builtinCeil},
	"abs":        {1, 1, builtinAbs},
	"min":        {1, -1, builtinMin},
	"max":        {1, -1, builtinMax},
	"sum":        {1, 1, builtinSum},
	"avg":        {1, 1, builtinAvg},
	"count":      {1, 1, builtinCount},
	"first":      {1, 1, builtinFirst},
	"last":       {1, 1, builtinLast},
	"get":        {2, 2, builtinGet},
}

func builtinNow(args []Value, ctx *EvalContext) (Value, error) {
	if ctx.Now != nil {
		return Value{Kind: ValueDate, Date: ctx.Now()}, nil
	}
	return Value{Kind: ValueDate, Date: time.Now()}, nil
}

func builtinDate(args []Value, ctx *EvalContext) (Value, error) {
	s := args[0].AsString()
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return Value{Kind: ValueDate, Date: t}, nil
		}
	}
	return Null, fmt.Errorf("formula: date() cannot parse %q", s)
}

func builtinDateAdd(args []Value, ctx *EvalContext) (Value, error) {
	if args[0].Kind != ValueDate {
		return Null, fmt.Errorf("%w: date_add expects date, got %s", ErrTypeMismatch, args[0].Kind)
	}
	unit := "day"
	if len(args) == 3 {
		unit = args[2].Str
	}
	d := time.Duration(args[1].Num * 24 * float64(time.Hour) / 24)
	switch unit {
	case "day":
		d = time.Duration(args[1].Num) * 24 * time.Hour
	case "hour":
		d = time.Duration(args[1].Num) * time.Hour
	case "minute":
		d = time.Duration(args[1].Num) * time.Minute
	case "second":
		d = time.Duration(args[1].Num) * time.Second
	default:
		return Null, fmt.Errorf("formula: date_add unit %q", unit)
	}
	return Value{Kind: ValueDate, Date: args[0].Date.Add(d)}, nil
}

func builtinDateDiff(args []Value, ctx *EvalContext) (Value, error) {
	if args[0].Kind != ValueDate || args[1].Kind != ValueDate {
		return Null, fmt.Errorf("%w: date_diff expects (date,date,unit)", ErrTypeMismatch)
	}
	unit := args[2].Str
	diff := args[0].Date.Sub(args[1].Date)
	switch unit {
	case "day":
		return Value{Kind: ValueNumber, Num: diff.Hours() / 24}, nil
	case "hour":
		return Value{Kind: ValueNumber, Num: diff.Hours()}, nil
	case "minute":
		return Value{Kind: ValueNumber, Num: diff.Minutes()}, nil
	case "second":
		return Value{Kind: ValueNumber, Num: diff.Seconds()}, nil
	}
	return Null, fmt.Errorf("formula: date_diff unit %q", unit)
}

func builtinYear(args []Value, ctx *EvalContext) (Value, error) {
	return dateField(args, "year")
}
func builtinMonth(args []Value, ctx *EvalContext) (Value, error) {
	return dateField(args, "month")
}
func builtinDay(args []Value, ctx *EvalContext) (Value, error) {
	return dateField(args, "day")
}

func dateField(args []Value, field string) (Value, error) {
	if args[0].Kind != ValueDate {
		return Null, fmt.Errorf("%w: %s expects date, got %s", ErrTypeMismatch, field, args[0].Kind)
	}
	t := args[0].Date
	switch field {
	case "year":
		return Value{Kind: ValueNumber, Num: float64(t.Year())}, nil
	case "month":
		return Value{Kind: ValueNumber, Num: float64(t.Month())}, nil
	case "day":
		return Value{Kind: ValueNumber, Num: float64(t.Day())}, nil
	}
	return Null, fmt.Errorf("formula: unknown date field %s", field)
}

func builtinUpper(args []Value, ctx *EvalContext) (Value, error) {
	return Value{Kind: ValueString, Str: strings.ToUpper(args[0].AsString())}, nil
}
func builtinLower(args []Value, ctx *EvalContext) (Value, error) {
	return Value{Kind: ValueString, Str: strings.ToLower(args[0].AsString())}, nil
}
func builtinTrim(args []Value, ctx *EvalContext) (Value, error) {
	return Value{Kind: ValueString, Str: strings.TrimSpace(args[0].AsString())}, nil
}
func builtinConcat(args []Value, ctx *EvalContext) (Value, error) {
	var b strings.Builder
	for _, a := range args {
		b.WriteString(a.AsString())
	}
	return Value{Kind: ValueString, Str: b.String()}, nil
}
func builtinLen(args []Value, ctx *EvalContext) (Value, error) {
	switch args[0].Kind {
	case ValueString:
		return Value{Kind: ValueNumber, Num: float64(len(args[0].Str))}, nil
	case ValueList:
		return Value{Kind: ValueNumber, Num: float64(len(args[0].List))}, nil
	}
	return Value{Kind: ValueNumber, Num: 0}, nil
}
func builtinContains(args []Value, ctx *EvalContext) (Value, error) {
	return Value{Kind: ValueBool, Bool: strings.Contains(args[0].AsString(), args[1].AsString())}, nil
}
func builtinStartsWith(args []Value, ctx *EvalContext) (Value, error) {
	return Value{Kind: ValueBool, Bool: strings.HasPrefix(args[0].AsString(), args[1].AsString())}, nil
}
func builtinEndsWith(args []Value, ctx *EvalContext) (Value, error) {
	return Value{Kind: ValueBool, Bool: strings.HasSuffix(args[0].AsString(), args[1].AsString())}, nil
}
func builtinIf(args []Value, ctx *EvalContext) (Value, error) {
	if truthy(args[0]) {
		return args[1], nil
	}
	return args[2], nil
}
func builtinCoalesce(args []Value, ctx *EvalContext) (Value, error) {
	for _, a := range args {
		if a.Kind != ValueNull {
			return a, nil
		}
	}
	return Null, nil
}
func builtinRound(args []Value, ctx *EvalContext) (Value, error) {
	places := int(args[1].Num)
	mul := math.Pow(10, float64(places))
	return Value{Kind: ValueNumber, Num: math.Round(args[0].Num*mul) / mul}, nil
}
func builtinFloor(args []Value, ctx *EvalContext) (Value, error) {
	return Value{Kind: ValueNumber, Num: math.Floor(args[0].Num)}, nil
}
func builtinCeil(args []Value, ctx *EvalContext) (Value, error) {
	return Value{Kind: ValueNumber, Num: math.Ceil(args[0].Num)}, nil
}
func builtinAbs(args []Value, ctx *EvalContext) (Value, error) {
	return Value{Kind: ValueNumber, Num: math.Abs(args[0].Num)}, nil
}
func builtinMin(args []Value, ctx *EvalContext) (Value, error) {
	return foldNumbers(args, math.Min)
}
func builtinMax(args []Value, ctx *EvalContext) (Value, error) {
	return foldNumbers(args, math.Max)
}

func foldNumbers(args []Value, op func(float64, float64) float64) (Value, error) {
	if len(args) == 0 {
		return Null, nil
	}
	if args[0].Kind == ValueList {
		args = args[0].List
	}
	if len(args) == 0 {
		return Null, nil
	}
	out := args[0].Num
	for _, a := range args[1:] {
		if a.Kind != ValueNumber {
			return Null, fmt.Errorf("%w: %s in numeric fold", ErrTypeMismatch, a.Kind)
		}
		out = op(out, a.Num)
	}
	return Value{Kind: ValueNumber, Num: out}, nil
}

func builtinSum(args []Value, ctx *EvalContext) (Value, error) {
	if args[0].Kind != ValueList {
		return Null, fmt.Errorf("%w: sum expects list, got %s", ErrTypeMismatch, args[0].Kind)
	}
	var sum float64
	for _, a := range args[0].List {
		if a.Kind != ValueNumber {
			return Null, fmt.Errorf("%w: %s in sum", ErrTypeMismatch, a.Kind)
		}
		sum += a.Num
	}
	return Value{Kind: ValueNumber, Num: sum}, nil
}
func builtinAvg(args []Value, ctx *EvalContext) (Value, error) {
	if args[0].Kind != ValueList || len(args[0].List) == 0 {
		return Null, fmt.Errorf("%w: avg expects non-empty list", ErrTypeMismatch)
	}
	var sum float64
	for _, a := range args[0].List {
		if a.Kind != ValueNumber {
			return Null, fmt.Errorf("%w: %s in avg", ErrTypeMismatch, a.Kind)
		}
		sum += a.Num
	}
	return Value{Kind: ValueNumber, Num: sum / float64(len(args[0].List))}, nil
}
func builtinCount(args []Value, ctx *EvalContext) (Value, error) {
	if args[0].Kind != ValueList {
		return Null, fmt.Errorf("%w: count expects list, got %s", ErrTypeMismatch, args[0].Kind)
	}
	return Value{Kind: ValueNumber, Num: float64(len(args[0].List))}, nil
}
func builtinFirst(args []Value, ctx *EvalContext) (Value, error) {
	if args[0].Kind != ValueList || len(args[0].List) == 0 {
		return Null, nil
	}
	return args[0].List[0], nil
}
func builtinLast(args []Value, ctx *EvalContext) (Value, error) {
	if args[0].Kind != ValueList || len(args[0].List) == 0 {
		return Null, nil
	}
	return args[0].List[len(args[0].List)-1], nil
}
func builtinGet(args []Value, ctx *EvalContext) (Value, error) {
	if args[0].Kind != ValueList {
		return Null, nil
	}
	v, _ := listGetByKey(args[0], args[1].Str)
	return v, nil
}

func listGetByKey(list Value, key string) (Value, bool) {
	for _, item := range list.List {
		if item.Kind == ValueList && len(item.List) >= 2 {
			k := item.List[0].AsString()
			if strings.EqualFold(k, key) {
				return item.List[1], true
			}
		}
	}
	return Null, false
}

func truthy(v Value) bool {
	switch v.Kind {
	case ValueNull:
		return false
	case ValueBool:
		return v.Bool
	case ValueNumber:
		return v.Num != 0
	case ValueString:
		return v.Str != ""
	case ValueList:
		return len(v.List) > 0
	case ValueDate:
		return !v.Date.IsZero()
	}
	return false
}
