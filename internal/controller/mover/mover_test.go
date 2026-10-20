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

package mover

import (
	"testing"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

func TestReconcileResult(t *testing.T) {
	positiveRetryAfter := 5 * time.Second
	zeroRetryAfter := time.Duration(0)
	negativeRetryAfter := -time.Second

	tests := []struct {
		name       string
		retryAfter *time.Duration
		want       ctrl.Result
	}{
		{name: "nil", want: ctrl.Result{}},
		{
			name:       "positive",
			retryAfter: &positiveRetryAfter,
			want:       ctrl.Result{RequeueAfter: positiveRetryAfter},
		},
		{
			name:       "zero",
			retryAfter: &zeroRetryAfter,
			want:       ctrl.Result{RequeueAfter: time.Nanosecond},
		},
		{
			name:       "negative",
			retryAfter: &negativeRetryAfter,
			want:       ctrl.Result{RequeueAfter: time.Nanosecond},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := (Result{RetryAfter: tt.retryAfter}).ReconcileResult()
			if got != tt.want {
				t.Errorf("ReconcileResult() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
