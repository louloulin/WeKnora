package formula

import (
	"testing"
	"time"
)

func TestEval_Arithmetic(t *testing.T) {
	cases := []struct {
		src  string
		want float64
	}{
		{"1 + 2", 3},
		{"10 - 3", 7},
		{"4 * 5", 20},
		{"20 / 4", 5},
		{"20 / 8", 2.5},
		{"17 % 5", 2},
		{"(1 + 2) * 3", 9},
		{"2 + 3 * 4", 14},
		{"-5 + 10", 5},
	}
	for _, c := range cases {
		v, err := Evaluate(c.src, nil)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if v.Kind != ValueNumber || v.Num != c.want {
			t.Errorf("%s = %v want %v", c.src, v.Num, c.want)
		}
	}
}

func TestEval_Comparisons(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{"1 < 2", true},
		{"2 <= 2", true},
		{"3 > 3", false},
		{"3 >= 3", true},
		{"1 == 1", true},
		{"1 != 2", true},
		{`"a" < "b"`, true},
	}
	for _, c := range cases {
		v, err := Evaluate(c.src, nil)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if v.Kind != ValueBool || v.Bool != c.want {
			t.Errorf("%s = %v want %v", c.src, v.Bool, c.want)
		}
	}
}

func TestEval_Logic(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{"true && false", false},
		{"true || false", true},
		{"!true", false},
		{"!false", true},
		{"1 < 2 && 3 < 4", true},
	}
	for _, c := range cases {
		v, err := Evaluate(c.src, nil)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if v.Kind != ValueBool || v.Bool != c.want {
			t.Errorf("%s = %v want %v", c.src, v.Bool, c.want)
		}
	}
}

func TestEval_Strings(t *testing.T) {
	cases := []struct {
		src, want string
	}{
		{`upper("hello")`, "HELLO"},
		{`lower("HELLO")`, "hello"},
		{`trim("  hi  ")`, "hi"},
		{`concat("a", "b", "c")`, "abc"},
		{`len("hello")`, "5"},
		{`contains("hello world", "world")`, "true"},
		{`starts_with("hello", "he")`, "true"},
		{`ends_with("hello", "lo")`, "true"},
	}
	for _, c := range cases {
		v, err := Evaluate(c.src, nil)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if v.AsString() != c.want {
			t.Errorf("%s = %q want %q", c.src, v.AsString(), c.want)
		}
	}
}

func TestEval_IfAndCoalesce(t *testing.T) {
	v, err := Evaluate("if(1 < 2, \"yes\", \"no\")", nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Str != "yes" {
		t.Errorf("if: %q", v.Str)
	}
	v, err = Evaluate(`coalesce(null, null, "fallback")`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Str != "fallback" {
		t.Errorf("coalesce: %q", v.Str)
	}
}

func TestEval_FieldRef(t *testing.T) {
	ctx := NewContext(map[string]Value{
		"price": {Kind: ValueNumber, Num: 100},
		"name":  {Kind: ValueString, Str: "Widget"},
	})
	v, err := Evaluate("round($price * 1.1, 0)", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Num != 110 {
		t.Errorf("price * 1.1 = %v want 110", v.Num)
	}
	v, err = Evaluate(`concat("Product: ", $name)`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Str != "Product: Widget" {
		t.Errorf("got %q", v.Str)
	}
}

func TestEval_UnknownFieldIsNull(t *testing.T) {
	v, err := Evaluate(`$missing`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Kind != ValueNull {
		t.Errorf("expected null, got %v", v)
	}
}

func TestEval_ArithmeticWithNull(t *testing.T) {
	// null + 1 should be null (no implicit coercion).
	v, err := Evaluate(`null + 1`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Kind != ValueNull {
		t.Errorf("got %v", v)
	}
}

func TestEval_DivisionByZero(t *testing.T) {
	_, err := Evaluate("1 / 0", nil)
	if err != ErrDivisionByZero {
		t.Errorf("err = %v, want ErrDivisionByZero", err)
	}
}

func TestEval_UnknownFunction(t *testing.T) {
	_, err := Evaluate("frobnicate(1)", nil)
	if err == nil {
		t.Fatal("expected error on unknown function")
	}
}

func TestEval_TypeMismatch(t *testing.T) {
	_, err := Evaluate(`"a" * 2`, nil)
	if err == nil {
		t.Fatal("expected error on type mismatch")
	}
}

func TestEval_ListAggregation(t *testing.T) {
	ctx := NewContext(map[string]Value{
		"items": {Kind: ValueList, List: []Value{
			{Kind: ValueNumber, Num: 1},
			{Kind: ValueNumber, Num: 2},
			{Kind: ValueNumber, Num: 3},
			{Kind: ValueNumber, Num: 4},
		}},
	})
	cases := []struct {
		src  string
		want float64
	}{
		{"sum($items)", 10},
		{"avg($items)", 2.5},
		{"count($items)", 4},
		{"min($items)", 1},
		{"max($items)", 4},
	}
	for _, c := range cases {
		v, err := Evaluate(c.src, ctx)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if v.Num != c.want {
			t.Errorf("%s = %v want %v", c.src, v.Num, c.want)
		}
	}
}

func TestEval_Numbers(t *testing.T) {
	cases := []struct {
		src  string
		want float64
	}{
		{"round(3.14159, 2)", 3.14},
		{"floor(3.7)", 3},
		{"ceil(3.2)", 4},
		{"abs(-7)", 7},
	}
	for _, c := range cases {
		v, err := Evaluate(c.src, nil)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if v.Num != c.want {
			t.Errorf("%s = %v want %v", c.src, v.Num, c.want)
		}
	}
}

func TestEval_DateFunctions(t *testing.T) {
	// Use a frozen clock so date math is deterministic.
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	ctx := NewContext(map[string]Value{})
	ctx.Now = func() time.Time { return now }
	v, err := Evaluate("now()", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !v.Date.Equal(now) {
		t.Errorf("now() = %v want %v", v.Date, now)
	}
	v, err = Evaluate("year(now())", ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Num != 2026 {
		t.Errorf("year: %v", v.Num)
	}
	v, err = Evaluate(`date_add(now(), 7, "day")`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	expected := now.Add(7 * 24 * time.Hour)
	if !v.Date.Equal(expected) {
		t.Errorf("date_add = %v want %v", v.Date, expected)
	}
}

func TestEval_RollupPattern(t *testing.T) {
	// Simulate: $line_items is a list of [key, value] pairs.
	// sum( map(items, x -> x[1]) ) == rollup pattern.
	ctx := NewContext(map[string]Value{
		"line_items": {Kind: ValueList, List: []Value{
			{Kind: ValueList, List: []Value{
				{Kind: ValueString, Str: "price"}, {Kind: ValueNumber, Num: 10},
			}},
			{Kind: ValueList, List: []Value{
				{Kind: ValueString, Str: "price"}, {Kind: ValueNumber, Num: 25},
			}},
		}},
	})
	v, err := Evaluate(`get($line_items, "price")`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Num != 10 {
		t.Errorf("get($line_items, 'price') = %v want 10", v.Num)
	}
}

func TestEval_DateDiffDays(t *testing.T) {
	ctx := NewContext(map[string]Value{
		"start": {Kind: ValueDate, Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		"end":   {Kind: ValueDate, Date: time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC)},
	})
	v, err := Evaluate(`date_diff($end, $start, "day")`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Num != 10 {
		t.Errorf("date_diff = %v want 10", v.Num)
	}
}

func TestEval_ConcatMixed(t *testing.T) {
	ctx := NewContext(map[string]Value{
		"first": {Kind: ValueString, Str: "Jane"},
		"last":  {Kind: ValueString, Str: "Doe"},
	})
	v, err := Evaluate(`concat($first, " ", $last)`, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if v.Str != "Jane Doe" {
		t.Errorf("got %q", v.Str)
	}
}
