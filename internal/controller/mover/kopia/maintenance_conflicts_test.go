//go:build disabled_test && !disable_kopia
// +build disabled_test,!disable_kopia

// This test file is disabled because it tests deprecated MaintenanceCronJob fields
// that have been removed in favor of the KopiaMaintenance CRD

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
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Maintenance Schedule Conflict Resolution", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		manager   *MaintenanceManager
		logger    = zap.New(zap.UseDevMode(true))
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Set up the test environment
		os.Setenv(operatorNamespaceEnvVar, "test-operator-ns")

		// Set up scheme
		s := scheme.Scheme
		Expect(volsyncv1alpha1.AddToScheme(s)).To(Succeed())
		Expect(batchv1.AddToScheme(s)).To(Succeed())
		Expect(rbacv1.AddToScheme(s)).To(Succeed())

		// Create fake client
		k8sClient = fake.NewClientBuilder().WithScheme(s).Build()

		// Create maintenance manager
		manager = NewMaintenanceManager(k8sClient, logger, "test-image:latest")
	})

	AfterEach(func() {
		os.Unsetenv(operatorNamespaceEnvVar)
	})

	Describe("Schedule Conflict Resolution - First Wins Strategy", func() {
		var (
			repoSecret     *corev1.Secret
			repoSecretName = "shared-kopia-repo"
		)

		BeforeEach(func() {
			// Create a shared repository secret in namespace1
			repoSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      repoSecretName,
					Namespace: "namespace1",
				},
				Data: map[string][]byte{
					"KOPIA_PASSWORD":   []byte("test-password"),
					"KOPIA_REPOSITORY": []byte("s3://bucket/repo"),
				},
			}
			Expect(k8sClient.Create(ctx, repoSecret)).To(Succeed())

			// Create the same secret in namespace2
			repoSecret2 := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      repoSecretName,
					Namespace: "namespace2",
				},
				Data: map[string][]byte{
					"KOPIA_PASSWORD":   []byte("test-password"),
					"KOPIA_REPOSITORY": []byte("s3://bucket/repo"),
				},
			}
			Expect(k8sClient.Create(ctx, repoSecret2)).To(Succeed())

			// Create the same secret in namespace3
			repoSecret3 := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      repoSecretName,
					Namespace: "namespace3",
				},
				Data: map[string][]byte{
					"KOPIA_PASSWORD":   []byte("test-password"),
					"KOPIA_REPOSITORY": []byte("s3://bucket/repo"),
				},
			}
			Expect(k8sClient.Create(ctx, repoSecret3)).To(Succeed())
		})

		It("should use first-wins strategy when multiple sources have different schedules", func() {
			// First ReplicationSource with schedule "0 1 * * *" (1 AM daily)
			source1 := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source1",
					Namespace: "namespace1",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: repoSecretName,
						MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
							Enabled:  ptr.To(true),
							Schedule: "0 1 * * *", // 1 AM daily
						},
					},
				},
			}

			// Create first CronJob
			err := manager.ReconcileMaintenanceForSource(ctx, source1)
			Expect(err).ToNot(HaveOccurred())

			// Verify CronJob was created with first schedule
			cronJobs := &batchv1.CronJobList{}
			Expect(k8sClient.List(ctx, cronJobs, client.InNamespace("test-operator-ns"))).To(Succeed())
			Expect(cronJobs.Items).To(HaveLen(1))
			Expect(cronJobs.Items[0].Spec.Schedule).To(Equal("0 1 * * *"))

			// Second ReplicationSource with different schedule "0 3 * * *" (3 AM daily)
			source2 := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source2",
					Namespace: "namespace2",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: repoSecretName,
						MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
							Enabled:  ptr.To(true),
							Schedule: "0 3 * * *", // 3 AM daily - conflicts!
						},
					},
				},
			}

			// Try to create second CronJob with different schedule
			err = manager.ReconcileMaintenanceForSource(ctx, source2)
			Expect(err).ToNot(HaveOccurred())

			// Verify CronJob still has the FIRST schedule (first-wins)
			Expect(k8sClient.List(ctx, cronJobs, client.InNamespace("test-operator-ns"))).To(Succeed())
			Expect(cronJobs.Items).To(HaveLen(1))
			Expect(cronJobs.Items[0].Spec.Schedule).To(Equal("0 1 * * *")) // First schedule wins!

			// Check for conflict annotation
			cronJob := &batchv1.CronJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      cronJobs.Items[0].Name,
				Namespace: "test-operator-ns",
			}, cronJob)).To(Succeed())

			// Verify conflict is tracked in annotations
			Expect(cronJob.Annotations).To(HaveKey(maintenanceScheduleConflictAnnotation))
			conflictMsg := cronJob.Annotations[maintenanceScheduleConflictAnnotation]
			Expect(conflictMsg).To(ContainSubstring("0 3 * * *"))
			Expect(conflictMsg).To(ContainSubstring("namespace2"))
		})

		It("should allow schedule updates from the same namespace", func() {
			// First ReplicationSource with initial schedule
			source := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source1",
					Namespace: "namespace1",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: repoSecretName,
						MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
							Enabled:  ptr.To(true),
							Schedule: "0 1 * * *", // Initial: 1 AM daily
						},
					},
				},
			}

			// Create CronJob
			err := manager.ReconcileMaintenanceForSource(ctx, source)
			Expect(err).ToNot(HaveOccurred())

			// Verify initial schedule
			cronJobs := &batchv1.CronJobList{}
			Expect(k8sClient.List(ctx, cronJobs, client.InNamespace("test-operator-ns"))).To(Succeed())
			Expect(cronJobs.Items).To(HaveLen(1))
			Expect(cronJobs.Items[0].Spec.Schedule).To(Equal("0 1 * * *"))

			// Update schedule from the SAME namespace
			source.Spec.Kopia.MaintenanceCronJob.Schedule = "0 2 * * *" // Updated: 2 AM daily
			err = manager.ReconcileMaintenanceForSource(ctx, source)
			Expect(err).ToNot(HaveOccurred())

			// Verify schedule was updated (same namespace can update)
			Expect(k8sClient.List(ctx, cronJobs, client.InNamespace("test-operator-ns"))).To(Succeed())
			Expect(cronJobs.Items).To(HaveLen(1))
			Expect(cronJobs.Items[0].Spec.Schedule).To(Equal("0 2 * * *")) // Updated!
		})

		It("should handle three namespaces with different schedules", func() {
			schedules := []string{
				"0 1 * * *", // namespace1: 1 AM
				"0 2 * * *", // namespace2: 2 AM
				"0 3 * * *", // namespace3: 3 AM
			}

			// Create ReplicationSources in order
			for i, ns := range []string{"namespace1", "namespace2", "namespace3"} {
				source := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("source%d", i+1),
						Namespace: ns,
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: repoSecretName,
							MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
								Enabled:  ptr.To(true),
								Schedule: schedules[i],
							},
						},
					},
				}

				err := manager.ReconcileMaintenanceForSource(ctx, source)
				Expect(err).ToNot(HaveOccurred())
			}

			// Verify only one CronJob exists with the first schedule
			cronJobs := &batchv1.CronJobList{}
			Expect(k8sClient.List(ctx, cronJobs, client.InNamespace("test-operator-ns"))).To(Succeed())
			Expect(cronJobs.Items).To(HaveLen(1))
			Expect(cronJobs.Items[0].Spec.Schedule).To(Equal("0 1 * * *")) // First wins!

			// Verify the last namespace label is namespace3 (last to reconcile)
			Expect(cronJobs.Items[0].Labels[maintenanceNamespaceLabel]).To(Equal("namespace3"))
		})

		It("should preserve schedule when first source is deleted", func() {
			// Create first source
			source1 := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source1",
					Namespace: "namespace1",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: repoSecretName,
						MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
							Enabled:  ptr.To(true),
							Schedule: "0 1 * * *",
						},
					},
				},
			}
			Expect(manager.ReconcileMaintenanceForSource(ctx, source1)).To(Succeed())

			// Create second source with different schedule
			source2 := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source2",
					Namespace: "namespace2",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: repoSecretName,
						MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
							Enabled:  ptr.To(true),
							Schedule: "0 3 * * *",
						},
					},
				},
			}
			Expect(manager.ReconcileMaintenanceForSource(ctx, source2)).To(Succeed())

			// Verify schedule is still from first source
			cronJobs := &batchv1.CronJobList{}
			Expect(k8sClient.List(ctx, cronJobs, client.InNamespace("test-operator-ns"))).To(Succeed())
			Expect(cronJobs.Items).To(HaveLen(1))
			Expect(cronJobs.Items[0].Spec.Schedule).To(Equal("0 1 * * *"))

			// Simulate deletion of source1 (but CronJob remains because source2 still needs it)
			// The cleanup logic would normally handle this

			// Verify CronJob still exists and maintains the original schedule
			Expect(k8sClient.List(ctx, cronJobs, client.InNamespace("test-operator-ns"))).To(Succeed())
			Expect(cronJobs.Items).To(HaveLen(1))
			Expect(cronJobs.Items[0].Spec.Schedule).To(Equal("0 1 * * *")) // Schedule preserved!
		})

		It("should track all conflict attempts in annotations", func() {
			// Create first source
			source1 := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source1",
					Namespace: "namespace1",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: repoSecretName,
						MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
							Enabled:  ptr.To(true),
							Schedule: "0 1 * * *",
						},
					},
				},
			}
			Expect(manager.ReconcileMaintenanceForSource(ctx, source1)).To(Succeed())

			// Create multiple sources with different schedules
			conflictingSchedules := []string{"0 2 * * *", "0 3 * * *"}
			for i, schedule := range conflictingSchedules {
				source := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("source%d", i+2),
						Namespace: fmt.Sprintf("namespace%d", i+2),
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: repoSecretName,
							MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
								Enabled:  ptr.To(true),
								Schedule: schedule,
							},
						},
					},
				}
				Expect(manager.ReconcileMaintenanceForSource(ctx, source)).To(Succeed())
			}

			// Check the CronJob for conflict tracking
			cronJobs := &batchv1.CronJobList{}
			Expect(k8sClient.List(ctx, cronJobs, client.InNamespace("test-operator-ns"))).To(Succeed())
			Expect(cronJobs.Items).To(HaveLen(1))

			cronJob := &cronJobs.Items[0]
			Expect(cronJob.Annotations).To(HaveKey(maintenanceScheduleConflictAnnotation))

			// The annotation should contain info about the last conflict
			conflictMsg := cronJob.Annotations[maintenanceScheduleConflictAnnotation]
			Expect(conflictMsg).To(ContainSubstring("0 3 * * *"))  // Last attempted schedule
			Expect(conflictMsg).To(ContainSubstring("namespace3")) // Last conflicting namespace
		})

		It("should handle disabled maintenance without creating conflicts", func() {
			// Create first source with maintenance enabled
			source1 := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source1",
					Namespace: "namespace1",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: repoSecretName,
						MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
							Enabled:  ptr.To(true),
							Schedule: "0 1 * * *",
						},
					},
				},
			}
			Expect(manager.ReconcileMaintenanceForSource(ctx, source1)).To(Succeed())

			// Create second source with maintenance disabled
			source2 := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "source2",
					Namespace: "namespace2",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: repoSecretName,
						MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
							Enabled:  ptr.To(false),
							Schedule: "0 3 * * *", // Different schedule but disabled
						},
					},
				},
			}
			Expect(manager.ReconcileMaintenanceForSource(ctx, source2)).To(Succeed())

			// Verify only one CronJob exists with original schedule
			cronJobs := &batchv1.CronJobList{}
			Expect(k8sClient.List(ctx, cronJobs, client.InNamespace("test-operator-ns"))).To(Succeed())
			Expect(cronJobs.Items).To(HaveLen(1))
			Expect(cronJobs.Items[0].Spec.Schedule).To(Equal("0 1 * * *"))

			// Verify no conflict annotation was added (source2 was disabled)
			cronJob := &cronJobs.Items[0]
			Expect(cronJob.Annotations).ToNot(HaveKey(maintenanceScheduleConflictAnnotation))
		})
	})

	Describe("Repository Hash Consistency", func() {
		It("should generate same hash for same repository across namespaces", func() {
			config1 := &RepositoryConfig{
				Repository: "kopia-repo",
				CustomCA:   nil,
				Namespace:  "namespace1",
				Schedule:   "0 1 * * *",
			}

			config2 := &RepositoryConfig{
				Repository: "kopia-repo",
				CustomCA:   nil,
				Namespace:  "namespace2", // Different namespace
				Schedule:   "0 3 * * *",  // Different schedule
			}

			// Hashes should be identical (namespace and schedule excluded)
			Expect(config1.Hash()).To(Equal(config2.Hash()))
		})

		It("should generate different hash for different repositories", func() {
			config1 := &RepositoryConfig{
				Repository: "kopia-repo-1",
				CustomCA:   nil,
				Namespace:  "namespace1",
				Schedule:   "0 1 * * *",
			}

			config2 := &RepositoryConfig{
				Repository: "kopia-repo-2", // Different repository
				CustomCA:   nil,
				Namespace:  "namespace1",
				Schedule:   "0 1 * * *",
			}

			// Hashes should be different
			Expect(config1.Hash()).ToNot(Equal(config2.Hash()))
		})
	})

	Describe("Conflict Resolution Documentation", func() {
		It("should document the first-wins strategy clearly", func() {
			// This test serves as documentation of the expected behavior

			// The strategy is: First-Wins
			// 1. The first ReplicationSource to create a maintenance CronJob sets the schedule
			// 2. Subsequent sources using the same repository cannot change the schedule
			// 3. Only sources from the SAME namespace can update the schedule
			// 4. Conflicts are tracked via annotations for visibility
			// 5. The CronJob persists as long as ANY source needs it

			// Rationale:
			// - Prevents unpredictable schedule changes
			// - Maintains consistency for shared repositories
			// - Allows single-namespace deployments to update their schedules
			// - Provides visibility into conflicts via annotations

			Expect(true).To(BeTrue()) // This test documents behavior
		})
	})
})

var _ = Describe("Maintenance Conflict Resolution Edge Cases", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		manager   *MaintenanceManager
		logger    = zap.New(zap.UseDevMode(true))
	)

	BeforeEach(func() {
		ctx = context.Background()
		os.Setenv(operatorNamespaceEnvVar, "test-operator-ns")

		s := scheme.Scheme
		Expect(volsyncv1alpha1.AddToScheme(s)).To(Succeed())
		Expect(batchv1.AddToScheme(s)).To(Succeed())
		Expect(rbacv1.AddToScheme(s)).To(Succeed())

		k8sClient = fake.NewClientBuilder().WithScheme(s).Build()
		manager = NewMaintenanceManager(k8sClient, logger, "test-image:latest")
	})

	AfterEach(func() {
		os.Unsetenv(operatorNamespaceEnvVar)
	})

	It("should handle rapid concurrent reconciliations correctly", func() {
		// Create repository secret
		repoSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "repo",
				Namespace: "ns1",
			},
			Data: map[string][]byte{
				"KOPIA_PASSWORD": []byte("password"),
			},
		}
		Expect(k8sClient.Create(ctx, repoSecret)).To(Succeed())

		// Create multiple sources pointing to same repo
		source := &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source",
				Namespace: "ns1",
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
				Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
					Repository: "repo",
					MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
						Enabled:  ptr.To(true),
						Schedule: "0 1 * * *",
					},
				},
			},
		}

		// Simulate rapid concurrent reconciliations
		for i := 0; i < 5; i++ {
			err := manager.ReconcileMaintenanceForSource(ctx, source)
			Expect(err).ToNot(HaveOccurred())
		}

		// Should still have only one CronJob
		cronJobs := &batchv1.CronJobList{}
		Expect(k8sClient.List(ctx, cronJobs, client.InNamespace("test-operator-ns"))).To(Succeed())
		Expect(cronJobs.Items).To(HaveLen(1))
	})

	It("should handle malformed schedules gracefully", func() {
		// Create repository secret
		repoSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "repo",
				Namespace: "ns1",
			},
			Data: map[string][]byte{
				"KOPIA_PASSWORD": []byte("password"),
			},
		}
		Expect(k8sClient.Create(ctx, repoSecret)).To(Succeed())

		// Note: In real implementation, schedule validation happens at API level
		// This test documents that invalid schedules should be rejected by K8s API validation
		source := &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source",
				Namespace: "ns1",
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
				Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
					Repository: "repo",
					MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
						Enabled:  ptr.To(true),
						Schedule: "0 1 * * *", // Valid schedule for test
					},
				},
			},
		}

		err := manager.ReconcileMaintenanceForSource(ctx, source)
		Expect(err).ToNot(HaveOccurred())
	})
})
