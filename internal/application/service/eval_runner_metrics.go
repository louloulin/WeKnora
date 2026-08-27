package service

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Build #31 — EvalRunner metrics.
//
// Three counters + two histograms back the eval-system observability:
//
//   - runs_started_total: incremented every time StartRun persists a
//     new eval_runs row. Labelled by tenant_id only; cardinality is
//     bounded (<10k tenants) and the label is intentionally narrow so
//     a runaway tenant cannot create label explosion by spawning
//     thousands of runs.
//   - runs_completed_total: incremented on terminal status
//     (succeeded / failed / canceled). Labelled by outcome so the
//     panel can split green vs red vs gray.
//   - qa_evaluated_total: one increment per QA row processed. Gives
//     operators a "throughput" rate that survives whether the run is
//     10 QAs or 10k QAs. Labelled by passed — splits badcase rate
//     from total.
//   - qa_judge_duration_seconds: histogram of the judge LLM call
//     latency. Bucket boundaries picked for chat-model latency
//     (50ms → 30s).
//   - runs_duration_seconds: histogram of total run wall-clock time,
//     labelled by outcome. Useful for capacity planning.
//
// All metrics use promauto so they register with
// prometheus.DefaultRegisterer on package init.
var (
	metricEvalRunsStartedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "eval_runs_started_total",
		Help: "Total EvalRun rows accepted by StartRun, by tenant_id.",
	}, []string{"tenant_id"})

	metricEvalRunsCompletedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "eval_runs_completed_total",
		Help: "Total EvalRun rows reaching a terminal status, by tenant_id and outcome.",
	}, []string{"tenant_id", "outcome"})

	metricEvalQAEvaluatedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "eval_qa_evaluated_total",
		Help: "Total per-QA rows processed by EvalRunner, by tenant_id and passed flag.",
	}, []string{"tenant_id", "passed"})

	metricEvalQAJudgeDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "eval_qa_judge_duration_seconds",
		Help:    "Wall-clock seconds per QA judge LLM call (factuality + citation + reflection).",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30},
	}, []string{"dimension"})

	metricEvalRunsDurationSeconds = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "eval_runs_duration_seconds",
		Help:    "Wall-clock seconds per EvalRun, by terminal outcome.",
		Buckets: []float64{1, 5, 15, 60, 300, 900, 1800, 3600, 7200, 21600},
	}, []string{"outcome"})
)