//nolint:lll

package controllers

import (
	"context"
	"fmt"
	"strconv"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ReplicationSource", func() {
	var ctx = context.Background()
	var namespace *corev1.Namespace
	var rs *volsyncv1alpha1.ReplicationSource
	var srcPVC *corev1.PersistentVolumeClaim
	srcPVCCapacity := resource.MustParse("7Gi")
	envvarDestinationAddress := "DESTINATION_ADDRESS"
	envvarDestinationPort := "DESTINATION_PORT"

	BeforeEach(func() {
		// Each test is run in its own namespace
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "volsync-test-",
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		Expect(namespace.Name).NotTo(BeEmpty())

		srcPVC = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "thesource",
				Namespace: namespace.Name,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: srcPVCCapacity,
					},
				},
			},
		}

		// Scaffold the ReplicationSource, but don't create so that it can
		// be customized per test scenario.
		rs = &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "instance",
				Namespace: namespace.Name,
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
				SourcePVC: srcPVC.Name,
			},
		}
	})
	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})
	JustBeforeEach(func() {
		Expect(k8sClient.Create(ctx, srcPVC)).To(Succeed())
		// ReplicationSource should have been customized in the BeforeEach
		// at each level, so now we create it.
		Expect(k8sClient.Create(ctx, rs)).To(Succeed())
		// Wait for it to show up in the API server
		Eventually(func() error {
			inst := &volsyncv1alpha1.ReplicationSource{}
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), inst)
		}, maxWait, interval).Should(Succeed())
	})

	Context("when an external replication method is specified", func() {
		BeforeEach(func() {
			rs.Spec.External = &volsyncv1alpha1.ReplicationSourceExternalSpec{}
		})
		It("the CR is not reconciled", func() {
			Consistently(func() *volsyncv1alpha1.ReplicationSourceStatus {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)).To(Succeed())
				return rs.Status
			}, duration, interval).Should(BeNil())
		})
	})

	Context("when no replication method is specified", func() {
		It("the CR is reports an error in the status", func() {
			Eventually(func() *volsyncv1alpha1.ReplicationSourceStatus {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)).To(Succeed())
				return rs.Status
			}, duration, interval).ShouldNot(BeNil())
			Expect(len(rs.Status.Conditions)).To(Equal(1))
			errCond := rs.Status.Conditions[0]
			Expect(errCond.Type).To(Equal(volsyncv1alpha1.ConditionSynchronizing))
			Expect(errCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(errCond.Reason).To(Equal(volsyncv1alpha1.SynchronizingReasonError))
			Expect(errCond.Message).To(ContainSubstring("a replication method must be specified"))
		})
	})

	directCopyMethodTypes := []volsyncv1alpha1.CopyMethodType{
		volsyncv1alpha1.CopyMethodNone,
		volsyncv1alpha1.CopyMethodDirect,
	}
	for i := range directCopyMethodTypes {
		// Test both None and Direct (results should be the same)
		Context(fmt.Sprintf("when a copyMethod of %s is specified", directCopyMethodTypes[i]), func() {
			directCopyMethodType := directCopyMethodTypes[i]
			BeforeEach(func() {
				rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
					ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
						CopyMethod: directCopyMethodType,
					},
				}
			})
			It("uses the source PVC as the sync source", func() {
				job := &batchv1.Job{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
				}, maxWait, interval).Should(Succeed())
				volumes := job.Spec.Template.Spec.Volumes
				found := false
				for _, v := range volumes {
					if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == srcPVC.Name {
						found = true
					}
				}
				Expect(found).To(BeTrue())
				Expect(srcPVC).NotTo(beOwnedBy(rs))
			})
		})
	}

	Context("when a copyMethod of Clone is specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodClone,
				},
			}
		})
		//nolint:dupl
		It("creates a clone of the source PVC as the sync source", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
			}, maxWait, interval).Should(Succeed())
			volumes := job.Spec.Template.Spec.Volumes
			pvc := &corev1.PersistentVolumeClaim{}
			pvc.Namespace = rs.Namespace
			found := false
			for _, v := range volumes {
				if v.PersistentVolumeClaim != nil {
					found = true
					pvc.Name = v.PersistentVolumeClaim.ClaimName
				}
			}
			Expect(found).To(BeTrue())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)).To(Succeed())
			Expect(pvc.Spec.DataSource.Name).To(Equal(srcPVC.Name))
			Expect(pvc).To(beOwnedBy(rs))
		})

		//nolint:dupl
		Context("SC, capacity, accessModes can be overridden", func() {
			newSC := "mysc2"
			newCapacity := resource.MustParse("1Gi")
			newAccessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
			BeforeEach(func() {
				rs.Spec.Rsync.StorageClassName = &newSC
				rs.Spec.Rsync.Capacity = &newCapacity
				rs.Spec.Rsync.AccessModes = newAccessModes
			})
			It("cloned PVC has overridden values", func() {
				job := &batchv1.Job{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
				}, maxWait, interval).Should(Succeed())
				volumes := job.Spec.Template.Spec.Volumes
				pvc := &corev1.PersistentVolumeClaim{}
				pvc.Namespace = rs.Namespace
				found := false
				for _, v := range volumes {
					if v.PersistentVolumeClaim != nil {
						found = true
						pvc.Name = v.PersistentVolumeClaim.ClaimName
					}
				}
				Expect(found).To(BeTrue())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)).To(Succeed())
				Expect(pvc).To(beOwnedBy(rs))
				Expect(pvc.Spec.AccessModes).To(ConsistOf(newAccessModes))
				Expect(*pvc.Spec.Resources.Requests.Storage()).To(Equal(newCapacity))
				Expect(*pvc.Spec.StorageClassName).To(Equal(newSC))
			})
		})
	})

	Context("when a copyMethod of Snapshot is specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodSnapshot,
				},
			}
			// Set a schedule - manual trigger to avoid starting more than 1 sync
			rs.Spec.Trigger = &volsyncv1alpha1.ReplicationSourceTriggerSpec{
				Manual: "testtrigger",
			}
		})
		XIt("creates a snapshot of the source PVC and restores it as the sync source", func() {
			// Reconcile waits until the Snap is bound before creating the PVC &
			// Job, so we need to fake the binding
			snap := &snapv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "volsync-rsync-src-" + rs.Name,
					Namespace: rs.Namespace,
				},
			}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(snap), snap)
			}, maxWait, interval).Should(Succeed())
			foo := "foo"
			snap.Status = &snapv1.VolumeSnapshotStatus{
				BoundVolumeSnapshotContentName: &foo,
			}
			Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())
			// Continue checking
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
			}, maxWait, interval).Should(Succeed())
			volumes := job.Spec.Template.Spec.Volumes
			pvc := &corev1.PersistentVolumeClaim{}
			pvc.Namespace = rs.Namespace
			for _, v := range volumes {
				if v.PersistentVolumeClaim != nil && v.Name == dataVolumeName {
					pvc.Name = v.PersistentVolumeClaim.ClaimName
				}
			}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)).To(Succeed())
			// XXX: Why doesn't the following pass?
			Expect(pvc.Spec.DataSource).NotTo(BeNil())
			Expect(pvc.Spec.DataSource.Kind).To(Equal("VolumeSnapshot"))
			Expect(pvc).To(beOwnedBy(rs))
		})

		//nolint:dupl
		Context("When snapshot is bound correctly", func() {
			var snapshot snapv1.VolumeSnapshot
			var job *batchv1.Job

			JustBeforeEach(func() {
				// Set snapshot to be bound so the source reconcile can proceed
				snapshots := &snapv1.VolumeSnapshotList{}
				Eventually(func() []snapv1.VolumeSnapshot {
					_ = k8sClient.List(ctx, snapshots, client.InNamespace(rs.Namespace))
					return snapshots.Items
				}, maxWait, interval).Should(Not(BeEmpty()))

				// update the VS name
				snapshot = snapshots.Items[0]
				foo := "dummysourcesnapshot"
				snapshot.Status = &snapv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: &foo,
				}
				Expect(k8sClient.Status().Update(ctx, &snapshot)).To(Succeed())

				job = &batchv1.Job{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "volsync-rsync-src-" + rs.Name,
						Namespace: rs.Namespace,
					},
				}
			})

			It("creates a snapshot of the source PVC as the sync source", func() {
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
				}, maxWait, interval).Should(Succeed())
				volumes := job.Spec.Template.Spec.Volumes
				pvc := &corev1.PersistentVolumeClaim{}
				pvc.Namespace = rs.Namespace
				found := false
				for _, v := range volumes {
					if v.PersistentVolumeClaim != nil {
						found = true
						pvc.Name = v.PersistentVolumeClaim.ClaimName
					}
				}
				Expect(found).To(BeTrue())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)).To(Succeed())
				Expect(pvc.Spec.DataSource.Name).To(Equal(snapshot.Name))
				Expect(pvc.Spec.DataSource.Kind).To(Equal("VolumeSnapshot"))
				Expect(pvc).To(beOwnedBy(rs))
			})

			It("Ensure that temp VolumeSnapshot and temp PVC are deleted at the end of an iteration", func() {
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
				}, maxWait, interval).Should(Succeed())
				volumes := job.Spec.Template.Spec.Volumes
				pvc := &corev1.PersistentVolumeClaim{}
				pvc.Namespace = rs.Namespace
				found := false
				for _, v := range volumes {
					if v.PersistentVolumeClaim != nil {
						found = true
						pvc.Name = v.PersistentVolumeClaim.ClaimName
					}
				}
				Expect(found).To(BeTrue())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)).To(Succeed())

				// Force job status to succeeded
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
				}, maxWait, interval).Should(Succeed())
				// just so the tests will run for now
				job.Status.Succeeded = 1
				Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

				// Check that temp pvc is cleaned up
				Eventually(func() bool {
					pvcFoundErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)
					if kerrors.IsNotFound(pvcFoundErr) {
						return true
					}
					// PVC may be stuck because of pvc finalizer in test scenario but check it's
					// marked for deletion
					return !pvc.GetDeletionTimestamp().IsZero()
				}, maxWait, interval).Should(BeTrue())

				// Check that temp snapshot is cleaned up
				Eventually(func() bool {
					snapFoundErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(&snapshot), &snapshot)
					return kerrors.IsNotFound(snapFoundErr)
				}, maxWait, interval).Should(BeTrue())
			})

			It("Ensure lastSyncDuration is set at the end of an iteration", func() {
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
				}, maxWait, interval).Should(Succeed())
				// Job was found, so synchronization should be in-progress

				Eventually(func() *metav1.Time {
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs); err != nil {
						return nil
					}
					return rs.Status.LastSyncStartTime
				}, maxWait, interval).ShouldNot(BeNil())

				// set the job to succeed so sync can finish
				job.Status.Succeeded = 1
				Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

				// Sync should complete and last sync duration should be set
				Eventually(func() bool {
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)).To(Succeed())
					if rs.Status == nil || rs.Status.LastSyncDuration == nil {
						return false
					}
					return true
				}, maxWait, interval).Should(BeTrue())

				// Now confirm lastSyncStartTime was un-set
				Expect(rs.Status.LastSyncStartTime).Should(BeNil())
			})
		})
		//nolint:dupl
		Context("SC, capacity, accessModes can be overridden", func() {
			newSC := "mysc2"
			newCapacity := resource.MustParse("1Gi")
			newAccessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
			BeforeEach(func() {
				rs.Spec.Rsync.StorageClassName = &newSC
				rs.Spec.Rsync.Capacity = &newCapacity
				rs.Spec.Rsync.AccessModes = newAccessModes
			})
			It("new PVC has overridden values", func() {
				// Reconcile waits until the Snap is bound before creating the PVC &
				// Job, so we need to fake the binding
				snap := &snapv1.VolumeSnapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "volsync-" + rs.Name + "-src",
						Namespace: rs.Namespace,
					},
				}
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(snap), snap)
				}, maxWait, interval).Should(Succeed())
				foo := "foo2"
				snap.Status = &snapv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: &foo,
				}
				Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())
				// Continue checking
				job := &batchv1.Job{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
				}, maxWait, interval).Should(Succeed())
				volumes := job.Spec.Template.Spec.Volumes
				pvc := &corev1.PersistentVolumeClaim{}
				pvc.Namespace = rs.Namespace
				found := false
				for _, v := range volumes {
					if v.PersistentVolumeClaim != nil {
						found = true
						pvc.Name = v.PersistentVolumeClaim.ClaimName
					}
				}
				Expect(found).To(BeTrue())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)).To(Succeed())
				Expect(pvc).To(beOwnedBy(rs))
				Expect(pvc.Spec.AccessModes).To(ConsistOf(newAccessModes))
				Expect(*pvc.Spec.Resources.Requests.Storage()).To(Equal(newCapacity))
				Expect(*pvc.Spec.StorageClassName).To(Equal(newSC))
			})
		})
	})

	Context("when the value of paused is set to true", func() {
		parallelism := int32(0)
		BeforeEach(func() {
			rs.Spec.Paused = true
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodClone,
				},
			}
		})
		It("the job will create but will not run", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
			}, maxWait, interval).Should(Succeed())
			Expect(*job.Spec.Parallelism).To(Equal(parallelism))
		})
	})

	Context("rsync: when no remote address is specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodClone,
				},
			}
		})
		It("Creates a Service for incoming connections", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "volsync-rsync-src-" + rs.Name,
					Namespace: rs.Namespace,
				},
			}
			By("creating a service")
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)
			}, maxWait, interval).Should(Succeed())
			By("making the service addr available in the CR status")
			Eventually(func() *string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)
				if rs.Status == nil || rs.Status.Rsync == nil {
					return nil
				}
				return rs.Status.Rsync.Address
			}, maxWait, interval).Should(Not(BeNil()))
			Expect(*rs.Status.Rsync.Address).To(Equal(svc.Spec.ClusterIP))
		})
		It("No environment variables are set for address or port", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
			}, maxWait, interval).Should(Succeed())
			env := job.Spec.Template.Spec.Containers[0].Env
			foundPort := false
			foundAddress := false
			for _, v := range env {
				if v.Name == envvarDestinationPort {
					// Should not happen
					foundPort = true
				}
				if v.Name == envvarDestinationAddress {
					// Should not happen
					foundAddress = true
				}
			}
			Expect(foundPort).To(BeFalse())    // Port env var should not be set
			Expect(foundAddress).To(BeFalse()) // Address env var should not be set
		})
	})
	Context("rsync: when a remote address is specified", func() {
		remoteAddr := "my.remote.host.com"
		BeforeEach(func() {
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodClone,
				},
				Address: &remoteAddr,
			}
		})
		It("No Service is created", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "volsync-rsync-src-" + rs.Name,
					Namespace: rs.Namespace,
				},
			}
			Consistently(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)
			}, duration, interval).Should(Not(Succeed()))
		})
		It("an environment variable is created for address", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
			}, maxWait, interval).Should(Succeed())
			env := job.Spec.Template.Spec.Containers[0].Env
			foundPort := false
			foundAddress := false
			for _, v := range env {
				if v.Name == envvarDestinationPort {
					// Should not happen
					foundPort = true
				}
				if v.Name == envvarDestinationAddress && v.Value == remoteAddr {
					foundAddress = true
				}
			}
			Expect(foundPort).To(BeFalse()) // Port env var should not be set
			Expect(foundAddress).To(BeTrue())
		})
	})

	Context("when a port is defined", func() {
		remotePort := int32(2222)
		remoteAddr := "my.remote.host.com"
		BeforeEach(func() {
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodClone,
				},
				Address: &remoteAddr,
				Port:    &remotePort,
			}
		})
		It("an environment variable is created for port & address", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
			}, maxWait, interval).Should(Succeed())
			env := job.Spec.Template.Spec.Containers[0].Env
			remotePortStr := strconv.Itoa(int(remotePort))
			foundPort := false
			foundAddress := false
			for _, v := range env {
				if v.Name == envvarDestinationPort && v.Value == remotePortStr {
					foundPort = true
				}
				if v.Name == envvarDestinationAddress && v.Value == remoteAddr {
					foundAddress = true
				}
			}
			Expect(foundPort).To(BeTrue())
			Expect(foundAddress).To(BeTrue())
		})
	})

	Context("rsync: when no key is provided", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodClone,
				},
			}
		})
		It("generates ssh keys automatically", func() {
			secret := &corev1.Secret{}
			Eventually(func() *volsyncv1alpha1.ReplicationSourceStatus {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)
				return rs.Status
			}, maxWait, interval).ShouldNot(BeNil())
			Eventually(func() *volsyncv1alpha1.ReplicationSourceRsyncStatus {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)
				return rs.Status.Rsync
			}, maxWait, interval).Should(Not(BeNil()))
			Eventually(func() *string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), rs)
				return rs.Status.Rsync.SSHKeys
			}, maxWait, interval).Should(Not(BeNil()))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: *rs.Status.Rsync.SSHKeys,
				Namespace: rs.Namespace}, secret)).To(Succeed())
			Expect(secret.Data).To(HaveKey("destination"))
			Expect(secret.Data).To(HaveKey("destination.pub"))
			Expect(secret.Data).To(HaveKey("source.pub"))
			Expect(secret.Data).NotTo(HaveKey("source"))
			Expect(secret).To(beOwnedBy(rs))
		})
	})
	//nolint:dupl
	Context("rsync: when ssh keys are provided", func() {
		var secret *corev1.Secret
		BeforeEach(func() {
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "keys",
					Namespace: rs.Namespace,
				},
				StringData: map[string]string{
					"source":          "foo",
					"source.pub":      "bar",
					"destination.pub": "baz",
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodClone,
				},
				SSHKeys: &secret.Name,
			}
		})
		It("they are used by the sync Job", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "volsync-rsync-src-" + rs.Name,
					Namespace: rs.Namespace,
				}, job)
			}, maxWait, interval).Should(Succeed())
			volumes := job.Spec.Template.Spec.Volumes
			found := false
			for _, v := range volumes {
				if v.Secret != nil && v.Secret.SecretName == secret.Name {
					found = true
				}
			}
			Expect(found).To(BeTrue())
			Expect(secret).NotTo(beOwnedBy(rs))
		})
	})

})
