/*
Copyright 2026 The VolSync authors.

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
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// collectedLabels returns the label sets of every series currently exposed by
// the supplied collector. A vec only emits a series once it has been created
// (e.g. via .With()), so an empty result means the series was removed.
func collectedLabels(t *testing.T, c prometheus.Collector) []string {
	t.Helper()
	ch := make(chan prometheus.Metric, 128)
	go func() {
		c.Collect(ch)
		close(ch)
	}()

	seen := []string{}
	for m := range ch {
		seen = append(seen, m.Desc().String())
	}
	return seen
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// TestClearVolSyncMetricsOnDelete is a regression test for issue #1194.
//
// When a ReplicationSource/ReplicationDestination is deleted, the controller's
// early NotFound return leaves the per-object gauge/counter/summary series
// behind. Previously the out-of-sync gauge stayed at 1 forever. This test
// ensures clearVolSyncMetrics() removes every series for the object.
func TestClearVolSyncMetricsOnDelete(t *testing.T) {
	labels := prometheus.Labels{
		"obj_name":      "my-rs",
		"obj_namespace": "my-ns",
		"role":          "source",
		"method":        "rsync",
	}

	// Reconcile start: series are created and the object is flagged out of sync.
	m := newVolSyncMetrics(labels)
	m.OutOfSync.Set(1)
	m.MissedIntervals.Inc()
	m.SyncDurations.Observe(1.5)

	outOfSyncBefore := collectedLabels(t, outOfSync)
	missedBefore := collectedLabels(t, missedIntervals)
	syncDurBefore := collectedLabels(t, syncDurations)

	if len(outOfSyncBefore) == 0 || len(missedBefore) == 0 || len(syncDurBefore) == 0 {
		t.Fatalf("expected metric series to exist before deleting the CR "+
			"(outOfSync=%d missedIntervals=%d syncDurations=%d)",
			len(outOfSyncBefore), len(missedBefore), len(syncDurBefore))
	}

	// The CR is deleted; clear its series.
	clearVolSyncMetrics(labels)

	if got := collectedLabels(t, outOfSync); containsAny(got, outOfSyncBefore) {
		t.Errorf("out-of-sync metric series still exported after CR deletion: %v", got)
	}
	if got := collectedLabels(t, missedIntervals); containsAny(got, missedBefore) {
		t.Errorf("missed-intervals metric series still exported after CR deletion: %v", got)
	}
	if got := collectedLabels(t, syncDurations); containsAny(got, syncDurBefore) {
		t.Errorf("sync-durations metric series still exported after CR deletion: %v", got)
	}
}

func containsAny(haystack, needles []string) bool {
	for _, n := range needles {
		if containsString(haystack, n) {
			return true
		}
	}
	return false
}
