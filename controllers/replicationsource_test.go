//nolint:lll

package controllers

import (
	"context"
	"strconv"
	"time"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/utils"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

//nolint:dupl
var _ = Describe("Source trigger", func() {
	var rs *volsyncv1alpha1.ReplicationSource
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	BeforeEach(func() {
		rs = &volsyncv1alpha1.ReplicationSource{
			Status: &volsyncv1alpha1.ReplicationSourceStatus{},
		}
	})

	Context("When a schedule is specified", func() {
		var schedule = "0 */2 * * *"
		metrics := newVolSyncMetrics(prometheus.Labels{"obj_name": "a", "obj_namespace": "b", "role": "c", "method": "d"})
		BeforeEach(func() {
			rs.Spec.Trigger = &volsyncv1alpha1.ReplicationSourceTriggerSpec{
				Schedule: &schedule,
			}
		})
		It("if never synced, sync now", func() {
			rs.Status.LastSyncTime = nil
			b, e := awaitNextSyncSource(rs, metrics, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
			Expect(rs.Status.NextSyncTime).To(Not(BeNil()))
		})
		It("if synced long ago, sync now", func() {
			when := metav1.Time{Time: time.Now().Add(-5 * time.Hour)}
			rs.Status.LastSyncTime = &when
			b, e := awaitNextSyncSource(rs, metrics, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
			Expect(rs.Status.NextSyncTime).To(Not(BeNil()))
		})
		It("if recently synced, wait", func() {
			when := metav1.Time{Time: time.Now().Add(-1 * time.Minute)}
			rs.Status.LastSyncTime = &when
			b, e := awaitNextSyncSource(rs, metrics, logger)
			Expect(b).To(BeFalse())
			Expect(e).To(BeNil())
			Expect(rs.Status.NextSyncTime).To(Not(BeNil()))
		})
	})

	Context("When a manual trigger is specified", func() {
		BeforeEach(func() {
			rs.Spec.Trigger = &volsyncv1alpha1.ReplicationSourceTriggerSpec{
				Manual: "1",
			}
		})
		metrics := newVolSyncMetrics(prometheus.Labels{"obj_name": "a", "obj_namespace": "b", "role": "c", "method": "d"})
		It("if never synced a manual trigger, sync now", func() {
			rs.Status.LastManualSync = ""
			b, e := awaitNextSyncSource(rs, metrics, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
			Expect(rs.Status.NextSyncTime).To(BeNil())
		})
		It("if already synced the current manual trigger, stay idle", func() {
			rs.Status.LastManualSync = rs.Spec.Trigger.Manual
			b, e := awaitNextSyncSource(rs, metrics, logger)
			Expect(b).To(BeFalse())
			Expect(e).To(BeNil())
			Expect(rs.Status.NextSyncTime).To(BeNil())
		})
		It("if already synced a previous manual trigger, sync now", func() {
			rs.Status.LastManualSync = "2"
			b, e := awaitNextSyncSource(rs, metrics, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
			Expect(rs.Status.NextSyncTime).To(BeNil())
		})
	})

	Context("When the trigger is empty", func() {
		metrics := newVolSyncMetrics(prometheus.Labels{"obj_name": "a", "obj_namespace": "b", "role": "c", "method": "d"})
		It("if never synced, sync now", func() {
			rs.Status.LastSyncTime = nil
			b, e := awaitNextSyncSource(rs, metrics, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
			Expect(rs.Status.NextSyncTime).To(BeNil())
		})
		It("if synced long ago, sync now", func() {
			when := metav1.Time{Time: time.Now().Add(-5 * time.Hour)}
			rs.Status.LastSyncTime = &when
			b, e := awaitNextSyncSource(rs, metrics, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
			Expect(rs.Status.NextSyncTime).To(BeNil())
		})
		It("if recently synced, sync now", func() {
			when := metav1.Time{Time: time.Now().Add(-1 * time.Minute)}
			rs.Status.LastSyncTime = &when
			b, e := awaitNextSyncSource(rs, metrics, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
			Expect(rs.Status.NextSyncTime).To(BeNil())
		})
	})
})

var _ = Describe("ReplicationSource", func() {
	var ctx = context.Background()
	var namespace *corev1.Namespace
	var rs *volsyncv1alpha1.ReplicationSource
	var srcPVC *corev1.PersistentVolumeClaim
	srcPVCCapacity := resource.MustParse("7Gi")

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
		rs = &volsyncv1alpha1.ReplicationSource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "instance",
				Namespace: namespace.Name,
			},
			Spec: volsyncv1alpha1.ReplicationSourceSpec{
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
			inst := &volsyncv1alpha1.ReplicationSource{}
			return k8sClient.Get(ctx, utils.NameFor(rs), inst)
		}, maxWait, interval).Should(Succeed())
	})

	Context("when an external replication method is specified", func() {
		BeforeEach(func() {
			rs.Spec.External = &volsyncv1alpha1.ReplicationSourceExternalSpec{}
		})
		It("the CR is not reconciled", func() {
			Consistently(func() *volsyncv1alpha1.ReplicationSourceStatus {
				Expect(k8sClient.Get(ctx, utils.NameFor(rs), rs)).To(Succeed())
				return rs.Status
			}, duration, interval).Should(BeNil())
		})
	})

	Context("when a schedule is specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodNone,
				},
			}
			schedule := "* * * * *"
			rs.Spec.Trigger = &volsyncv1alpha1.ReplicationSourceTriggerSpec{
				Schedule: &schedule,
			}
		})
		It("the next sync time is set in .status.nextSyncTime", func() {
			Eventually(func() bool {
				Expect(k8sClient.Get(ctx, utils.NameFor(rs), rs)).To(Succeed())
				if rs.Status == nil || rs.Status.NextSyncTime.IsZero() {
					return false
				}
				return true
			}, maxWait, interval).Should(BeTrue())
		})
	})

	Context("when a schedule is not specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodNone,
				},
			}
		})
		It("the next sync time is nil", func() {
			Consistently(func() bool {
				Expect(k8sClient.Get(ctx, utils.NameFor(rs), rs)).To(Succeed())
				if rs.Status == nil || rs.Status.NextSyncTime.IsZero() {
					return false
				}
				return true
			}, duration, interval).Should(BeFalse())
		})
	})

	Context("when a copyMethod of None is specified", func() {
		BeforeEach(func() {
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodNone,
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
			Expect(k8sClient.Get(ctx, utils.NameFor(pvc), pvc)).To(Succeed())
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
				Expect(k8sClient.Get(ctx, utils.NameFor(pvc), pvc)).To(Succeed())
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
				return k8sClient.Get(ctx, utils.NameFor(snap), snap)
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
			Expect(k8sClient.Get(ctx, utils.NameFor(pvc), pvc)).To(Succeed())
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
						Name:      "volsync-src-" + rs.Name,
						Namespace: rs.Namespace,
					},
				}
				Eventually(func() error {
					return k8sClient.Get(ctx, utils.NameFor(snap), snap)
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
				Expect(k8sClient.Get(ctx, utils.NameFor(pvc), pvc)).To(Succeed())
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
				return k8sClient.Get(ctx, utils.NameFor(svc), svc)
			}, maxWait, interval).Should(Succeed())
			By("making the service addr available in the CR status")
			Eventually(func() *string {
				_ = k8sClient.Get(ctx, utils.NameFor(rs), rs)
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
				return k8sClient.Get(ctx, utils.NameFor(svc), svc)
			}, duration, interval).Should(Not(Succeed()))
		})
	})

	Context("when a port is defined", func() {
		BeforeEach(func() {
			remotePort := int32(2222)
			remoteAddr := "my.remote.host.com"
			rs.Spec.Rsync = &volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod: volsyncv1alpha1.CopyMethodClone,
				},
				Address: &remoteAddr,
				Port:    &remotePort,
			}
		})
		It("an environment variable is created", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-src-" + rs.Name, Namespace: rs.Namespace}, job)
			}, maxWait, interval).Should(Succeed())
			port := job.Spec.Template.Spec.Containers[0].Env
			remotePort := strconv.Itoa(int(2222))
			found := false
			for _, v := range port {
				if v.Value == remotePort {
					found = true
				}
			}
			Expect(found).To(BeTrue())
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
				_ = k8sClient.Get(ctx, utils.NameFor(rs), rs)
				return rs.Status
			}, maxWait, interval).ShouldNot(BeNil())
			Eventually(func() *volsyncv1alpha1.ReplicationSourceRsyncStatus {
				_ = k8sClient.Get(ctx, utils.NameFor(rs), rs)
				return rs.Status.Rsync
			}, maxWait, interval).Should(Not(BeNil()))
			Eventually(func() *string {
				_ = k8sClient.Get(ctx, utils.NameFor(rs), rs)
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
