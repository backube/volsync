/*
Copyright 2020 The VolSync authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package controller

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	metricsNamespace = "volsync"
)

// volsyncMetrics holds references to fully qualified instances of the metrics
type volsyncMetrics struct {
	MissedIntervals prometheus.Counter
	OutOfSync       prometheus.Gauge
	SyncDurations   prometheus.Observer
}

var (
	metricLabels = []string{
		"obj_name",      // Name of the replication CR
		"obj_namespace", // Namespace containing the CR
		"role",          // Direction: "source" or "destination"
		"method",        // Synchronization method (rsync, rclone, etc.)
	}

	missedIntervals = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      "missed_intervals_total",
			Namespace: metricsNamespace,
			Help:      "The number of times a synchronization failed to complete before the next scheduled start",
		},
		metricLabels,
	)
	outOfSync = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      "volume_out_of_sync",
			Namespace: metricsNamespace,
			Help:      "Set to 1 if the volume is not properly synchronized",
		},
		metricLabels,
	)
	syncDurations = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "sync_duration_seconds",
			Namespace:  metricsNamespace,
			Help:       "Duration of the synchronization interval in seconds",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			MaxAge:     24 * time.Hour,
		},
		metricLabels,
	)
)

func newVolSyncMetrics(labels prometheus.Labels) volsyncMetrics {
	return volsyncMetrics{
		MissedIntervals: missedIntervals.With(labels),
		OutOfSync:       outOfSync.With(labels),
		SyncDurations:   syncDurations.With(labels),
	}
}

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(missedIntervals, outOfSync, syncDurations)
}
