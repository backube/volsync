/*
Copyright 2020 The Scribe authors.

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

package controllers

import (
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	metricsNamespace = "scribe"
)

// scribeMetrics holds references to fully qualified instances of the metrics
type scribeMetrics struct {
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

func newScribeMetrics(labels prometheus.Labels) scribeMetrics {
	return scribeMetrics{
		MissedIntervals: missedIntervals.With(labels),
		OutOfSync:       outOfSync.With(labels),
		SyncDurations:   syncDurations.With(labels),
	}
}

func init() {
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(missedIntervals, outOfSync, syncDurations)
}

func nameFor(obj metav1.Object) types.NamespacedName {
	return types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
}

// reconcileFunc is a function that partially reconciles an object. It returns a
// bool indicating whether reconciling should continue and an error.
type reconcileFunc func(logr.Logger) (bool, error)

// reconcileBatch steps through a list of reconcile functions until one returns
// false or an error.
func reconcileBatch(l logr.Logger, reconcileFuncs ...reconcileFunc) (bool, error) {
	for _, f := range reconcileFuncs {
		if cont, err := f(l); !cont || err != nil {
			return cont, err
		}
	}
	return true, nil
}
