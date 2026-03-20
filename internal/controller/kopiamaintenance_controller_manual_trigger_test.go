/*
Copyright 2025 The VolSync authors.

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
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/go-logr/logr"
)

func TestHandleManualTrigger(t *testing.T) {
	// Setup the scheme
	s := scheme.Scheme
	_ = volsyncv1alpha1.AddToScheme(s)

	tests := []struct {
		name                 string
		maintenance          *volsyncv1alpha1.KopiaMaintenance
		existingObjects      []client.Object
		expectedRequeue      bool
		expectedRequeueAfter bool
		expectedError        bool
		expectedJobCreated   bool
		expectedSyncUpdated  bool
	}{
		{
			name: "first manual trigger creates job",
			maintenance: &volsyncv1alpha1.KopiaMaintenance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-maintenance",
					Namespace: "test-namespace",
				},
				Spec: volsyncv1alpha1.KopiaMaintenanceSpec{
					Trigger: &volsyncv1alpha1.KopiaMaintenanceTriggerSpec{
						Manual: "test-trigger-1",
					},
					Repository: volsyncv1alpha1.KopiaRepositorySpec{
						Repository: "test-secret",
					},
				},
				Status: nil, // No status yet
			},
			existingObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: "test-namespace",
					},
				},
			},
			expectedRequeue:      false,                                          // RequeueAfter is used instead of Requeue
			expectedRequeueAfter: true,                                           // Should requeue after 10 seconds to check job status
			expectedError:        false,
			expectedJobCreated:   true,
			expectedSyncUpdated:  false, // Job not completed yet
		},
		{
			name: "already processed trigger does not create job",
			maintenance: &volsyncv1alpha1.KopiaMaintenance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-maintenance",
					Namespace: "test-namespace",
				},
				Spec: volsyncv1alpha1.KopiaMaintenanceSpec{
					Trigger: &volsyncv1alpha1.KopiaMaintenanceTriggerSpec{
						Manual: "test-trigger-1",
					},
					Repository: volsyncv1alpha1.KopiaRepositorySpec{
						Repository: "test-secret",
					},
				},
				Status: &volsyncv1alpha1.KopiaMaintenanceStatus{
					LastManualSync: "test-trigger-1", // Already processed
				},
			},
			existingObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: "test-namespace",
					},
				},
			},
			expectedRequeue:      false, // Should not requeue (fixed behavior)
			expectedRequeueAfter: false,
			expectedError:        false,
			expectedJobCreated:   false,
			expectedSyncUpdated:  false,
		},
		{
			name: "new trigger after previous one creates new job",
			maintenance: &volsyncv1alpha1.KopiaMaintenance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-maintenance",
					Namespace: "test-namespace",
				},
				Spec: volsyncv1alpha1.KopiaMaintenanceSpec{
					Trigger: &volsyncv1alpha1.KopiaMaintenanceTriggerSpec{
						Manual: "test-trigger-2", // New trigger
					},
					Repository: volsyncv1alpha1.KopiaRepositorySpec{
						Repository: "test-secret",
					},
				},
				Status: &volsyncv1alpha1.KopiaMaintenanceStatus{
					LastManualSync: "test-trigger-1", // Different from current
				},
			},
			existingObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: "test-namespace",
					},
				},
			},
			expectedRequeue:      false, // RequeueAfter is used instead of Requeue
			expectedRequeueAfter: true,
			expectedError:        false,
			expectedJobCreated:   true,
			expectedSyncUpdated:  false,
		},
		{
			name: "too many failures prevents job creation",
			maintenance: &volsyncv1alpha1.KopiaMaintenance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-maintenance",
					Namespace: "test-namespace",
				},
				Spec: volsyncv1alpha1.KopiaMaintenanceSpec{
					Trigger: &volsyncv1alpha1.KopiaMaintenanceTriggerSpec{
						Manual: "test-trigger-3",
					},
					Repository: volsyncv1alpha1.KopiaRepositorySpec{
						Repository: "test-secret",
					},
				},
				Status: &volsyncv1alpha1.KopiaMaintenanceStatus{
					LastManualSync:      "test-trigger-2",
					MaintenanceFailures: 5, // Too many failures
				},
			},
			existingObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-secret",
						Namespace: "test-namespace",
					},
				},
			},
			expectedRequeue:      false, // Implementation doesn't requeue when blocked
			expectedRequeueAfter: false, // Waits for user to reset failures
			expectedError:        false, // Returns nil error to stop reconciliation
			expectedJobCreated:   false,
			expectedSyncUpdated:  true, // Should still update LastManualSync
		},
		{
			name: "missing repository secret prevents job creation",
			maintenance: &volsyncv1alpha1.KopiaMaintenance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-maintenance",
					Namespace: "test-namespace",
				},
				Spec: volsyncv1alpha1.KopiaMaintenanceSpec{
					Trigger: &volsyncv1alpha1.KopiaMaintenanceTriggerSpec{
						Manual: "test-trigger-4",
					},
					Repository: volsyncv1alpha1.KopiaRepositorySpec{
						Repository: "missing-secret",
					},
				},
				Status: nil,
			},
			existingObjects:      []client.Object{},
			expectedRequeue:      false, // RequeueAfter is used instead of Requeue
			expectedRequeueAfter: true,
			expectedError:        true,
			expectedJobCreated:   false,
			expectedSyncUpdated:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with existing objects
			objs := append(tt.existingObjects, tt.maintenance)
			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(objs...).
				WithStatusSubresource(&volsyncv1alpha1.KopiaMaintenance{}).
				Build()

			// Create reconciler
			r := &KopiaMaintenanceReconciler{
				Client:         fakeClient,
				Scheme:         s,
				Log:            logr.Discard(),
				EventRecorder:  &record.FakeRecorder{},
				containerImage: "test-image:latest",
			}

			// Call handleManualTrigger
			result, err := r.handleManualTrigger(context.Background(), tt.maintenance, logr.Discard())

			// Check error
			if (err != nil) != tt.expectedError {
				t.Errorf("handleManualTrigger() error = %v, expectedError %v", err, tt.expectedError)
			}

			// Check requeue
			if result.Requeue != tt.expectedRequeue {
				t.Errorf("handleManualTrigger() Requeue = %v, expected %v", result.Requeue, tt.expectedRequeue)
			}

			// Check requeue after
			if (result.RequeueAfter > 0) != tt.expectedRequeueAfter {
				t.Errorf("handleManualTrigger() RequeueAfter = %v, expected RequeueAfter %v",
					result.RequeueAfter, tt.expectedRequeueAfter)
			}

			// Check if job was created
			jobs := &batchv1.JobList{}
			if err := fakeClient.List(context.Background(), jobs, client.InNamespace("test-namespace")); err != nil {
				t.Fatalf("Failed to list jobs: %v", err)
			}

			jobCreated := len(jobs.Items) > 0
			if jobCreated != tt.expectedJobCreated {
				t.Errorf("Job created = %v, expected %v", jobCreated, tt.expectedJobCreated)
			}

			// Check if LastManualSync was updated
			updatedMaintenance := &volsyncv1alpha1.KopiaMaintenance{}
			if err := fakeClient.Get(context.Background(), types.NamespacedName{
				Name:      tt.maintenance.Name,
				Namespace: tt.maintenance.Namespace,
			}, updatedMaintenance); err != nil {
				t.Fatalf("Failed to get updated maintenance: %v", err)
			}

			if tt.expectedSyncUpdated {
				if updatedMaintenance.Status == nil ||
				   updatedMaintenance.Status.LastManualSync != tt.maintenance.GetManualTrigger() {
					t.Errorf("LastManualSync not updated as expected")
				}
			}
		})
	}
}

func TestJobCompletionHandling(t *testing.T) {
	// Setup the scheme
	s := scheme.Scheme
	_ = volsyncv1alpha1.AddToScheme(s)

	maintenance := &volsyncv1alpha1.KopiaMaintenance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-maintenance",
			Namespace: "test-namespace",
		},
		Spec: volsyncv1alpha1.KopiaMaintenanceSpec{
			Trigger: &volsyncv1alpha1.KopiaMaintenanceTriggerSpec{
				Manual: "test-trigger",
			},
			Repository: volsyncv1alpha1.KopiaRepositorySpec{
				Repository: "test-secret",
			},
		},
		Status: nil,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
		},
	}

	t.Run("successful job completion", func(t *testing.T) {
		// Create client with maintenance and secret
		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(maintenance, secret).
			WithStatusSubresource(&volsyncv1alpha1.KopiaMaintenance{}).
			Build()

		r := &KopiaMaintenanceReconciler{
			Client:         fakeClient,
			Scheme:         s,
			Log:            logr.Discard(),
			EventRecorder:  &record.FakeRecorder{},
			containerImage: "test-image:latest",
		}

		// First call creates the job
		result, err := r.handleManualTrigger(context.Background(), maintenance, logr.Discard())
		if err != nil {
			t.Fatalf("Unexpected error creating job: %v", err)
		}
		if result.RequeueAfter == 0 {
			t.Error("Expected RequeueAfter after job creation")
		}

		// Get the created job and mark it as succeeded
		jobs := &batchv1.JobList{}
		if err := fakeClient.List(context.Background(), jobs, client.InNamespace("test-namespace")); err != nil {
			t.Fatalf("Failed to list jobs: %v", err)
		}
		if len(jobs.Items) != 1 {
			t.Fatalf("Expected 1 job, got %d", len(jobs.Items))
		}

		job := &jobs.Items[0]
		job.Status.Succeeded = 1
		if err := fakeClient.Status().Update(context.Background(), job); err != nil {
			t.Fatalf("Failed to update job status: %v", err)
		}

		// Second call should see the succeeded job
		result, err = r.handleManualTrigger(context.Background(), maintenance, logr.Discard())
		if err != nil {
			t.Fatalf("Unexpected error after job success: %v", err)
		}

		// Should not requeue after successful completion (fixed behavior)
		if result.Requeue || result.RequeueAfter > 0 {
			t.Error("Should not requeue after successful job completion")
		}

		// Check that LastManualSync was updated
		updatedMaintenance := &volsyncv1alpha1.KopiaMaintenance{}
		if err := fakeClient.Get(context.Background(), types.NamespacedName{
			Name:      maintenance.Name,
			Namespace: maintenance.Namespace,
		}, updatedMaintenance); err != nil {
			t.Fatalf("Failed to get updated maintenance: %v", err)
		}

		if updatedMaintenance.Status == nil ||
		   updatedMaintenance.Status.LastManualSync != "test-trigger" {
			t.Error("LastManualSync should be updated after job success")
		}

		if updatedMaintenance.Status.MaintenanceFailures != 0 {
			t.Error("MaintenanceFailures should be reset after success")
		}
	})

	t.Run("failed job handling", func(t *testing.T) {
		// Reset maintenance status
		maintenance.Status = nil

		fakeClient := fake.NewClientBuilder().
			WithScheme(s).
			WithObjects(maintenance, secret).
			WithStatusSubresource(&volsyncv1alpha1.KopiaMaintenance{}).
			Build()

		r := &KopiaMaintenanceReconciler{
			Client:         fakeClient,
			Scheme:         s,
			Log:            logr.Discard(),
			EventRecorder:  &record.FakeRecorder{},
			containerImage: "test-image:latest",
		}

		// First call creates the job
		_, err := r.handleManualTrigger(context.Background(), maintenance, logr.Discard())
		if err != nil {
			t.Fatalf("Unexpected error creating job: %v", err)
		}

		// Get the created job and mark it as failed
		jobs := &batchv1.JobList{}
		if err := fakeClient.List(context.Background(), jobs, client.InNamespace("test-namespace")); err != nil {
			t.Fatalf("Failed to list jobs: %v", err)
		}

		job := &jobs.Items[0]
		job.Status.Failed = 1
		if err := fakeClient.Status().Update(context.Background(), job); err != nil {
			t.Fatalf("Failed to update job status: %v", err)
		}

		// Second call should see the failed job
		result, err := r.handleManualTrigger(context.Background(), maintenance, logr.Discard())
		if err == nil {
			t.Error("Expected error after job failure")
		}

		// Should requeue after failure (using RequeueAfter, not Requeue)
		if result.RequeueAfter == 0 {
			t.Error("Should RequeueAfter after job failure")
		}

		// Check that failure count was incremented
		updatedMaintenance := &volsyncv1alpha1.KopiaMaintenance{}
		if err := fakeClient.Get(context.Background(), types.NamespacedName{
			Name:      maintenance.Name,
			Namespace: maintenance.Namespace,
		}, updatedMaintenance); err != nil {
			t.Fatalf("Failed to get updated maintenance: %v", err)
		}

		if updatedMaintenance.Status == nil ||
		   updatedMaintenance.Status.MaintenanceFailures != 1 {
			t.Error("MaintenanceFailures should be incremented after failure")
		}

		// LastManualSync should still be updated to prevent infinite retries
		if updatedMaintenance.Status.LastManualSync != "test-trigger" {
			t.Error("LastManualSync should be updated even after failure")
		}
	})
}

func TestReconcileLoopPrevention(t *testing.T) {
	// This test verifies that the fix prevents infinite job creation
	s := scheme.Scheme
	_ = volsyncv1alpha1.AddToScheme(s)

	maintenance := &volsyncv1alpha1.KopiaMaintenance{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-maintenance",
			Namespace:  "test-namespace",
			Generation: 1,
			Finalizers: []string{"volsync.backube/kopiamaintenance-protection"},
		},
		Spec: volsyncv1alpha1.KopiaMaintenanceSpec{
			Enabled: ptr.To(true),
			Trigger: &volsyncv1alpha1.KopiaMaintenanceTriggerSpec{
				Manual: "test-trigger",
			},
			Repository: volsyncv1alpha1.KopiaRepositorySpec{
				Repository: "test-secret",
			},
		},
		Status: nil,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-namespace",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(maintenance, secret).
		WithStatusSubresource(&volsyncv1alpha1.KopiaMaintenance{}).
		Build()

	r := &KopiaMaintenanceReconciler{
		Client:         fakeClient,
		Scheme:         s,
		Log:            logr.Discard(),
		EventRecorder:  &record.FakeRecorder{},
		containerImage: "test-image:latest",
	}

	// First reconcile - should create job
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      maintenance.Name,
			Namespace: maintenance.Namespace,
		},
	}

	result, err := r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("First reconcile failed: %v", err)
	}

	// Should requeue to check job status
	if !result.Requeue && result.RequeueAfter == 0 {
		t.Error("Should requeue after creating job")
	}

	// Verify job was created
	jobs := &batchv1.JobList{}
	if err := fakeClient.List(context.Background(), jobs, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("Failed to list jobs: %v", err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("Expected 1 job after first reconcile, got %d", len(jobs.Items))
	}

	// Mark job as succeeded
	job := &jobs.Items[0]
	job.Status.Succeeded = 1
	if err := fakeClient.Status().Update(context.Background(), job); err != nil {
		t.Fatalf("Failed to update job status: %v", err)
	}

	// Second reconcile - should update status
	result, err = r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Second reconcile failed: %v", err)
	}

	// Should not requeue after processing (fixed behavior)
	if result.Requeue || result.RequeueAfter > 0 {
		t.Error("Should not requeue after processing manual trigger")
	}

	// Third reconcile - manual trigger still present but already processed
	// This simulates the bug scenario where reconciliation keeps happening
	result, err = r.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("Third reconcile failed: %v", err)
	}

	// Should not requeue (fixed behavior prevents infinite loop)
	if result.Requeue || result.RequeueAfter > 0 {
		t.Error("Should not requeue when manual trigger already processed")
	}

	// Verify no new jobs were created
	jobs = &batchv1.JobList{}
	if err := fakeClient.List(context.Background(), jobs, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("Failed to list jobs: %v", err)
	}
	if len(jobs.Items) != 1 {
		t.Errorf("No new jobs should be created, but found %d jobs", len(jobs.Items))
	}

	// Multiple additional reconciles should not create new jobs
	for i := 0; i < 5; i++ {
		result, err = r.Reconcile(context.Background(), req)
		if err != nil {
			t.Fatalf("Reconcile %d failed: %v", i+4, err)
		}
		if result.Requeue || result.RequeueAfter > 0 {
			t.Errorf("Reconcile %d: Should not requeue", i+4)
		}
	}

	// Final verification - still only 1 job
	jobs = &batchv1.JobList{}
	if err := fakeClient.List(context.Background(), jobs, client.InNamespace("test-namespace")); err != nil {
		t.Fatalf("Failed to list jobs: %v", err)
	}
	if len(jobs.Items) != 1 {
		t.Errorf("Bug not fixed: Multiple jobs created (%d) for same manual trigger", len(jobs.Items))
	}
}