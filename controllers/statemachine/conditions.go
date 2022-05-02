/*
Copyright 2022 The VolSync authors.

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

package statemachine

import (
	"github.com/go-logr/logr"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

func setConditionSyncing(r ReplicationMachine, _ logr.Logger) {
	apimeta.SetStatusCondition(r.Conditions(),
		metav1.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  metav1.ConditionTrue,
			Reason:  volsyncv1alpha1.SynchronizingReasonSync,
			Message: "Synchronization in-progress",
		})
}

func setConditionManual(r ReplicationMachine, _ logr.Logger) {
	apimeta.SetStatusCondition(r.Conditions(),
		metav1.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  metav1.ConditionFalse,
			Reason:  volsyncv1alpha1.SynchronizingReasonManual,
			Message: "Waiting for manual trigger",
		})
}

func setConditionScheduled(r ReplicationMachine, _ logr.Logger) {
	apimeta.SetStatusCondition(r.Conditions(),
		metav1.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  metav1.ConditionFalse,
			Reason:  volsyncv1alpha1.SynchronizingReasonSched,
			Message: "Waiting for next scheduled synchronization",
		})
}

func setConditionCleanup(r ReplicationMachine, _ logr.Logger) {
	apimeta.SetStatusCondition(r.Conditions(),
		metav1.Condition{
			Type:    volsyncv1alpha1.ConditionSynchronizing,
			Status:  metav1.ConditionFalse,
			Reason:  volsyncv1alpha1.SynchronizingReasonCleanup,
			Message: "Cleaning up",
		})
}
