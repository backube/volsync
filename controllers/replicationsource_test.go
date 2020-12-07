//nolint:lll

package controllers

import (
	"context"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var _ = Describe("ReplicationSource", func() {
	var ctx = context.Background()
	var namespace *corev1.Namespace
	var rs *scribev1alpha1.ReplicationSource
	var srcPVC *corev1.PersistentVolumeClaim
	srcPVCCapacity := resource.MustParse("7Gi")

	BeforeEach(func() {
		// Each test is run in its own namespace
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "scribe-test-",
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
				AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: srcPVCCapacity,
					},
				},
			},
		}

		// Scaffold the ReplicationSource, but don't create so that it can
		// be customized per test scenario.
		rs = &scribev1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "instance",
				Namespace: namespace.Name,
			},
			Spec: scribev1alpha1.ReplicationSourceSpec{
				SourcePVC: srcPVC.Name,
			},
		}
		RsyncContainerImage = DefaultRsyncContainerImage
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
			inst := &scribev1alpha1.ReplicationSource{}
			return k8sClient.Get(ctx, nameFor(rs), inst)
		}, maxWait, interval).Should(Succeed())
	})

	Context("when an external replication method is specified", func() {
		BeforeEach(func() {
			rs.Spec.External = &scribev1alpha1.ReplicationSourceExternalSpec{}
		})
		It("the CR is not reconciled", func() {
			Consistently(func() *scribev1alpha1.ReplicationSourceStatus {
				Expect(k8sClient.Get(ctx, nameFor(rs), rs)).To(Succeed())
				return rs.Status
			}, duration, interval).Should(BeNil())
		})
	})

	Context("when a schedule is specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &scribev1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: scribev1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: scribev1alpha1.CopyMethodNone,
				},
			}
			schedule := "* * * * *"
			rs.Spec.Trigger = &scribev1alpha1.ReplicationSourceTriggerSpec{
				Schedule: &schedule,
			}
		})
		It("the next sync time is set in .status.nextSyncTime", func() {
			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, nameFor(rs), rs)).To(Succeed())
				if rs.Status == nil || rs.Status.NextSyncTime.IsZero() {
					return false
				}
				return true
			}, maxWait, interval).Should(BeTrue())
		})
	})

	Context("when a schedule is not specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &scribev1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: scribev1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: scribev1alpha1.CopyMethodNone,
				},
			}
		})
		It("the next sync time is nil", func() {
			Consistently(func() bool {
				Expect(k8sClient.Get(ctx, nameFor(rs), rs)).To(Succeed())
				if rs.Status == nil || rs.Status.NextSyncTime.IsZero() {
					return false
				}
				return true
			}, duration, interval).Should(BeFalse())
		})
	})

	Context("when a copyMethod of None is specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &scribev1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: scribev1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: scribev1alpha1.CopyMethodNone,
				},
			}
		})
		It("uses the source PVC as the sync source", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
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

	Context("when a copyMethod of Clone is specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &scribev1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: scribev1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: scribev1alpha1.CopyMethodClone,
				},
			}
		})
		It("creates a clone of the source PVC as the sync source", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
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
			Expect(k8sClient.Get(ctx, nameFor(pvc), pvc)).To(Succeed())
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
					return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
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
				Expect(k8sClient.Get(ctx, nameFor(pvc), pvc)).To(Succeed())
				Expect(pvc).To(beOwnedBy(rs))
				Expect(pvc.Spec.AccessModes).To(ConsistOf(newAccessModes))
				Expect(*pvc.Spec.Resources.Requests.Storage()).To(Equal(newCapacity))
				Expect(*pvc.Spec.StorageClassName).To(Equal(newSC))
			})
		})
	})

	Context("when a copyMethod of Snapshot is specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &scribev1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: scribev1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: scribev1alpha1.CopyMethodSnapshot,
				},
			}
		})
		XIt("creates a snapshot of the source PVC and restores it as the sync source", func() {
			// Reconcile waits until the Snap is bound before creating the PVC &
			// Job, so we need to fake the binding
			snap := &snapv1.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scribe-rsync-src-" + rs.Name,
					Namespace: rs.Namespace,
				},
			}
			Eventually(func() error {
				return k8sClient.Get(ctx, nameFor(snap), snap)
			}, maxWait, interval).Should(Succeed())
			foo := "foo"
			snap.Status = &snapv1.VolumeSnapshotStatus{
				BoundVolumeSnapshotContentName: &foo,
			}
			Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())
			// Continue checking
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
			}, maxWait, interval).Should(Succeed())
			volumes := job.Spec.Template.Spec.Volumes
			pvc := &corev1.PersistentVolumeClaim{}
			pvc.Namespace = rs.Namespace
			for _, v := range volumes {
				if v.PersistentVolumeClaim != nil && v.Name == dataVolumeName {
					pvc.Name = v.PersistentVolumeClaim.ClaimName
				}
			}
			Expect(k8sClient.Get(ctx, nameFor(pvc), pvc)).To(Succeed())
			// XXX: Why doesn't the following pass?
			Expect(pvc.Spec.DataSource).NotTo(BeNil())
			Expect(pvc.Spec.DataSource.Kind).To(Equal("VolumeSnapshot"))
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
			It("new PVC has overridden values", func() {
				// Reconcile waits until the Snap is bound before creating the PVC &
				// Job, so we need to fake the binding
				snap := &snapv1.VolumeSnapshot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "scribe-rsync-src-" + rs.Name,
						Namespace: rs.Namespace,
					},
				}
				Eventually(func() error {
					return k8sClient.Get(ctx, nameFor(snap), snap)
				}, maxWait, interval).Should(Succeed())
				foo := "foo2"
				snap.Status = &snapv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: &foo,
				}
				Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())
				// Continue checking
				job := &batchv1.Job{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
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
				Expect(k8sClient.Get(ctx, nameFor(pvc), pvc)).To(Succeed())
				Expect(pvc).To(beOwnedBy(rs))
				Expect(pvc.Spec.AccessModes).To(ConsistOf(newAccessModes))
				Expect(*pvc.Spec.Resources.Requests.Storage()).To(Equal(newCapacity))
				Expect(*pvc.Spec.StorageClassName).To(Equal(newSC))
			})
		})
	})

	Context("rsync: when no remote address is specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &scribev1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: scribev1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: scribev1alpha1.CopyMethodClone,
				},
			}
		})
		It("Creates a Service for incoming connections", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scribe-rsync-src-" + rs.Name,
					Namespace: rs.Namespace,
				},
			}
			By("creating a service")
			Eventually(func() error {
				return k8sClient.Get(ctx, nameFor(svc), svc)
			}, maxWait, interval).Should(Succeed())
			By("making the service addr available in the CR status")
			Eventually(func() *string {
				_ = k8sClient.Get(ctx, nameFor(rs), rs)
				if rs.Status == nil || rs.Status.Rsync == nil {
					return nil
				}
				return rs.Status.Rsync.Address
			}, maxWait, interval).Should(Not(BeNil()))
			Expect(*rs.Status.Rsync.Address).To(Equal(svc.Spec.ClusterIP))
		})
	})
	Context("rsync: when a remote address is specified", func() {
		BeforeEach(func() {
			remoteAddr := "my.remote.host.com"
			rs.Spec.Rsync = &scribev1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: scribev1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: scribev1alpha1.CopyMethodClone,
				},
				Address: &remoteAddr,
			}
		})
		It("No Service is created", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scribe-rsync-src-" + rs.Name,
					Namespace: rs.Namespace,
				},
			}
			Consistently(func() error {
				return k8sClient.Get(ctx, nameFor(svc), svc)
			}, duration, interval).Should(Not(Succeed()))
		})
	})

	Context("rsync: when no key is provided", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &scribev1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: scribev1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: scribev1alpha1.CopyMethodClone,
				},
			}
		})
		It("generates ssh keys automatically", func() {
			secret := &corev1.Secret{}
			Eventually(func() *scribev1alpha1.ReplicationSourceStatus {
				_ = k8sClient.Get(ctx, nameFor(rs), rs)
				return rs.Status
			}, maxWait, interval).ShouldNot(BeNil())
			Eventually(func() *scribev1alpha1.ReplicationSourceRsyncStatus {
				_ = k8sClient.Get(ctx, nameFor(rs), rs)
				return rs.Status.Rsync
			}, maxWait, interval).Should(Not(BeNil()))
			Eventually(func() *string {
				_ = k8sClient.Get(ctx, nameFor(rs), rs)
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
		var secret *v1.Secret
		BeforeEach(func() {
			secret = &v1.Secret{
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
			rs.Spec.Rsync = &scribev1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: scribev1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: scribev1alpha1.CopyMethodClone,
				},
				SSHKeys: &secret.Name,
			}
		})
		It("they are used by the sync Job", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      "scribe-rsync-src-" + rs.Name,
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
