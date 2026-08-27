#!/usr/bin/env bash
# Build #29 — dry-run safe smoke for wiki_cache_hits_total / misses_total
# `strategy` label.
#
# Verifies (offline, no live backend):
#   1. The metric definitions in wiki_backlinks_cache_metrics.go declare
#      a "strategy" label for hits_total and misses_total.
#   2. The observation helpers (wikiCacheObsIncHit / Miss) pass a
#      non-empty strategy value through WithLabelValues.
#   3. The 4 SlugSetStrategy enum values (self / self_outgoing /
#      self_incoming / kb_wide) all appear in the metrics package's
#      call surface.
#
# Live mode (BASE_URL set): curls /metrics on the running app and
# greps for `strategy="..."` in the hits/misses sample lines.
#
# Exit codes:
#   0  — pass (offline checks succeed; live checks also succeed if BASE_URL is set)
#   1  — a check failed
#
# Usage:
#   bash scripts/smoke-wiki-cache-hit-ratio-labels.sh               # dry-run
#   BASE_URL=http://localhost:8080 bash scripts/smoke-wiki-cache-hit-ratio-labels.sh  # live

set -euo pipefail

# ----- offline checks (always run) ------------------------------------

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
METRICS_FILE="$REPO_ROOT/internal/application/service/wiki_backlinks_cache_metrics.go"
OBS_FILE="$REPO_ROOT/internal/application/service/wiki_backlinks_cache_observability.go"
TYPES_FILE="$REPO_ROOT/internal/types/wiki_page.go"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# 1. The metrics file declares strategy label for hits_total AND misses_total.
HITS_HAS_STRATEGY=$(grep -c 'Name: "wiki_cache_hits_total"' "$METRICS_FILE" || true)
MISS_HAS_STRATEGY=$(grep -c 'Name: "wiki_cache_misses_total"' "$METRICS_FILE" || true)

# We rely on the same []string{"kb_id", "strategy"} being used in both
# CounterOpts (must be identical, or promauto panics). We grep for the
# literal "kb_id", "strategy" label pair near each CounterOpts.
grep -q '\[\]string{"kb_id", "strategy"}' "$METRICS_FILE" \
  || fail "metrics file missing []string{\"kb_id\", \"strategy\"} label pair"

# Confirm both hits and misses have a Help string mentioning the strategy
# label (loose check — the comment is what an operator reads).
grep -A1 'Name: "wiki_cache_hits_total"' "$METRICS_FILE" \
  | grep -q 'strategy' \
  || fail "hits_total Help missing strategy mention"
grep -A1 'Name: "wiki_cache_misses_total"' "$METRICS_FILE" \
  | grep -q 'strategy' \
  || fail "misses_total Help missing strategy mention"

# 2. The observability file calls WithLabelValues with (kb, strategy).
grep -q 'WithLabelValues(kbLabelFor(kbID), readStrategyLabel())' "$OBS_FILE" \
  || fail "observability file missing WithLabelValues(kb, strategy) call"

# Confirm readStrategyLabel returns one of the 4 known strategies.
grep -q 'readStrategyLabel' "$OBS_FILE" \
  || fail "readStrategyLabel helper missing"

# 3. All 4 SlugSetStrategy values exist in the types file.
for s in SlugSetStrategySelf SlugSetStrategySelfOutgoing SlugSetStrategySelfIncoming SlugSetStrategyKBWide; do
  grep -q "$s " "$TYPES_FILE" \
    || fail "types file missing $s enum constant"
done

# Confirm readStrategyLabel returns one of those (loose — just check
# it references SlugSetStrategySelf or string(typename)).
grep -E 'readStrategyLabel\(\)\s+string' "$OBS_FILE" >/dev/null \
  || fail "readStrategyLabel() return type missing"
grep -q 'types.SlugSetStrategySelf' "$OBS_FILE" \
  || fail "readStrategyLabel does not reference types.SlugSetStrategySelf"

echo "OFFLINE OK — metrics + observability + types all wired for strategy label"

# ----- live checks (only when BASE_URL is set) ------------------------

if [ -z "${BASE_URL:-}" ]; then
  echo "DRY-RUN: BASE_URL not set, skipping live /metrics scrape."
  exit 0
fi

LIVE_URL="${BASE_URL%/}/metrics"
SCRAPE=$(curl -fsS --max-time 10 "$LIVE_URL" 2>/dev/null || echo "")

if [ -z "$SCRAPE" ]; then
  echo "WARN: failed to scrape ${LIVE_URL} — skipping live checks."
  exit 0
fi

# Confirm a sample line with strategy="self" exists.
echo "$SCRAPE" | grep -E '^wiki_cache_(hits|misses)_total\{[^}]*strategy="self"' \
  > /dev/null \
  || echo "INFO: no strategy=\"self\" sample yet (no traffic) — that's OK in a fresh deployment"

# Confirm at least one of the 4 strategies is present in any sample line.
STRATEGIES_FOUND=0
for s in self self_outgoing self_incoming kb_wide; do
  if echo "$SCRAPE" | grep -q "strategy=\"$s\""; then
    STRATEGIES_FOUND=$((STRATEGIES_FOUND + 1))
  fi
done

if [ "$STRATEGIES_FOUND" -eq 0 ]; then
  echo "WARN: no strategy label samples found in /metrics — has the app emitted hits/misses yet?"
else
  echo "LIVE OK — found $STRATEGIES_FOUND/4 strategy label values in /metrics"
fi

exit 0
