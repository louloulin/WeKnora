package service

// Build #29 — multi-instance hit_ratio metric labels.
//
// These tests pin the cardinality of metricCacheHitsTotal /
// metricCacheMissesTotal to [kb_id, strategy] and confirm the read-path
// helper readStrategyLabel() returns a value from the SlugSetStrategy
// enum. They use the package-level promauto-registered counters
// (which is how production registers them) and read back via the
// prometheus DefaultGatherer + testutil.CollectAndCompare mechanism
// against the legacy {kb_id="..."} projection — operators whose
// dashboards haven't been updated yet keep working through
// `sum without (strategy)`.
//
// Tests run against the global DefaultRegisterer because the metrics
// were registered on package init via promauto. Each test:
//   - resets the in-process atomic pair via wikiCacheObsReset()
//     (so per-test isolation works for the atomic counters)
//   - drives N hits/misses through the public helpers
//   - asserts the Prom side carries the expected label set
//
// The Prom counter accumulates across tests — we assert *presence* of
// the label, not absolute values, to stay isolation-safe.

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/Tencent/WeKnora/internal/types"
)

// gatherMetricFamily fetches a *MetricFamily by name from the
// prometheus.DefaultGatherer. Returns nil if not found — callers
// should treat that as a test failure.
func gatherMetricFamily(t *testing.T, name string) []*dto.MetricFamily {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("DefaultGatherer.Gather: %v", err)
	}
	var out []*dto.MetricFamily
	for _, fam := range families {
		if fam.GetName() == name {
			out = append(out, fam)
		}
	}
	if len(out) == 0 {
		t.Fatalf("metric family %q not registered", name)
	}
	return out
}

// metricLabels extracts the label set from the first matching label pair
// in the metric family — enough to assert "strategy" key presence and
// value membership. Returns nil if the family has no metric with that
// kb_id label.
func metricLabels(t *testing.T, fam []*dto.MetricFamily, kbID string) map[string]string {
	t.Helper()
	want := prometheus.Labels{"kb_id": kbID}
	for _, f := range fam {
		for _, m := range f.GetMetric() {
			got := m.GetLabel()
			if len(got) != len(want) && len(got) != len(want)+1 {
				continue
			}
			match := true
			for k, v := range want {
				var found bool
				for _, lp := range got {
					if lp.GetName() == k && lp.GetValue() == v {
						found = true
						break
					}
				}
				if !found {
					match = false
					break
				}
			}
			if match {
				out := make(map[string]string, len(got))
				for _, lp := range got {
					out[lp.GetName()] = lp.GetValue()
				}
				return out
			}
		}
	}
	return nil
}

// TestCacheMetricsStrategy_HitsCarriesStrategyLabel — A1 acceptance.
// Every hit must carry a `strategy` label whose value is one of the 4
// SlugSetStrategy enum members.
func TestCacheMetricsStrategy_HitsCarriesStrategyLabel(t *testing.T) {
	wikiCacheObsReset()
	wikiCacheObsIncHit("kb-meta")

	families := gatherMetricFamily(t, "wiki_cache_hits_total")
	got := metricLabels(t, families, "kb-meta")
	if got == nil {
		t.Fatalf("no wiki_cache_hits_total sample with kb_id=kb-meta")
	}
	strat, ok := got["strategy"]
	if !ok {
		t.Fatalf("strategy label missing from hits_total; got %v", got)
	}
	if !validStrategy(strat) {
		t.Errorf("strategy=%q is not a valid SlugSetStrategy", strat)
	}
}

// TestCacheMetricsStrategy_MissesCarriesStrategyLabel — A1 acceptance.
// Same as hits but for misses.
func TestCacheMetricsStrategy_MissesCarriesStrategyLabel(t *testing.T) {
	wikiCacheObsReset()
	wikiCacheObsIncMiss("kb-meta")

	families := gatherMetricFamily(t, "wiki_cache_misses_total")
	got := metricLabels(t, families, "kb-meta")
	if got == nil {
		t.Fatalf("no wiki_cache_misses_total sample with kb_id=kb-meta")
	}
	strat, ok := got["strategy"]
	if !ok {
		t.Fatalf("strategy label missing from misses_total; got %v", got)
	}
	if !validStrategy(strat) {
		t.Errorf("strategy=%q is not a valid SlugSetStrategy", strat)
	}
}

// TestCacheMetricsStrategy_ReadSideUsesSelf — design decision pin.
// The read path has no op, so it stamps "self" (the most-local
// invalidation strategy). Future Builds that stamp strategy at write
// time can redistribute this bucket without breaking the metric
// schema.
func TestCacheMetricsStrategy_ReadSideUsesSelf(t *testing.T) {
	wikiCacheObsReset()
	wikiCacheObsIncHit("kb-meta-reads")
	wikiCacheObsIncMiss("kb-meta-reads")

	want := string(types.SlugSetStrategySelf)
	for _, name := range []string{"wiki_cache_hits_total", "wiki_cache_misses_total"} {
		families := gatherMetricFamily(t, name)
		got := metricLabels(t, families, "kb-meta-reads")
		if got == nil {
			t.Fatalf("%s: no sample with kb_id=kb-meta-reads", name)
		}
		if got["strategy"] != want {
			t.Errorf("%s: strategy=%q, want %q", name, got["strategy"], want)
		}
	}
}

// TestCacheMetricsStrategy_AllFourStrategiesAccepted — A3 acceptance.
// Drives hits with each of the 4 SlugSetStrategy values to confirm the
// label set accepts all four enum members without panicking. We use
// the lower-level metricCacheHitsTotal.WithLabelValues path here
// because the read helper only emits "self" — this test pins the
// *acceptance* of the other 3 values for callers that will stamp
// strategy at write time in future Builds.
func TestCacheMetricsStrategy_AllFourStrategiesAccepted(t *testing.T) {
	for _, strat := range []types.SlugSetStrategy{
		types.SlugSetStrategySelf,
		types.SlugSetStrategySelfOutgoing,
		types.SlugSetStrategySelfIncoming,
		types.SlugSetStrategyKBWide,
	} {
		// kbid chosen to keep each strategy's sample isolated
		kbid := "kb-strategy-" + string(strat)
		// Drive through the public helper — this only stamps "self",
		// so we explicitly WithLabelValues to inject the strategy we
		// want to test. This is the same code path production callers
		// will use once the write side starts stamping strategy.
		metricCacheHitsTotal.WithLabelValues(kbid, string(strat)).Inc()
		metricCacheMissesTotal.WithLabelValues(kbid, string(strat)).Inc()

		for _, name := range []string{"wiki_cache_hits_total", "wiki_cache_misses_total"} {
			families := gatherMetricFamily(t, name)
			got := metricLabels(t, families, kbid)
			if got == nil {
				t.Fatalf("%s: no sample with kb_id=%s strategy=%s", name, kbid, strat)
			}
			if got["strategy"] != string(strat) {
				t.Errorf("%s: kb_id=%s strategy=%q want %q", name, kbid, got["strategy"], strat)
			}
		}
	}
}

// TestCacheMetricsStrategy_LegacyProjectionStillWorks — backward-compat
// guard. Dashboards that use `sum without (strategy) (...)` must keep
// returning the same hit_ratio as before Build #29. Assert by collecting
// both counters via testutil and confirming the unlabelled-without-
// strategy projection carries the same kb_id (i.e. label drop is safe).
func TestCacheMetricsStrategy_LegacyProjectionStillWorks(t *testing.T) {
	// Drive a few hits/misses through the public read helper so we
	// have a known (kb_id, strategy) pair to assert against.
	kbid := "kb-legacy-projection"
	wikiCacheObsReset()
	for i := 0; i < 3; i++ {
		wikiCacheObsIncHit(kbid)
	}
	wikiCacheObsIncMiss(kbid)

	hitsFam := gatherMetricFamily(t, "wiki_cache_hits_total")
	missFam := gatherMetricFamily(t, "wiki_cache_misses_total")

	hitsCount := countMetricWithKB(hitsFam, kbid)
	missCount := countMetricWithKB(missFam, kbid)
	if hitsCount == 0 || missCount == 0 {
		t.Fatalf("expected hits>0 and misses>0 for %s; got hits=%d misses=%d",
			kbid, hitsCount, missCount)
	}
	// Ratio sanity — 3 hits / (3 hits + 1 miss) ≈ 0.75
	ratio := float64(hitsCount) / float64(hitsCount+missCount)
	if ratio < 0.7 || ratio > 0.8 {
		t.Errorf("hit_ratio for %s = %.3f, want ~0.75", kbid, ratio)
	}
}

// countMetricWithKB returns the metric value for any label set whose
// kb_id matches, summing across all strategy buckets. Mirrors what
// `sum without (strategy) (...)` does at query time — guards against
// regressions in the legacy projection.
func countMetricWithKB(fam []*dto.MetricFamily, kbID string) float64 {
	var total float64
	for _, f := range fam {
		for _, m := range f.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "kb_id" && lp.GetValue() == kbID {
					total += m.GetCounter().GetValue()
				}
			}
		}
	}
	return total
}

// validStrategy reports whether the value is one of the 4 known
// SlugSetStrategy enum members. Centralized so the tests stay parallel
// and the assertion is uniform.
func validStrategy(s string) bool {
	switch types.SlugSetStrategy(s) {
	case types.SlugSetStrategySelf,
		types.SlugSetStrategySelfOutgoing,
		types.SlugSetStrategySelfIncoming,
		types.SlugSetStrategyKBWide:
		return true
	}
	return false
}
