// Package wikicachemetrics holds a small set of cross-package metric
// handles for the wiki backlinks cache subsystem.
//
// Lives outside `service` so the repository package can update the
// backref gauge without introducing an import cycle (service already
// depends on repository). The cleanup service refreshes the gauge on
// its sweep cycle; the repository maintains it incrementally on
// Upsert / Delete / DeleteByKB / DeleteStale to keep the value
// accurate between sweeps.
package wikicachemetrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// BackrefRows tracks the size of the wiki_backlinks_cache_backref
// inverted index (Build #26). The repo updates this gauge on every
// write; the cleanup service also refreshes it during its sweep
// cycle so a missed update (e.g. a rolled-back transaction) gets
// corrected at the next sweep.
var BackrefRows = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "wiki_cache_backref_rows_remaining",
	Help: "Current number of rows in the wiki_backlinks_cache_backref inverted index table.",
})