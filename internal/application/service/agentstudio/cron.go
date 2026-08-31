package agentstudio

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cronParser implements a minimal subset of the 5-field POSIX cron
// format used for Agent Studio triggers:
//
//   minute       (0-59)
//   hour         (0-23)
//   day-of-month (1-31)
//   month        (1-12)
//   day-of-week  (0-6, Sunday=0)
//
// Supported syntax: * (any), N (specific), N-M (range), N,M (list),
// */N (step). Day-of-week 7 is normalized to 0 (Sunday) for
// compatibility with Quartz-style schedulers.
//
// This is not a full RFC 5545 implementation — it's deliberately
// small (~150 LOC) so it can be audited at a glance and doesn't pull
// in a third-party dependency for a single use site.
type cronParser struct{}

// newCronParser returns a fresh parser. Stateless — could be a global.
func newCronParser() *cronParser { return &cronParser{} }

// nextAfter returns the first time strictly after `from` that matches
// the expression. Returns an error for unparseable expressions.
func (c *cronParser) nextAfter(expr string, from time.Time) (time.Time, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return time.Time{}, fmt.Errorf("cron: empty expression")
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron: expected 5 fields, got %d", len(fields))
	}
	minute, err := parseField(fields[0], 0, 59)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron.minute: %w", err)
	}
	hour, err := parseField(fields[1], 0, 23)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron.hour: %w", err)
	}
	dom, err := parseField(fields[2], 1, 31)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron.dom: %w", err)
	}
	month, err := parseField(fields[3], 1, 12)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron.month: %w", err)
	}
	dow, err := parseField(fields[4], 0, 6)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron.dow: %w", err)
	}

	// Iterate second-by-second up to ~4 years to find the next match.
	// Linear search is fine because cron jobs are coarse (per-minute
	// granularity) and the iteration cap prevents infinite loops on
	// impossible expressions like "Feb 30".
	t := from.Add(time.Minute).Truncate(time.Minute)
	deadline := from.Add(4 * 365 * 24 * time.Hour)
	for ; t.Before(deadline); t = t.Add(time.Minute) {
		if !month[t.Month()] {
			continue
		}
		if !dom[t.Day()] {
			continue
		}
		if !dow[int(t.Weekday())] {
			continue
		}
		if !hour[t.Hour()] {
			continue
		}
		if !minute[t.Minute()] {
			continue
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cron: no match within 4 years for %q", expr)
}

// fieldSet is a [60]bool covering the full minute/hour range, or [32]
// for day-of-month, [13] for month, [7] for day-of-week.
type fieldSet []bool

// parseField parses one cron field. min..max inclusive.
func parseField(s string, min, max int) (fieldSet, error) {
	out := make(fieldSet, max+1)
	// Wildcard or step-only.
	if s == "*" {
		for i := min; i <= max; i++ {
			out[i] = true
		}
		return out, nil
	}
	// Step: */N
	if strings.HasPrefix(s, "*/") {
		step, err := strconv.Atoi(s[2:])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step %q", s)
		}
		for i := min; i <= max; i += step {
			out[i] = true
		}
		return out, nil
	}
	// List: a,b,c
	for _, part := range strings.Split(s, ",") {
		if err := parseFieldPart(part, min, max, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func parseFieldPart(s string, min, max int, out fieldSet) error {
	// Range: a-b or a-b/n
	var step int = 1
	if strings.Contains(s, "-") {
		idx := strings.Index(s, "/")
		if idx > 0 {
			st, err := strconv.Atoi(s[idx+1:])
			if err != nil || st <= 0 {
				return fmt.Errorf("invalid step in %q", s)
			}
			step = st
			s = s[:idx]
		}
		parts := strings.SplitN(s, "-", 2)
		a, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid range start %q", parts[0])
		}
		b, err := strconv.Atoi(parts[1])
		if err != nil {
			return fmt.Errorf("invalid range end %q", parts[1])
		}
		if a < min || b > max || a > b {
			return fmt.Errorf("range out of bounds: %d-%d (expected %d-%d)", a, b, min, max)
		}
		for i := a; i <= b; i += step {
			out[i] = true
		}
		return nil
	}
	// Single value.
	v, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid value %q", s)
	}
	if v < min || v > max {
		return fmt.Errorf("value out of bounds: %d (expected %d-%d)", v, min, max)
	}
	// Normalize Sunday=7 → 0 to match time.Weekday().
	if min == 0 && max == 6 && v == 7 {
		v = 0
	}
	out[v] = true
	return nil
}
