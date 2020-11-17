//nolint:lll

package controllers

import (
	"context"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		JustBeforeEach(func() {
			// TODO: Find the sync Job and its PVC
		})
		It("uses the source PVC as the sync source", func() {
			// TODO: Check the sync Job to make sure it's using the source PVC
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
		JustBeforeEach(func() {
			// TODO: Find the sync Job and its PVC
		})
		It("creates a clone of the source PVC as the sync source", func() {
			// TODO: Check the sync Job to make sure it's using a PVC w/ a
			// DataSource that matches srcPVC
		})

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
				// TODO: Check the capacity, accessModes, and storageClassName
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
		JustBeforeEach(func() {
			// TODO: Find the sync Job and its PVC
		})
		It("creates a snapshot of the source PVC and restores it as the sync source", func() {
			// TODO: Check the sync Job to make sure it's using a PVC w/ a
			// DataSource that is a VolumeSnapshot
		})

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
				// TODO: Check the capacity, accessModes, and storageClassName
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
			Eventually(func() error {
				return k8sClient.Get(ctx, nameFor(svc), svc)
			}, maxWait, interval).Should(Succeed())
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
})
