//go:build !disable_kopia

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

package kopia

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Kopia Maintenance Metrics", func() {
	var (
		manager *MaintenanceManager
		ctx     context.Context
		source  *volsyncv1alpha1.ReplicationSource
	)

	BeforeEach(func() {
		ctx = context.Background()
		logger := zap.New(zap.UseDevMode(true))

		// Create a fake client
		client := fake.NewClientBuilder().Build()

		// Create maintenance manager
		manager = NewMaintenanceManager(client, logger, "quay.io/backube/volsync-kopia:latest", nil)

		// Create a test ReplicationSource
		// enabled := true // Field removed
		source = &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-source",
				Namespace: "test-namespace",
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
				Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
					Repository: "test-repo-secret",
					// MaintenanceCronJob removed - use KopiaMaintenance CRD
				},
			},
		}

		// Mark ctx as used
		_ = ctx
	})

	Describe("CronJob Metrics", func() {
		It("should record metrics when creating a CronJob", func() {
			// Record creation metric
			manager.recordCronJobMetric(source, "created")

			// Get the metric value
			metric := &dto.Metric{}
			labels := prometheus.Labels{
				"obj_name":      "test-source",
				"obj_namespace": "test-namespace",
				"role":          "source",
				"operation":     "maintenance",
				"repository":    "test-repo-secret",
			}

			err := manager.metrics.MaintenanceCronJobCreated.With(labels).Write(metric)
			Expect(err).NotTo(HaveOccurred())
			Expect(metric.Counter.GetValue()).To(BeNumerically(">=", 1))
		})

		It("should record metrics when updating a CronJob", func() {
			// Record update metric
			manager.recordCronJobMetric(source, "updated")

			// Get the metric value
			metric := &dto.Metric{}
			labels := prometheus.Labels{
				"obj_name":      "test-source",
				"obj_namespace": "test-namespace",
				"role":          "source",
				"operation":     "maintenance",
				"repository":    "test-repo-secret",
			}

			err := manager.metrics.MaintenanceCronJobUpdated.With(labels).Write(metric)
			Expect(err).NotTo(HaveOccurred())
			Expect(metric.Counter.GetValue()).To(BeNumerically(">=", 1))
		})

		It("should record metrics when deleting a CronJob", func() {
			// Record deletion metric
			manager.recordCronJobDeletionMetric("test-namespace", "test-cronjob")

			// Get the metric value
			metric := &dto.Metric{}
			labels := prometheus.Labels{
				"obj_name":      "test-cronjob",
				"obj_namespace": "test-namespace",
				"role":          "source",
				"operation":     "maintenance",
				"repository":    "",
			}

			err := manager.metrics.MaintenanceCronJobDeleted.With(labels).Write(metric)
			Expect(err).NotTo(HaveOccurred())
			Expect(metric.Counter.GetValue()).To(BeNumerically(">=", 1))
		})
	})

	Describe("Maintenance Status Metrics", func() {
		It("should update metrics based on maintenance status", func() {
			durationStr := "120s"
			status := &MaintenanceStatus{
				Configured:               true,
				LastSuccessfulTime:       &metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
				FailuresSinceLastSuccess: 2,
				LastMaintenanceDuration:  &durationStr,
			}

			// Update metrics
			manager.updateMaintenanceMetrics(source, status)

			// Check last run timestamp
			metric := &dto.Metric{}
			labels := prometheus.Labels{
				"obj_name":      "test-source",
				"obj_namespace": "test-namespace",
				"role":          "source",
				"operation":     "maintenance",
				"repository":    "test-repo-secret",
			}

			err := manager.metrics.MaintenanceLastRunTimestamp.With(labels).Write(metric)
			Expect(err).NotTo(HaveOccurred())
			Expect(metric.Gauge.GetValue()).To(BeNumerically(">", 0))
		})
	})

	Describe("Job History Analysis", func() {
		It("should correctly analyze job history", func() {
			now := time.Now()
			jobs := []batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "maintenance-job-1",
						CreationTimestamp: metav1.Time{Time: now.Add(-2 * time.Hour)}, // More recent
					},
					Status: batchv1.JobStatus{
						StartTime:      &metav1.Time{Time: now.Add(-2 * time.Hour)},
						CompletionTime: &metav1.Time{Time: now.Add(-1 * time.Hour)},
						Conditions: []batchv1.JobCondition{
							{
								Type:   batchv1.JobComplete,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "maintenance-job-2",
						CreationTimestamp: metav1.Time{Time: now.Add(-4 * time.Hour)}, // Older
					},
					Status: batchv1.JobStatus{
						StartTime:      &metav1.Time{Time: now.Add(-4 * time.Hour)},
						CompletionTime: &metav1.Time{Time: now.Add(-3 * time.Hour)},
						Conditions: []batchv1.JobCondition{
							{
								Type:    batchv1.JobFailed,
								Status:  corev1.ConditionTrue,
								Message: "Job failed due to timeout",
							},
						},
					},
				},
			}

			status := &MaintenanceStatus{}
			manager.analyzeJobHistory(jobs, status)

			// Verify analysis results
			Expect(status.LastSuccessfulTime).NotTo(BeNil())
			Expect(status.LastSuccessfulTime.Time).To(Equal(now.Add(-1 * time.Hour)))
			Expect(status.LastFailedTime).NotTo(BeNil())
			Expect(status.LastFailedTime.Time).To(Equal(now.Add(-3 * time.Hour)))
			Expect(status.FailuresSinceLastSuccess).To(Equal(0)) // Success came after failure
			Expect(status.LastMaintenanceDuration).NotTo(BeNil())
			Expect(*status.LastMaintenanceDuration).To(Equal("3600s"))
		})

		It("should count consecutive failures correctly", func() {
			now := time.Now()
			jobs := []batchv1.Job{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "maintenance-job-1",
					},
					Status: batchv1.JobStatus{
						CompletionTime: &metav1.Time{Time: now.Add(-1 * time.Hour)},
						Conditions: []batchv1.JobCondition{
							{
								Type:   batchv1.JobFailed,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "maintenance-job-2",
					},
					Status: batchv1.JobStatus{
						CompletionTime: &metav1.Time{Time: now.Add(-2 * time.Hour)},
						Conditions: []batchv1.JobCondition{
							{
								Type:   batchv1.JobFailed,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "maintenance-job-3",
					},
					Status: batchv1.JobStatus{
						CompletionTime: &metav1.Time{Time: now.Add(-3 * time.Hour)},
						Conditions: []batchv1.JobCondition{
							{
								Type:   batchv1.JobFailed,
								Status: corev1.ConditionTrue,
							},
						},
					},
				},
			}

			status := &MaintenanceStatus{}
			manager.analyzeJobHistory(jobs, status)

			// Should count all 3 failures since there's no success
			Expect(status.FailuresSinceLastSuccess).To(Equal(3))
			Expect(status.LastSuccessfulTime).To(BeNil())
		})
	})

	Describe("Next Schedule Calculation", func() {
		It("should calculate next schedule for daily cron", func() {
			lastTime := time.Now()
			nextTime := manager.calculateNextScheduledTime("0 2 * * *", lastTime)

			Expect(nextTime).NotTo(BeNil())
			expectedTime := lastTime.Add(24 * time.Hour)
			Expect(nextTime.Time).To(Equal(expectedTime))
		})

		It("should calculate next schedule for weekly cron", func() {
			lastTime := time.Now()
			nextTime := manager.calculateNextScheduledTime("0 2 * * 0", lastTime)

			Expect(nextTime).NotTo(BeNil())
			expectedTime := lastTime.Add(7 * 24 * time.Hour)
			Expect(nextTime.Time).To(Equal(expectedTime))
		})

		It("should calculate next schedule for monthly cron", func() {
			lastTime := time.Date(2025, 1, 1, 2, 0, 0, 0, time.UTC)
			nextTime := manager.calculateNextScheduledTime("0 2 1 * *", lastTime)

			Expect(nextTime).NotTo(BeNil())
			expectedTime := time.Date(2025, 2, 1, 2, 0, 0, 0, time.UTC)
			Expect(nextTime.Time).To(Equal(expectedTime))
		})
	})
})
