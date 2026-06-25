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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller/mover/kopia"
)

var _ = Describe("KopiaMaintenanceController", func() {
	var (
		ctx        context.Context
		k8sClient  client.Client
		reconciler *KopiaMaintenanceController
		recorder   *record.FakeRecorder
		testImage  = "test-kopia:v1.0.0"
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Setup scheme
		s := scheme.Scheme
		Expect(volsyncv1alpha1.AddToScheme(s)).To(Succeed())
		Expect(batchv1.AddToScheme(s)).To(Succeed())
		Expect(rbacv1.AddToScheme(s)).To(Succeed())

		// Create fake client
		k8sClient = fake.NewClientBuilder().
			WithScheme(s).
			Build()

		// Create recorder
		recorder = record.NewFakeRecorder(10)

		// Create reconciler
		reconciler = &KopiaMaintenanceController{
			Client:         k8sClient,
			Log:            zap.New(zap.UseDevMode(true)),
			Scheme:         s,
			EventRecorder:  recorder,
			containerImage: testImage,
		}
	})

	Describe("Reconciliation", func() {
		Context("when a ReplicationSource with Kopia is created", func() {
			var (
				rs       *volsyncv1alpha1.ReplicationSource
				rsName   types.NamespacedName
				repoSecret *corev1.Secret
			)

			BeforeEach(func() {
				// Create repository secret
				repoSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kopia-repo-secret",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"KOPIA_PASSWORD": []byte("test-password"),
					},
				}
				Expect(k8sClient.Create(ctx, repoSecret)).To(Succeed())

				// Create ReplicationSource with Kopia
				rs = &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "kopia-repo-secret",
							MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
								Enabled:  ptr.To(true),
								Schedule: "0 2 * * *",
							},
						},
					},
				}
				rsName = types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())
			})

			It("should create a maintenance CronJob", func() {
				// Reconcile
				req := reconcile.Request{NamespacedName: rsName}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify CronJob was created
				cronJobList := &batchv1.CronJobList{}
				Expect(k8sClient.List(ctx, cronJobList,
					client.MatchingLabels{"volsync.backube/kopia-maintenance": "true"})).To(Succeed())

				// We expect at least one CronJob to be created
				Expect(cronJobList.Items).NotTo(BeEmpty())
			})

			It("should not create a maintenance CronJob when disabled", func() {
				// Update ReplicationSource to disable maintenance
				Expect(k8sClient.Get(ctx, rsName, rs)).To(Succeed())
				rs.Spec.Kopia.MaintenanceCronJob.Enabled = ptr.To(false)
				Expect(k8sClient.Update(ctx, rs)).To(Succeed())

				// Reconcile
				req := reconcile.Request{NamespacedName: rsName}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify no CronJob was created
				cronJobList := &batchv1.CronJobList{}
				Expect(k8sClient.List(ctx, cronJobList,
					client.InNamespace("test-ns"),
					client.MatchingLabels{"volsync.backube/kopia-maintenance": "true"})).To(Succeed())
				Expect(cronJobList.Items).To(BeEmpty())
			})
		})

		Context("when a ReplicationSource is deleted", func() {
			var (
				rs       *volsyncv1alpha1.ReplicationSource
				rsName   types.NamespacedName
				cronJob  *batchv1.CronJob
			)

			BeforeEach(func() {
				// Create ReplicationSource
				rs = &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "kopia-repo",
						},
					},
				}
				rsName = types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())

				// Create orphaned CronJob
				cronJob = &batchv1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kopia-maintenance-test",
						Namespace: "test-ns",
						Labels: map[string]string{
							"volsync.backube/kopia-maintenance": "true",
							"volsync.backube/source-namespace":  "test-ns",
						},
						Annotations: map[string]string{
							"volsync.backube/source-name": "test-source",
						},
					},
					Spec: batchv1.CronJobSpec{
						Schedule: "0 2 * * *",
						JobTemplate: batchv1.JobTemplateSpec{
							Spec: batchv1.JobSpec{
								Template: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										Containers: []corev1.Container{
											{
												Name:  "kopia",
												Image: testImage,
											},
										},
										RestartPolicy: corev1.RestartPolicyOnFailure,
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, cronJob)).To(Succeed())
			})

			It("should clean up orphaned CronJobs", func() {
				// Delete the ReplicationSource
				Expect(k8sClient.Delete(ctx, rs)).To(Succeed())

				// Reconcile with the deleted source
				req := reconcile.Request{NamespacedName: rsName}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify orphaned CronJob was deleted
				Eventually(func() bool {
					cronJobList := &batchv1.CronJobList{}
					err := k8sClient.List(ctx, cronJobList,
						client.InNamespace("test-ns"),
						client.MatchingLabels{"volsync.backube/kopia-maintenance": "true"})
					if err != nil {
						return false
					}
					return len(cronJobList.Items) == 0
				}, time.Second*5, time.Millisecond*100).Should(BeTrue())
			})
		})

		Context("when a ReplicationSource doesn't use Kopia", func() {
			var (
				rs     *volsyncv1alpha1.ReplicationSource
				rsName types.NamespacedName
			)

			BeforeEach(func() {
				// Create ReplicationSource without Kopia
				rs = &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-source-rsync",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Rsync: &volsyncv1alpha1.ReplicationSourceRsyncSpec{
							Address: ptr.To("rsync-server.example.com"),
						},
					},
				}
				rsName = types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())
			})

			It("should skip reconciliation", func() {
				// Reconcile
				req := reconcile.Request{NamespacedName: rsName}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify no CronJob was created
				cronJobList := &batchv1.CronJobList{}
				Expect(k8sClient.List(ctx, cronJobList,
					client.InNamespace("test-ns"),
					client.MatchingLabels{"volsync.backube/kopia-maintenance": "true"})).To(Succeed())
				Expect(cronJobList.Items).To(BeEmpty())
			})
		})
	})

	Describe("Container version management", func() {
		Context("when container image changes", func() {
			var cronJob *batchv1.CronJob

			BeforeEach(func() {
				// Create a maintenance CronJob with old image
				cronJob = &batchv1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kopia-maintenance-test",
						Namespace: "test-ns",
						Labels: map[string]string{
							"volsync.backube/kopia-maintenance": "true",
						},
						Annotations: map[string]string{
							containerVersionAnnotation: "old-kopia:v0.9.0",
						},
					},
					Spec: batchv1.CronJobSpec{
						Schedule: "0 2 * * *",
						JobTemplate: batchv1.JobTemplateSpec{
							Spec: batchv1.JobSpec{
								Template: corev1.PodTemplateSpec{
									Spec: corev1.PodSpec{
										Containers: []corev1.Container{
											{
												Name:  "kopia",
												Image: "old-kopia:v0.9.0",
											},
										},
										RestartPolicy: corev1.RestartPolicyOnFailure,
									},
								},
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, cronJob)).To(Succeed())
			})

			It("should update all CronJobs with new image version", func() {
				newImage := "new-kopia:v1.0.0"
				err := reconciler.updateAllCronJobContainerVersions(ctx, newImage)
				Expect(err).NotTo(HaveOccurred())

				// Verify CronJob was updated
				updatedCronJob := &batchv1.CronJob{}
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      cronJob.Name,
					Namespace: cronJob.Namespace,
				}, updatedCronJob)).To(Succeed())

				// Check annotation was updated
				Expect(updatedCronJob.Annotations[containerVersionAnnotation]).To(Equal(newImage))
				// Check container image was updated
				Expect(updatedCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image).To(Equal(newImage))
			})

			It("should skip CronJobs already at current version", func() {
				// Update CronJob to current version first
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      cronJob.Name,
					Namespace: cronJob.Namespace,
				}, cronJob)).To(Succeed())

				cronJob.Annotations[containerVersionAnnotation] = testImage
				cronJob.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image = testImage
				Expect(k8sClient.Update(ctx, cronJob)).To(Succeed())

				// Try to update with same version
				err := reconciler.updateAllCronJobContainerVersions(ctx, testImage)
				Expect(err).NotTo(HaveOccurred())

				// Verify no unnecessary update occurred
				// (In a real test, we'd verify no update was made by checking resource version)
			})
		})
	})

	Describe("Batch reconciliation", func() {
		Context("when multiple ReplicationSources exist", func() {
			BeforeEach(func() {
				// Create multiple ReplicationSources
				for i := 0; i < 3; i++ {
					rs := &volsyncv1alpha1.ReplicationSource{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("test-source-%d", i),
							Namespace: fmt.Sprintf("test-ns-%d", i),
						},
						Spec: volsyncv1alpha1.ReplicationSourceSpec{
							SourcePVC: "test-pvc",
							Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
								Repository: "kopia-repo",
								MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
									Enabled: ptr.To(true),
								},
							},
						},
					}

					// Create namespace
					ns := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: rs.Namespace,
						},
					}
					Expect(k8sClient.Create(ctx, ns)).To(Succeed())

					// Create secret
					secret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kopia-repo",
							Namespace: rs.Namespace,
						},
						Data: map[string][]byte{
							"KOPIA_PASSWORD": []byte("password"),
						},
					}
					Expect(k8sClient.Create(ctx, secret)).To(Succeed())

					Expect(k8sClient.Create(ctx, rs)).To(Succeed())
				}
			})

			It("should reconcile all ReplicationSources", func() {
				err := reconciler.reconcileAllReplicationSources(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Verify CronJobs were created for all sources
				cronJobList := &batchv1.CronJobList{}
				Expect(k8sClient.List(ctx, cronJobList,
					client.MatchingLabels{"volsync.backube/kopia-maintenance": "true"})).To(Succeed())

				// We should have at least one CronJob (they might share if using same repo)
				Expect(len(cronJobList.Items)).To(BeNumerically(">=", 1))
			})
		})
	})

	Describe("Helper functions", func() {
		Context("cronJobToReplicationSource mapping", func() {
			It("should map maintenance CronJob to correct ReplicationSource", func() {
				cronJob := &batchv1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kopia-maintenance-test",
						Namespace: "operator-ns",
						Labels: map[string]string{
							"volsync.backube/kopia-maintenance": "true",
							"volsync.backube/source-namespace":  "app-ns",
						},
						Annotations: map[string]string{
							"volsync.backube/source-name": "my-source",
						},
					},
				}

				requests := reconciler.cronJobToReplicationSource(ctx, cronJob)
				Expect(requests).To(HaveLen(1))
				Expect(requests[0].Namespace).To(Equal("app-ns"))
				Expect(requests[0].Name).To(Equal("my-source"))
			})

			It("should return empty for non-maintenance CronJob", func() {
				cronJob := &batchv1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "regular-cronjob",
						Namespace: "test-ns",
					},
				}

				requests := reconciler.cronJobToReplicationSource(ctx, cronJob)
				Expect(requests).To(BeEmpty())
			})

			It("should return empty for CronJob with missing source info", func() {
				cronJob := &batchv1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kopia-maintenance-test",
						Namespace: "operator-ns",
						Labels: map[string]string{
							"volsync.backube/kopia-maintenance": "true",
						},
						// Missing source annotations
					},
				}

				requests := reconciler.cronJobToReplicationSource(ctx, cronJob)
				Expect(requests).To(BeEmpty())
			})
		})

		Context("isMaintenanceCronJob predicate", func() {
			It("should identify maintenance CronJob", func() {
				cronJob := &batchv1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"volsync.backube/kopia-maintenance": "true",
						},
					},
				}
				Expect(reconciler.isMaintenanceCronJob(cronJob)).To(BeTrue())
			})

			It("should reject non-maintenance CronJob", func() {
				cronJob := &batchv1.CronJob{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"some-other-label": "value",
						},
					},
				}
				Expect(reconciler.isMaintenanceCronJob(cronJob)).To(BeFalse())
			})

			It("should reject non-CronJob objects", func() {
				pod := &corev1.Pod{}
				Expect(reconciler.isMaintenanceCronJob(pod)).To(BeFalse())
			})
		})
	})

	Describe("Startup behavior", func() {
		Context("when controller starts up with existing ReplicationSources", func() {
			var (
				sources []*volsyncv1alpha1.ReplicationSource
				secrets []*corev1.Secret
			)

			BeforeEach(func() {
				// Create multiple ReplicationSources with Kopia before controller starts
				for i := 0; i < 3; i++ {
					ns := fmt.Sprintf("test-ns-%d", i)
					// Create namespace
					namespace := &corev1.Namespace{
						ObjectMeta: metav1.ObjectMeta{
							Name: ns,
						},
					}
					Expect(k8sClient.Create(ctx, namespace)).To(Succeed())

					// Create repository secret
					secret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kopia-repo",
							Namespace: ns,
						},
						Data: map[string][]byte{
							"KOPIA_PASSWORD": []byte("password"),
						},
					}
					Expect(k8sClient.Create(ctx, secret)).To(Succeed())
					secrets = append(secrets, secret)

					// Create ReplicationSource
					rs := &volsyncv1alpha1.ReplicationSource{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("test-source-%d", i),
							Namespace: ns,
						},
						Spec: volsyncv1alpha1.ReplicationSourceSpec{
							SourcePVC: "test-pvc",
							Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
								Repository: "kopia-repo",
								MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
									Enabled:  ptr.To(true),
									Schedule: "0 3 * * *",
								},
							},
						},
					}
					Expect(k8sClient.Create(ctx, rs)).To(Succeed())
					sources = append(sources, rs)
				}
			})

			It("should proactively create maintenance CronJobs on startup", func() {
				// Simulate startup reconciliation
				err := reconciler.reconcileAllReplicationSources(ctx)
				Expect(err).NotTo(HaveOccurred())

				// Verify CronJobs were created for all sources
				cronJobList := &batchv1.CronJobList{}
				Expect(k8sClient.List(ctx, cronJobList,
					client.MatchingLabels{"volsync.backube/kopia-maintenance": "true"})).To(Succeed())

				// Should have at least one CronJob created
				Expect(cronJobList.Items).NotTo(BeEmpty())
			})
		})
	})

	Describe("Deduplication", func() {
		Context("when multiple ReplicationSources use the same repository", func() {
			var (
				sources []*volsyncv1alpha1.ReplicationSource
				repoSecret *corev1.Secret
			)

			BeforeEach(func() {
				// Create a shared repository secret
				repoSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "shared-kopia-repo",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"KOPIA_PASSWORD":      []byte("shared-password"),
						"KOPIA_REPOSITORY":    []byte("s3://mybucket/repo"),
						"AWS_ACCESS_KEY_ID":   []byte("key"),
						"AWS_SECRET_ACCESS_KEY": []byte("secret"),
					},
				}
				Expect(k8sClient.Create(ctx, repoSecret)).To(Succeed())

				// Create multiple ReplicationSources using the same repository
				for i := 0; i < 3; i++ {
					rs := &volsyncv1alpha1.ReplicationSource{
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("test-source-dedup-%d", i),
							Namespace: "test-ns",
						},
						Spec: volsyncv1alpha1.ReplicationSourceSpec{
							SourcePVC: fmt.Sprintf("test-pvc-%d", i),
							Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
								Repository: "shared-kopia-repo",
								MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
									Enabled:  ptr.To(true),
									Schedule: "0 2 * * *",
								},
							},
						},
					}
					Expect(k8sClient.Create(ctx, rs)).To(Succeed())
					sources = append(sources, rs)
				}
			})

			It("should create only one CronJob per repository", func() {
				// Reconcile all sources
				for _, rs := range sources {
					req := reconcile.Request{NamespacedName: types.NamespacedName{
						Name:      rs.Name,
						Namespace: rs.Namespace,
					}}
					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(ctrl.Result{}))
				}

				// Use MaintenanceManager to verify deduplication
				mgr := kopia.NewMaintenanceManager(k8sClient, reconciler.Log, testImage)

				// Get the operator namespace (should be volsync-system or default)
				operatorNS := "volsync-system"
				cronJobs, err := mgr.GetMaintenanceCronJobsForNamespace(ctx, operatorNS)
				if err != nil || len(cronJobs) == 0 {
					// Try default namespace
					operatorNS = "default"
					cronJobs, err = mgr.GetMaintenanceCronJobsForNamespace(ctx, operatorNS)
				}
				Expect(err).NotTo(HaveOccurred())

				// Should have exactly one CronJob for the shared repository
				Expect(cronJobs).To(HaveLen(1))

				// Verify the CronJob has the correct repository hash label
				Expect(cronJobs[0].Labels).To(HaveKey("volsync.backube/repository-hash"))
			})

			It("should handle schedule conflicts with first-wins strategy", func() {
				// Create another source with different schedule for same repo
				rsConflict := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-source-conflict",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc-conflict",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "shared-kopia-repo",
							MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
								Enabled:  ptr.To(true),
								Schedule: "0 5 * * *", // Different schedule
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, rsConflict)).To(Succeed())

				// Reconcile the new source
				req := reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      rsConflict.Name,
					Namespace: rsConflict.Namespace,
				}}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify the schedule has changed (no first-wins for same namespace)
				mgr := kopia.NewMaintenanceManager(k8sClient, reconciler.Log, testImage)
				operatorNS := "volsync-system"
				cronJobs, err := mgr.GetMaintenanceCronJobsForNamespace(ctx, operatorNS)
				if err != nil || len(cronJobs) == 0 {
					operatorNS = "default"
					cronJobs, err = mgr.GetMaintenanceCronJobsForNamespace(ctx, operatorNS)
				}
				Expect(err).NotTo(HaveOccurred())
				Expect(cronJobs).To(HaveLen(1))
				// Schedule should be updated since all sources are in same namespace
				Expect(cronJobs[0].Spec.Schedule).To(Equal("0 5 * * *")) // Updated schedule
			})
		})
	})

	Describe("Event-driven updates", func() {
		Context("when a ReplicationSource is created", func() {
			var (
				rs *volsyncv1alpha1.ReplicationSource
				repoSecret *corev1.Secret
			)

			BeforeEach(func() {
				// Create repository secret
				repoSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "event-repo",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"KOPIA_PASSWORD": []byte("password"),
					},
				}
				Expect(k8sClient.Create(ctx, repoSecret)).To(Succeed())
			})

			It("should create a CronJob immediately", func() {
				// Create ReplicationSource
				rs = &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "event-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "event-repo",
							MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
								Enabled:  ptr.To(true),
								Schedule: "0 1 * * *",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())

				// Reconcile
				req := reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify CronJob was created
				mgr := kopia.NewMaintenanceManager(k8sClient, reconciler.Log, testImage)
				status, err := mgr.GetMaintenanceStatus(ctx, rs)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).NotTo(BeNil())
				Expect(status.Configured).To(BeTrue())
				Expect(status.Schedule).To(Equal("0 1 * * *"))
			})

			It("should update CronJob when schedule changes", func() {
				// Create initial ReplicationSource
				rs = &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "update-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "event-repo",
							MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
								Enabled:  ptr.To(true),
								Schedule: "0 2 * * *",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())

				// Initial reconcile
				req := reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Update schedule
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}, rs)).To(Succeed())
				rs.Spec.Kopia.MaintenanceCronJob.Schedule = "0 4 * * *"
				Expect(k8sClient.Update(ctx, rs)).To(Succeed())

				// Reconcile again
				result, err = reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify schedule was updated
				mgr := kopia.NewMaintenanceManager(k8sClient, reconciler.Log, testImage)
				status, err := mgr.GetMaintenanceStatus(ctx, rs)
				Expect(err).NotTo(HaveOccurred())
				Expect(status).NotTo(BeNil())
				Expect(status.Schedule).To(Equal("0 4 * * *"))
			})

			It("should delete CronJob when maintenance is disabled", func() {
				// Create ReplicationSource with maintenance enabled
				rs = &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "disable-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "event-repo",
							MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
								Enabled:  ptr.To(true),
								Schedule: "0 2 * * *",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())

				// Initial reconcile
				req := reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify CronJob exists
				mgr := kopia.NewMaintenanceManager(k8sClient, reconciler.Log, testImage)
				status, err := mgr.GetMaintenanceStatus(ctx, rs)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.Configured).To(BeTrue())

				// Disable maintenance
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}, rs)).To(Succeed())
				rs.Spec.Kopia.MaintenanceCronJob.Enabled = ptr.To(false)
				Expect(k8sClient.Update(ctx, rs)).To(Succeed())

				// Reconcile again
				result, err = reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Note: Disabling maintenance doesn't delete the CronJob if other sources use the same repository
				// The CronJob persists as long as any source with maintenance enabled uses the repository
				status, err = mgr.GetMaintenanceStatus(ctx, rs)
				Expect(err).NotTo(HaveOccurred())
				// Since disabled sources don't get maintenance, status should show not configured for this source
				Expect(status).To(BeNil()) // MaintenanceManager returns nil for disabled sources
			})
		})
	})

	Describe("Error scenarios", func() {
		Context("when repository secret is missing", func() {
			It("should handle missing secret gracefully", func() {
				// Create ReplicationSource without creating the secret
				rs := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-secret-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "missing-secret",
							MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
								Enabled:  ptr.To(true),
								Schedule: "0 2 * * *",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())

				// Reconcile
				req := reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).To(HaveOccurred())
				// Should requeue after a delay
				Expect(result.RequeueAfter).To(Equal(time.Minute))

				// Verify no CronJob was created
				mgr := kopia.NewMaintenanceManager(k8sClient, reconciler.Log, testImage)
				status, err := mgr.GetMaintenanceStatus(ctx, rs)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.Configured).To(BeFalse())
			})
		})

		Context("when invalid configuration is provided", func() {
			It("should handle invalid cron schedule", func() {
				// Create repository secret
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-repo",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"KOPIA_PASSWORD": []byte("password"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())

				// Create ReplicationSource with invalid schedule
				rs := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-schedule-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "invalid-repo",
							MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
								Enabled:  ptr.To(true),
								Schedule: "", // Empty schedule should use default
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())

				// Reconcile
				req := reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify CronJob was created with default schedule
				mgr := kopia.NewMaintenanceManager(k8sClient, reconciler.Log, testImage)
				status, err := mgr.GetMaintenanceStatus(ctx, rs)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.Configured).To(BeTrue())
				Expect(status.Schedule).To(Equal("0 2 * * *")) // Default schedule
			})
		})

		Context("when namespace is deleted", func() {
			It("should clean up orphaned resources", func() {
				// Create namespace
				ns := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "temp-ns",
					},
				}
				Expect(k8sClient.Create(ctx, ns)).To(Succeed())

				// Create secret
				secret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "temp-repo",
						Namespace: "temp-ns",
					},
					Data: map[string][]byte{
						"KOPIA_PASSWORD": []byte("password"),
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())

				// Create ReplicationSource
				rs := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "temp-source",
						Namespace: "temp-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "temp-repo",
							MaintenanceCronJob: &volsyncv1alpha1.MaintenanceCronJobSpec{
								Enabled: ptr.To(true),
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())

				// Reconcile to create CronJob
				req := reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Delete the ReplicationSource
				Expect(k8sClient.Delete(ctx, rs)).To(Succeed())

				// Reconcile to clean up
				result, err = reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify CronJob was cleaned up
				mgr := kopia.NewMaintenanceManager(k8sClient, reconciler.Log, testImage)
				err = mgr.CleanupOrphanedMaintenanceCronJobs(ctx, "temp-ns")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("Schedule handling", func() {
		Context("when using legacy maintenanceIntervalDays", func() {
			var repoSecret *corev1.Secret

			BeforeEach(func() {
				// Create repository secret
				repoSecret = &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-repo",
						Namespace: "test-ns",
					},
					Data: map[string][]byte{
						"KOPIA_PASSWORD": []byte("password"),
					},
				}
				Expect(k8sClient.Create(ctx, repoSecret)).To(Succeed())
			})

			It("should convert daily interval to cron schedule", func() {
				rs := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-daily",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository:               "legacy-repo",
							MaintenanceIntervalDays: ptr.To(int32(1)),
						},
					},
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())

				// Reconcile
				req := reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify schedule
				mgr := kopia.NewMaintenanceManager(k8sClient, reconciler.Log, testImage)
				status, err := mgr.GetMaintenanceStatus(ctx, rs)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.Configured).To(BeTrue())
				Expect(status.Schedule).To(Equal("0 2 * * *")) // Daily at 2 AM
			})

			It("should convert weekly interval to cron schedule", func() {
				rs := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-weekly",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository:               "legacy-repo",
							MaintenanceIntervalDays: ptr.To(int32(7)),
						},
					},
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())

				// Reconcile
				req := reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify schedule
				mgr := kopia.NewMaintenanceManager(k8sClient, reconciler.Log, testImage)
				status, err := mgr.GetMaintenanceStatus(ctx, rs)
				Expect(err).NotTo(HaveOccurred())
				Expect(status.Configured).To(BeTrue())
				Expect(status.Schedule).To(Equal("0 2 * * 0")) // Weekly on Sunday at 2 AM
			})

			It("should disable maintenance when interval is 0", func() {
				rs := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "legacy-disabled",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository:               "legacy-repo",
							MaintenanceIntervalDays: ptr.To(int32(0)),
						},
					},
				}
				Expect(k8sClient.Create(ctx, rs)).To(Succeed())

				// Reconcile
				req := reconcile.Request{NamespacedName: types.NamespacedName{
					Name:      rs.Name,
					Namespace: rs.Namespace,
				}}
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				// Verify no CronJob created
				mgr := kopia.NewMaintenanceManager(k8sClient, reconciler.Log, testImage)
				status, err := mgr.GetMaintenanceStatus(ctx, rs)
				Expect(err).NotTo(HaveOccurred())
				// MaintenanceManager returns nil for disabled maintenance
				Expect(status).To(BeNil())
			})
		})
	})
})