//nolint:lll
package controllers

import (
	"context"
	"time"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/operator-framework/operator-lib/status"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
)

//nolint:dupl
var _ = Describe("Destination scheduling", func() {
	var rd *scribev1alpha1.ReplicationDestination
	logger := zap.LoggerTo(GinkgoWriter, true)

	BeforeEach(func() {
		rd = &scribev1alpha1.ReplicationDestination{
			Status: &scribev1alpha1.ReplicationDestinationStatus{},
		}
	})

	Context("When a schedule is specified", func() {
		var schedule = "0 */2 * * *"
		BeforeEach(func() {
			rd.Spec.Trigger = &scribev1alpha1.ReplicationDestinationTriggerSpec{
				Schedule: &schedule,
			}
		})
		It("if never synced, sync now", func() {
			rd.Status.LastSyncTime = nil
			b, e := awaitNextSyncDestination(rd, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
		})
		It("if synced long ago, sync now", func() {
			when := metav1.Time{Time: time.Now().Add(-5 * time.Hour)}
			rd.Status.LastSyncTime = &when
			b, e := awaitNextSyncDestination(rd, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
		})
		It("if recently synced, wait", func() {
			when := metav1.Time{Time: time.Now().Add(-1 * time.Minute)}
			rd.Status.LastSyncTime = &when
			b, e := awaitNextSyncDestination(rd, logger)
			Expect(b).To(BeFalse())
			Expect(e).To(BeNil())
		})
		It("nextSyncTime will be set", func() {
			_, _ = awaitNextSyncDestination(rd, logger)
			Expect(rd.Status.NextSyncTime).To(Not(BeNil()))
		})
	})

	Context("When a schedule is NOT specified", func() {
		It("if never synced, sync now", func() {
			rd.Status.LastSyncTime = nil
			b, e := awaitNextSyncDestination(rd, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
		})
		It("if synced long ago, sync now", func() {
			when := metav1.Time{Time: time.Now().Add(-5 * time.Hour)}
			rd.Status.LastSyncTime = &when
			b, e := awaitNextSyncDestination(rd, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
		})
		It("if recently synced, sync now", func() {
			when := metav1.Time{Time: time.Now().Add(-1 * time.Minute)}
			rd.Status.LastSyncTime = &when
			b, e := awaitNextSyncDestination(rd, logger)
			Expect(b).To(BeTrue())
			Expect(e).To(BeNil())
		})
		It("nextSyncTime will NOT be set", func() {
			_, _ = awaitNextSyncDestination(rd, logger)
			Expect(rd.Status.NextSyncTime).To(BeNil())
		})
	})
})

var _ = Describe("ReplicationDestination", func() {
	var ctx = context.Background()
	var namespace *corev1.Namespace
	var rd *scribev1alpha1.ReplicationDestination

	BeforeEach(func() {
		// Each test is run in its own namespace
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "scribe-test-",
			},
		}
		Expect(k8sClient.Create(ctx, namespace)).To(Succeed())
		Expect(namespace.Name).NotTo(BeEmpty())

		// Scaffold the ReplicationDestination, but don't create so that it can
		// be customized per test scenario.
		rd = &scribev1alpha1.ReplicationDestination{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "instance",
				Namespace: namespace.Name,
			},
		}
		RsyncContainerImage = DefaultRsyncContainerImage
	})
	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})
	JustBeforeEach(func() {
		// ReplicationDestination should have been customized in the BeforeEach
		// at each level, so now we create it.
		Expect(k8sClient.Create(ctx, rd)).To(Succeed())
		// Wait for it to show up in the API server
		Eventually(func() error {
			inst := &scribev1alpha1.ReplicationDestination{}
			return k8sClient.Get(ctx, nameFor(rd), inst)
		}, maxWait, interval).Should(Succeed())
	})

	Context("when an external replication method is specified", func() {
		BeforeEach(func() {
			rd.Spec.External = &scribev1alpha1.ReplicationDestinationExternalSpec{}
		})
		It("the CR is not reconciled", func() {
			Consistently(func() *scribev1alpha1.ReplicationDestinationStatus {
				Expect(k8sClient.Get(ctx, nameFor(rd), rd)).To(Succeed())
				return rd.Status
			}, duration, interval).Should(BeNil())
		})
	})

	Context("when a destinationPVC is specified", func() {
		var pvc *v1.PersistentVolumeClaim
		BeforeEach(func() {
			pvc = &v1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: rd.Namespace,
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{
							"storage": resource.MustParse("10Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pvc)).To(Succeed())
			rd.Spec.Rsync = &scribev1alpha1.ReplicationDestinationRsyncSpec{
				ReplicationDestinationVolumeOptions: scribev1alpha1.ReplicationDestinationVolumeOptions{
					DestinationPVC: &pvc.Name,
				},
			}
		})
		It("is used as the target PVC", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-dest-" + rd.Name, Namespace: rd.Namespace}, job)
			}, maxWait, interval).Should(Succeed())
			volumes := job.Spec.Template.Spec.Volumes
			found := false
			for _, v := range volumes {
				if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == pvc.Name {
					found = true
				}
			}
			Expect(found).To(BeTrue())
			Expect(pvc).NotTo(beOwnedBy(rd))
		})
	})

	Context("when none of capacity, accessMode, or destinationPVC are specified", func() {
		BeforeEach(func() {
			rd.Spec.Rsync = &scribev1alpha1.ReplicationDestinationRsyncSpec{
				ReplicationDestinationVolumeOptions: scribev1alpha1.ReplicationDestinationVolumeOptions{},
			}
		})
		It("generates a reconcile error", func() {
			Eventually(func() *scribev1alpha1.ReplicationDestinationStatus {
				_ = k8sClient.Get(ctx, nameFor(rd), rd)
				return rd.Status
			}, maxWait, interval).Should(Not(BeNil()))
			var cond *status.Condition
			Eventually(func() *status.Condition {
				_ = k8sClient.Get(ctx, nameFor(rd), rd)
				cond = rd.Status.Conditions.GetCondition(scribev1alpha1.ConditionReconciled)
				return cond
			}, maxWait, interval).Should(Not(BeNil()))
			Expect(cond.Status).To(Equal(corev1.ConditionFalse))
			Expect(cond.Reason).To(Equal(scribev1alpha1.ReconciledReasonError))
		})
	})

	Context("when capacity and accessModes are specified", func() {
		capacity := resource.MustParse("2Gi")
		accessModes := []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce}
		BeforeEach(func() {
			rd.Spec.Rsync = &scribev1alpha1.ReplicationDestinationRsyncSpec{
				ReplicationDestinationVolumeOptions: scribev1alpha1.ReplicationDestinationVolumeOptions{
					Capacity:    &capacity,
					AccessModes: accessModes,
				},
			}
		})

		It("creates a ClusterIP service by default", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scribe-rsync-dest-" + rd.Name,
					Namespace: rd.Namespace,
				},
			}
			Eventually(func() error {
				return k8sClient.Get(ctx, nameFor(svc), svc)
			}, maxWait, interval).Should(Succeed())
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Eventually(func() *string {
				_ = k8sClient.Get(ctx, nameFor(rd), rd)
				if rd.Status == nil || rd.Status.Rsync == nil {
					return nil
				}
				return rd.Status.Rsync.Address
			}, maxWait, interval).Should(Not(BeNil()))
			Expect(*rd.Status.Rsync.Address).To(Equal(svc.Spec.ClusterIP))
			Expect(svc).To(beOwnedBy(rd))
			By("opening a single port for ssh")
			Expect(svc.Spec.Ports).To(HaveLen(1))
			thePort := svc.Spec.Ports[0]
			Expect(thePort.Name).To(Equal("ssh"))
			Expect(thePort.Port).To(Equal(int32(22)))
			Expect(thePort.Protocol).To(Equal(corev1.ProtocolTCP))
			Expect(thePort.TargetPort).To(Equal(intstr.FromInt(22)))
		})

		Context("when serviceType is LoadBalancer", func() {
			BeforeEach(func() {
				lb := v1.ServiceTypeLoadBalancer
				rd.Spec.Rsync.ServiceType = &lb
			})
			It("a LoadBalancer service is created", func() {
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "scribe-rsync-dest-" + rd.Name,
						Namespace: rd.Namespace,
					},
				}
				Eventually(func() error {
					return k8sClient.Get(ctx, nameFor(svc), svc)
				}, maxWait, interval).Should(Succeed())
				Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
				// test env doesn't support LB, so fake the address
				svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
					{IP: "127.0.0.1"},
				}
				Expect(k8sClient.Status().Update(ctx, svc)).To(Succeed())
				Eventually(func() *string {
					_ = k8sClient.Get(ctx, nameFor(rd), rd)
					if rd.Status == nil || rd.Status.Rsync == nil {
						return nil
					}
					return rd.Status.Rsync.Address
				}, maxWait, interval).Should(Not(BeNil()))
			})
		})

		It("creates a PVC", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-dest-" + rd.Name, Namespace: rd.Namespace}, job)
			}, maxWait, interval).Should(Succeed())
			var pvcName string
			volumes := job.Spec.Template.Spec.Volumes
			for _, v := range volumes {
				if v.PersistentVolumeClaim != nil && v.Name == dataVolumeName {
					pvcName = v.PersistentVolumeClaim.ClaimName
				}
			}
			pvc := &v1.PersistentVolumeClaim{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: rd.Namespace}, pvc)
			}, maxWait, interval).Should(Succeed())
			Expect(pvc).To(beOwnedBy(rd))
			Expect(*pvc.Spec.Resources.Requests.Storage()).To(Equal(capacity))
			Expect(pvc.Spec.AccessModes).To(Equal(accessModes))
		})

		Context("when a SC is specified", func() {
			scName := "mysc"
			BeforeEach(func() {
				rd.Spec.Rsync.ReplicationDestinationVolumeOptions.StorageClassName = &scName
			})
			It("is used in the PVC", func() {
				job := &batchv1.Job{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-dest-" + rd.Name, Namespace: rd.Namespace}, job)
				}, maxWait, interval).Should(Succeed())
				var pvcName string
				volumes := job.Spec.Template.Spec.Volumes
				for _, v := range volumes {
					if v.PersistentVolumeClaim != nil && v.Name == dataVolumeName {
						pvcName = v.PersistentVolumeClaim.ClaimName
					}
				}
				pvc := &v1.PersistentVolumeClaim{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: rd.Namespace}, pvc)
				}, maxWait, interval).Should(Succeed())
				Expect(*pvc.Spec.StorageClassName).To(Equal(scName))
			})
		})
		Context("when sync should be paused", func() {
			parallelism := int32(0)
			BeforeEach(func() {
				rd.Spec.Paused = true
			})
			It("is used to define parallelism", func() {
				job := &batchv1.Job{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-dest-" + rd.Name, Namespace: rd.Namespace}, job)
				}, maxWait, interval).Should(Succeed())
				Expect(*job.Spec.Parallelism).To(Equal(parallelism))
			})
		})

		It("Generates ssh keys automatically", func() {
			secret := &v1.Secret{}
			Eventually(func() *scribev1alpha1.ReplicationDestinationStatus {
				_ = k8sClient.Get(ctx, nameFor(rd), rd)
				return rd.Status
			}, maxWait, interval).Should(Not(BeNil()))
			Eventually(func() *scribev1alpha1.ReplicationDestinationRsyncStatus {
				_ = k8sClient.Get(ctx, nameFor(rd), rd)
				return rd.Status.Rsync
			}, maxWait, interval).Should(Not(BeNil()))
			Eventually(func() *string {
				_ = k8sClient.Get(ctx, nameFor(rd), rd)
				return rd.Status.Rsync.SSHKeys
			}, maxWait, interval).Should(Not(BeNil()))
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: *rd.Status.Rsync.SSHKeys,
				Namespace: rd.Namespace}, secret)).To(Succeed())
			Expect(secret.Data).To(HaveKey("source"))
			Expect(secret.Data).To(HaveKey("source.pub"))
			Expect(secret.Data).To(HaveKey("destination.pub"))
			Expect(secret.Data).NotTo(HaveKey("destination"))
			Expect(secret).To(beOwnedBy(rd))
		})

		//nolint:dupl
		Context("when ssh keys are provided", func() {
			var secret *v1.Secret
			BeforeEach(func() {
				secret = &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "keys",
						Namespace: rd.Namespace,
					},
					StringData: map[string]string{
						"destination":     "foo",
						"destination.pub": "bar",
						"source.pub":      "baz",
					},
				}
				Expect(k8sClient.Create(ctx, secret)).To(Succeed())
				rd.Spec.Rsync.SSHKeys = &secret.Name
			})
			It("they are used by the sync Job", func() {
				job := &batchv1.Job{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: "scribe-rsync-dest-" + rd.Name, Namespace: rd.Namespace}, job)
				}, maxWait, interval).Should(Succeed())
				volumes := job.Spec.Template.Spec.Volumes
				found := false
				for _, v := range volumes {
					if v.Secret != nil && v.Secret.SecretName == secret.Name {
						found = true
					}
				}
				Expect(found).To(BeTrue())
				Expect(secret).NotTo(beOwnedBy(rd))
			})
		})
	})

	Context("after sync is complete", func() {
		BeforeEach(func() {
			capacity := resource.MustParse("10Gi")
			rd.Spec.Rsync = &scribev1alpha1.ReplicationDestinationRsyncSpec{
				ReplicationDestinationVolumeOptions: scribev1alpha1.ReplicationDestinationVolumeOptions{
					Capacity:    &capacity,
					AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
				},
			}
		})
		JustBeforeEach(func() {
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "scribe-rsync-dest-" + rd.Name,
					Namespace: rd.Namespace,
				},
			}
			// Wait for the Rsync Job to be created
			Eventually(func() error {
				return k8sClient.Get(ctx, nameFor(job), job)
			}, maxWait, interval).Should(Succeed())
			// Mark it as succeeded
			job.Status.Succeeded = 1
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
		})
		Context("with a CopyMethod of None", func() {
			BeforeEach(func() {
				rd.Spec.Rsync.CopyMethod = scribev1alpha1.CopyMethodNone
			})
			It("the PVC should be the latestImage", func() {
				Eventually(func() *v1.TypedLocalObjectReference {
					_ = k8sClient.Get(ctx, nameFor(rd), rd)
					return rd.Status.LatestImage
				}, maxWait, interval).Should(Not(BeNil()))
				li := rd.Status.LatestImage
				Expect(li.Kind).To(Equal("PersistentVolumeClaim"))
				Expect(*li.APIGroup).To(Equal(""))
				Expect(li.Name).To(Not(Equal("")))
			})
		})
		Context("with a CopyMethod of Snapshot", func() {
			BeforeEach(func() {
				rd.Spec.Rsync.CopyMethod = scribev1alpha1.CopyMethodSnapshot
			})
			It("a snapshot should be the latestImage", func() {
				By("once snapshot is created, force it to be bound")
				snapList := &snapv1.VolumeSnapshotList{}
				Eventually(func() []snapv1.VolumeSnapshot {
					_ = k8sClient.List(ctx, snapList, client.InNamespace(rd.Namespace))
					return snapList.Items
				}, maxWait, interval).Should(Not(BeEmpty()))
				snap := snapList.Items[0]
				foo := "foo"
				snap.Status = &snapv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: &foo,
				}
				Expect(k8sClient.Status().Update(ctx, &snap)).To(Succeed())
				By("seeing the now-bound snap in the LatestImage field")
				Eventually(func() *v1.TypedLocalObjectReference {
					_ = k8sClient.Get(ctx, nameFor(rd), rd)
					return rd.Status.LatestImage
				}, maxWait, interval).Should(Not(BeNil()))
				li := rd.Status.LatestImage
				Expect(li.Kind).To(Equal("VolumeSnapshot"))
				Expect(*li.APIGroup).To(Equal(snapv1.SchemeGroupVersion.Group))
				Expect(li.Name).To(Not(Equal("")))
			})
		})
	})
})
