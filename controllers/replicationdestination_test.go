//nolint:lll
package controllers

import (
	"context"
	"strings"
	"time"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("ReplicationDestination", func() {
	var ctx = context.Background()
	var namespace *corev1.Namespace
	var rd *volsyncv1alpha1.ReplicationDestination

	BeforeEach(func() {
		// Each test is run in its own namespace
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "volsync-test-",
			},
		}
		createWithCacheReload(ctx, k8sClient, namespace)
		Expect(namespace.Name).NotTo(BeEmpty())

		// Scaffold the ReplicationDestination, but don't create so that it can
		// be customized per test scenario.
		rd = &volsyncv1alpha1.ReplicationDestination{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "instance",
				Namespace: namespace.Name,
			},
		}
	})
	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})
	JustBeforeEach(func() {
		// ReplicationDestination should have been customized in the BeforeEach
		// at each level, so now we create it.
		createWithCacheReload(ctx, k8sClient, rd)
	})

	Context("when an external replication method is specified", func() {
		BeforeEach(func() {
			rd.Spec.External = &volsyncv1alpha1.ReplicationDestinationExternalSpec{}
		})
		It("the CR is not reconciled", func() {
			Consistently(func() *volsyncv1alpha1.ReplicationDestinationStatus {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)).To(Succeed())
				return rd.Status
			}, duration, interval).Should(BeNil())
		})
	})

	Context("when no replication method is specified", func() {
		It("the CR is reports an error in the status", func() {
			Eventually(func() *volsyncv1alpha1.ReplicationDestinationStatus {
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)).To(Succeed())
				return rd.Status
			}, duration, interval).ShouldNot(BeNil())
			Expect(len(rd.Status.Conditions)).To(Equal(1))
			errCond := rd.Status.Conditions[0]
			Expect(errCond.Type).To(Equal(volsyncv1alpha1.ConditionSynchronizing))
			Expect(errCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(errCond.Reason).To(Equal(volsyncv1alpha1.SynchronizingReasonError))
			Expect(errCond.Message).To(ContainSubstring("a replication method must be specified"))
		})
	})

	//nolint:dupl
	Context("when a destinationPVC is specified", func() {
		var pvc *corev1.PersistentVolumeClaim
		BeforeEach(func() {
			pvc = &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: rd.Namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"storage": resource.MustParse("10Gi"),
						},
					},
				},
			}
			createWithCacheReload(ctx, k8sClient, pvc)
			rd.Spec.Rsync = &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
				ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
					DestinationPVC: &pvc.Name,
				},
			}
		})
		It("is used as the target PVC", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-dst-" + rd.Name, Namespace: rd.Namespace}, job)
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
			rd.Spec.Rsync = &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
				ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{},
			}
		})
		It("generates a reconcile error", func() {
			Eventually(func() *volsyncv1alpha1.ReplicationDestinationStatus {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
				return rd.Status
			}, maxWait, interval).Should(Not(BeNil()))
			var cond *metav1.Condition
			Eventually(func() *metav1.Condition {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
				cond = apimeta.FindStatusCondition(rd.Status.Conditions, volsyncv1alpha1.ConditionSynchronizing)
				if cond == nil {
					return nil
				}
				if cond.Status != metav1.ConditionFalse || cond.Reason != volsyncv1alpha1.SynchronizingReasonError {
					return nil
				}
				return cond
			}, maxWait, interval).Should(Not(BeNil()))
		})
	})

	Context("when capacity and accessModes are specified", func() {
		capacity := resource.MustParse("2Gi")
		accessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
		BeforeEach(func() {
			rd.Spec.Rsync = &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
				ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
					Capacity:    &capacity,
					AccessModes: accessModes,
				},
			}
		})

		It("creates a ClusterIP service by default", func() {
			svc := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "volsync-rsync-dst-" + rd.Name,
					Namespace: rd.Namespace,
				},
			}
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)
			}, maxWait, interval).Should(Succeed())
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Eventually(func() *string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
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
			Expect(thePort.TargetPort).To(Equal(intstr.FromInt(8022)))
		})

		Context("when serviceType is LoadBalancer", func() {
			BeforeEach(func() {
				lb := corev1.ServiceTypeLoadBalancer
				rd.Spec.Rsync.ServiceType = &lb
			})
			It("a LoadBalancer service is created", func() {
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "volsync-rsync-dst-" + rd.Name,
						Namespace: rd.Namespace,
					},
				}
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)
				}, maxWait, interval).Should(Succeed())
				Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeLoadBalancer))
				// test env doesn't support LB, so fake the address
				svc.Status.LoadBalancer.Ingress = []corev1.LoadBalancerIngress{
					{IP: "127.0.0.1"},
				}
				Expect(k8sClient.Status().Update(ctx, svc)).To(Succeed())
				Eventually(func() *string {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
					if rd.Status == nil || rd.Status.Rsync == nil {
						return nil
					}
					return rd.Status.Rsync.Address
				}, maxWait, interval).Should(Not(BeNil()))
			})
		})

		//nolint:dupl
		It("creates a PVC", func() {
			job := &batchv1.Job{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-dst-" + rd.Name, Namespace: rd.Namespace}, job)
			}, maxWait, interval).Should(Succeed())
			var pvcName string
			volumes := job.Spec.Template.Spec.Volumes
			for _, v := range volumes {
				if v.PersistentVolumeClaim != nil && v.Name == dataVolumeName {
					pvcName = v.PersistentVolumeClaim.ClaimName
				}
			}
			pvc := &corev1.PersistentVolumeClaim{}
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
					return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-dst-" + rd.Name, Namespace: rd.Namespace}, job)
				}, maxWait, interval).Should(Succeed())
				var pvcName string
				volumes := job.Spec.Template.Spec.Volumes
				for _, v := range volumes {
					if v.PersistentVolumeClaim != nil && v.Name == dataVolumeName {
						pvcName = v.PersistentVolumeClaim.ClaimName
					}
				}
				pvc := &corev1.PersistentVolumeClaim{}
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
					return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-dst-" + rd.Name, Namespace: rd.Namespace}, job)
				}, maxWait, interval).Should(Succeed())
				Expect(*job.Spec.Parallelism).To(Equal(parallelism))
			})
		})

		It("Generates ssh keys automatically", func() {
			secret := &corev1.Secret{}
			Eventually(func() *volsyncv1alpha1.ReplicationDestinationStatus {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
				return rd.Status
			}, maxWait, interval).Should(Not(BeNil()))
			Eventually(func() *volsyncv1alpha1.ReplicationDestinationRsyncStatus {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
				return rd.Status.Rsync
			}, maxWait, interval).Should(Not(BeNil()))
			Eventually(func() *string {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
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
			var secret *corev1.Secret
			BeforeEach(func() {
				secret = &corev1.Secret{
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
				createWithCacheReload(ctx, k8sClient, secret)
				rd.Spec.Rsync.SSHKeys = &secret.Name
			})
			It("they are used by the sync Job", func() {
				job := &batchv1.Job{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: "volsync-rsync-dst-" + rd.Name, Namespace: rd.Namespace}, job)
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
		var job *batchv1.Job
		BeforeEach(func() {
			capacity := resource.MustParse("10Gi")
			rd.Spec.Rsync = &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
				ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
					Capacity:    &capacity,
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				},
			}
		})
		JustBeforeEach(func() {
			job = &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "volsync-rsync-dst-" + rd.Name,
					Namespace: rd.Namespace,
				},
			}

			// Wait for the Rsync Job to be created
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
			}, maxWait, interval).Should(Succeed())
			// Mark it as succeeded
			job.Status.Succeeded = 1
			Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())
		})
		Context("with a CopyMethod of None", func() {
			BeforeEach(func() {
				rd.Spec.Rsync.CopyMethod = volsyncv1alpha1.CopyMethodNone
			})
			It("the PVC should be the latestImage", func() {
				Eventually(func() *corev1.TypedLocalObjectReference {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
					return rd.Status.LatestImage
				}, maxWait, interval).Should(Not(BeNil()))
				li := rd.Status.LatestImage
				Expect(li.Kind).To(Equal("PersistentVolumeClaim"))
				Expect(*li.APIGroup).To(Equal(""))
				Expect(li.Name).To(Not(Equal("")))
			})
		})
		Context("with a CopyMethod of Direct", func() {
			BeforeEach(func() {
				rd.Spec.Rsync.CopyMethod = volsyncv1alpha1.CopyMethodDirect
			})
			It("the PVC should be the latestImage", func() {
				Eventually(func() *corev1.TypedLocalObjectReference {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
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
				rd.Spec.Rsync.CopyMethod = volsyncv1alpha1.CopyMethodSnapshot
				rd.Spec.Trigger = &volsyncv1alpha1.ReplicationDestinationTriggerSpec{
					Manual: "test1", // Use manual trigger to prevent 2nd sync from starting immediately
				}
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
				Eventually(func() *corev1.TypedLocalObjectReference {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
					return rd.Status.LatestImage
				}, maxWait, interval).Should(Not(BeNil()))
				li := rd.Status.LatestImage
				Expect(li.Kind).To(Equal("VolumeSnapshot"))
				Expect(*li.APIGroup).To(Equal(snapv1.SchemeGroupVersion.Group))
				Expect(li.Name).To(Not(Equal("")))
			})

			It("Ensure that previous VolumeSnapshot is deleted at the end of an iteration", func() {
				// get list of volume snapshots in the ns - this is the 1st snapshot
				snapshots := &snapv1.VolumeSnapshotList{}
				Eventually(func() []snapv1.VolumeSnapshot {
					_ = k8sClient.List(ctx, snapshots, client.InNamespace(rd.Namespace))
					return snapshots.Items
				}, maxWait, interval).Should(Not(BeEmpty()))
				Expect(len(snapshots.Items)).To(Equal(1))

				// sync should be waiting for the snapshot to be bound - check that lastSyncStartTime
				// is set
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)).To(Succeed())
				Expect(rd.Status.LastSyncStartTime).Should(Not(BeNil()))

				// make the snapshot appear to be bound
				snapshot1 := snapshots.Items[0]
				foo := "fakesnapshot"
				snapshot1.Status = &snapv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: &foo,
				}
				Expect(k8sClient.Status().Update(ctx, &snapshot1)).To(Succeed())
				// wait for an image to be set for RD
				Eventually(func() *corev1.TypedLocalObjectReference {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
					return rd.Status.LatestImage
				}, maxWait, interval).Should(Not(BeNil()))
				latestImage1 := rd.Status.LatestImage
				// ensure the name was correctly set
				Expect(latestImage1.Kind).To(Equal("VolumeSnapshot"))
				Expect(*latestImage1.APIGroup).To(Equal(snapv1.SchemeGroupVersion.Group))
				Expect(latestImage1.Name).To(Equal(snapshot1.GetName()))
				// Ensure the duration was set
				Expect(rd.Status.LastSyncDuration).Should(Not(BeNil()))
				// Ensure the lastSyncStartTime was unset
				Expect(rd.Status.LastSyncStartTime).Should(BeNil())

				// Sync completed, Job should now get cleaned up
				Eventually(func() bool {
					jobFoundErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
					return kerrors.IsNotFound(jobFoundErr)
				}, maxWait, interval).Should(BeTrue())

				Eventually(func() bool {
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
					if err != nil {
						return false
					}
					synchronizingCondition := apimeta.FindStatusCondition(rd.Status.Conditions,
						volsyncv1alpha1.ConditionSynchronizing)
					if synchronizingCondition == nil {
						return false
					}
					return (synchronizingCondition.Status == metav1.ConditionFalse &&
						synchronizingCondition.Reason == volsyncv1alpha1.SynchronizingReasonManual)
				}, maxWait, interval).Should(BeTrue())
				Expect(rd.Status.LastManualSync).To(Equal("test1"))

				// About to trigger another sync - Snapshots use a time format for naming that uses seconds
				// make sure test isn't running so fast that the next sync could use the same snapshot name
				now := time.Now().Format("20060102150405")
				snap1Name := snapshot1.GetName()
				snap1NameSplit := strings.Split(snap1Name, "-")
				snap1Time := snap1NameSplit[len(snap1NameSplit)-1]
				if snap1Time == now {
					// Sleep to make sure next snapshot will not have the same name as previous
					time.Sleep(1 * time.Second)
				}

				//
				// Now manually trigger another sync to generate another snapshot
				//
				manualTrigger := "testrightnow1"
				Eventually(func() error {
					// Put this in Eventually loop to avoid update issues (controller is also updating the rd)
					err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
					if err != nil {
						return err
					}
					// Update RD with manual trigger to force another sync
					rd.Spec.Trigger.Manual = manualTrigger
					return k8sClient.Update(ctx, rd)
				}, maxWait, interval).Should(Succeed())

				// Job should be recreated for 2nd sync, force 2nd job to succeed
				Eventually(func() error {
					return k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
				}, maxWait, interval).Should(Succeed())

				// sync should be waiting for job to complete - before forcing job to succeed,
				// check that lastSyncStartTime is set
				Eventually(func() *metav1.Time {
					if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd); err != nil {
						return nil
					}
					return rd.Status.LastSyncStartTime
				}, maxWait, interval).ShouldNot(BeNil())

				job.Status.Succeeded = 1
				Expect(k8sClient.Status().Update(ctx, job)).To(Succeed())

				snapshotsAfter2ndSync := &snapv1.VolumeSnapshotList{}
				Eventually(func() []snapv1.VolumeSnapshot {
					_ = k8sClient.List(ctx, snapshotsAfter2ndSync, client.InNamespace(rd.Namespace))
					return snapshotsAfter2ndSync.Items
				}, maxWait, interval).Should(HaveLen(2))
				// Find the new VS and update its BoundVolumeSnapshotContentName
				var snapshot2 snapv1.VolumeSnapshot
				for _, sn := range snapshotsAfter2ndSync.Items {
					if sn.GetName() != snapshot1.GetName() {
						snapshot2 = sn
						break
					}
				}
				Expect(snapshot2.GetName).To(Not(Equal("")))
				foo2 := "fakesnapshot2"
				snapshot2.Status = &snapv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: &foo2,
				}
				Expect(k8sClient.Status().Update(ctx, &snapshot2)).To(Succeed())

				// wait for an image to be set for RD
				Eventually(func() *corev1.TypedLocalObjectReference {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
					if rd.Status.LatestImage == nil || rd.Status.LatestImage.Name == snapshot1.GetName() {
						// Return nil if the status is still reporting the previous image
						return nil
					}
					return rd.Status.LatestImage
				}, maxWait, interval).Should(Not(BeNil()))
				latestImage2 := rd.Status.LatestImage
				// ensure the name was correctly set
				Expect(latestImage2.Kind).To(Equal("VolumeSnapshot"))
				Expect(*latestImage2.APIGroup).To(Equal(snapv1.SchemeGroupVersion.Group))
				Expect(latestImage2.Name).To(Equal(snapshot2.GetName()))
				Expect(rd.Status.LastManualSync).To(Equal(manualTrigger))

				// Sync 2 completed, job and previous snapshot should be cleaned up
				Eventually(func() bool {
					jobFoundErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(job), job)
					return kerrors.IsNotFound(jobFoundErr)
				}, maxWait, interval).Should(BeTrue())

				Eventually(func() bool {
					snapFoundErr := k8sClient.Get(ctx, client.ObjectKeyFromObject(&snapshot1), &snapshot1)
					return kerrors.IsNotFound(snapFoundErr)
				}, maxWait, interval).Should(BeTrue())

				// Confirm the latest snapshot is not cleaned up
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(&snapshot2), &snapshot2)).To(Succeed())

				// Re-load RD to ensure we have the latest status after sync cycle completion
				// and then check status conditions
				Eventually(func() *metav1.Condition {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
					reconcileCondition := apimeta.FindStatusCondition(rd.Status.Conditions, volsyncv1alpha1.ConditionSynchronizing)
					if reconcileCondition == nil {
						return nil
					}
					if reconcileCondition.Status != metav1.ConditionFalse {
						return nil
					}
					if reconcileCondition.Reason != volsyncv1alpha1.SynchronizingReasonManual {
						return nil
					}
					return reconcileCondition
				}, maxWait, interval).Should(Not(BeNil()))

				// Ensure the duration was set
				Expect(rd.Status.LastSyncDuration).Should(Not(BeNil()))
				// Ensure the lastSyncStartTime was unset
				Expect(rd.Status.LastSyncStartTime).Should(BeNil())

				Eventually(func() *metav1.Condition {
					_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)
					syncCondition := apimeta.FindStatusCondition(rd.Status.Conditions, volsyncv1alpha1.ConditionSynchronizing)
					if syncCondition == nil {
						return nil
					}
					if syncCondition.Status != metav1.ConditionFalse {
						return nil
					}
					if syncCondition.Reason != volsyncv1alpha1.SynchronizingReasonManual {
						return nil
					}
					return syncCondition
				}, maxWait, interval).Should(Not(BeNil()))
			})
		})
	})
})
