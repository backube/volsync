package utils_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/utils"
)

var _ = Describe("Cleanup", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	var testNamespace *corev1.Namespace

	var rdA *volsyncv1alpha1.ReplicationDestination
	var rdB *volsyncv1alpha1.ReplicationDestination

	var snapA1 *snapv1.VolumeSnapshot
	var snapA2 *snapv1.VolumeSnapshot
	var snapB1 *snapv1.VolumeSnapshot

	var pvcA1 *corev1.PersistentVolumeClaim
	var pvcA2 *corev1.PersistentVolumeClaim

	BeforeEach(func() {
		// Create namespace for test
		testNamespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "ns-cleantests-",
			},
		}
		Expect(k8sClient.Create(ctx, testNamespace)).To(Succeed())
		Expect(testNamespace.Name).NotTo(BeEmpty())

		//
		// Create some replication destinations
		//
		rdA = &volsyncv1alpha1.ReplicationDestination{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "rd-a-",
				Namespace:    testNamespace.GetName(),
			},
			Spec: volsyncv1alpha1.ReplicationDestinationSpec{
				External: &volsyncv1alpha1.ReplicationDestinationExternalSpec{},
			},
		}
		Expect(k8sClient.Create(ctx, rdA)).To(Succeed())

		rdB = &volsyncv1alpha1.ReplicationDestination{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "rd-b-",
				Namespace:    testNamespace.GetName(),
			},
			Spec: volsyncv1alpha1.ReplicationDestinationSpec{
				External: &volsyncv1alpha1.ReplicationDestinationExternalSpec{},
			},
		}
		Expect(k8sClient.Create(ctx, rdB)).To(Succeed())

		//
		// Create some volume snapshots owned by the ReplicationDestinations
		//
		snapA1 = &snapv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "snap-a-1-",
				Namespace:    testNamespace.GetName(),
			},
			Spec: snapv1.VolumeSnapshotSpec{
				Source: snapv1.VolumeSnapshotSource{
					PersistentVolumeClaimName: pointer.String("dummy"),
				},
			},
		}
		// Make this owned by rdA
		Expect(ctrl.SetControllerReference(rdA, snapA1, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, snapA1)).To(Succeed())

		snapA2 = &snapv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "snap-a-2-",
				Namespace:    testNamespace.GetName(),
			},
			Spec: snapv1.VolumeSnapshotSpec{
				Source: snapv1.VolumeSnapshotSource{
					PersistentVolumeClaimName: pointer.String("dummy"),
				},
			},
		}
		// Make this owned by rdA
		Expect(ctrl.SetControllerReference(rdA, snapA2, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, snapA2)).To(Succeed())

		//
		// Create some volume snapshots owned by the ReplicationDestinations
		//
		snapB1 = &snapv1.VolumeSnapshot{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "snap-b-1-",
				Namespace:    testNamespace.GetName(),
			},
			Spec: snapv1.VolumeSnapshotSpec{
				Source: snapv1.VolumeSnapshotSource{
					PersistentVolumeClaimName: pointer.String("dummy"),
				},
			},
		}
		// Make this owned by rdB
		Expect(ctrl.SetControllerReference(rdB, snapB1, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, snapB1)).To(Succeed())

		// Create some PVCs as well
		capacity := resource.MustParse("1Gi")
		pvcA1 = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "pvc-a-1-",
				Namespace:    testNamespace.GetName(),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: capacity,
					},
				},
			},
		}
		// Make this owned by rdA
		Expect(ctrl.SetControllerReference(rdA, pvcA1, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, pvcA1)).To(Succeed())

		pvcA2 = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "pvc-a-2-",
				Namespace:    testNamespace.GetName(),
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: capacity,
					},
				},
			},
		}
		// Make this owned by rdA
		Expect(ctrl.SetControllerReference(rdA, pvcA2, k8sClient.Scheme())).To(Succeed())
		Expect(k8sClient.Create(ctx, pvcA2)).To(Succeed())
	})

	Describe("Cleanup Objects", func() {
		Context("When some objects have the cleanup label", func() {
			cleanupTypes := []client.Object{
				&corev1.PersistentVolumeClaim{},
				&snapv1.VolumeSnapshot{},
			}

			BeforeEach(func() {
				// Mark snaps A1 and B1 for cleanup
				utils.MarkForCleanup(rdA, snapA1)
				Expect(k8sClient.Update(ctx, snapA1)).To(Succeed())

				utils.MarkForCleanup(rdB, snapB1)
				Expect(k8sClient.Update(ctx, snapB1)).To(Succeed())

				// Mark pvc A1 for cleanup
				utils.MarkForCleanup(rdA, pvcA1)
				Expect(k8sClient.Update(ctx, pvcA1)).To(Succeed())
			})

			It("Should cleanup only the objects matching the cleanup label owner", func() {
				Expect(utils.CleanupObjects(ctx, k8sClient, logger, rdA, cleanupTypes)).To(Succeed())

				remainingSnapList := &snapv1.VolumeSnapshotList{}
				Expect(k8sClient.List(ctx, remainingSnapList,
					client.InNamespace(testNamespace.GetName()))).To(Succeed())
				Expect(len(remainingSnapList.Items)).To(Equal(2))

				// snapA2 should remain, no cleanup label
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snapA2), snapA1)).To(Succeed())
				// snapB1 should remain, different owner
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snapB1), snapB1)).To(Succeed())

				// Note pvcs have finalizer automatically so will not actually delete - but should be marked for deletion
				// pvcA1 should be marked for deletion
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pvcA1), pvcA1)
				if err == nil {
					Expect(pvcA1.DeletionTimestamp.IsZero()).To(BeFalse())
				} else {
					Expect(kerrors.IsNotFound(err)).To(BeTrue())
				}
				// pvcA2 should remain, no cleanup label
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvcA2), pvcA2)).To(Succeed())
				Expect(pvcA2.DeletionTimestamp.IsZero()).To(BeTrue())
			})

			Context("When a snapshot has the do-not-delete label and cleanup label", func() {
				BeforeEach(func() {
					// Mark B1 with do-not-delete
					snapB1.Labels[utils.DoNotDeleteLabelKey] = utils.DoNotDeleteLabelValue
					Expect(k8sClient.Update(ctx, snapB1)).To(Succeed())
				})

				It("Should not cleanup the snapshot(s) marked with do-not-delete", func() {
					Expect(utils.CleanupObjects(ctx, k8sClient, logger, rdB, cleanupTypes)).To(Succeed())

					remainingSnapList := &snapv1.VolumeSnapshotList{}
					Expect(k8sClient.List(ctx, remainingSnapList,
						client.InNamespace(testNamespace.GetName()))).To(Succeed())
					Expect(len(remainingSnapList.Items)).To(Equal(3)) // Nothing should be deleted

					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snapB1), snapB1)).To(Succeed())
					validateCleanupLabelAndOwnerRefRemoved(snapB1)
				})
			})
		})
	})

	Describe("Delete with preconditions", func() {
		// Want to test preconditions here - when cleaning up snapshots we use a precondition with the
		// resourceVersion to ensure the snapshot has not been modified prior to us attempting to delete it.
		// Call CleanupSnapshotsWithLabelCheck() directly here so we can test the precondition
		Context("When cleaning up snapshots that have been modified since they were read", func() {
			var snapsForCleanup *snapv1.VolumeSnapshotList
			var listOptions []client.ListOption
			var err error

			BeforeEach(func() {
				// Mark snaps A1 and A2 for cleanup
				utils.MarkForCleanup(rdA, snapA1)
				Expect(k8sClient.Update(ctx, snapA1)).To(Succeed())

				utils.MarkForCleanup(rdA, snapA2)
				Expect(k8sClient.Update(ctx, snapA2)).To(Succeed())

				// Load our list of snapshots
				snapsForCleanup = &snapv1.VolumeSnapshotList{}
				listOptions = []client.ListOption{
					client.MatchingLabels{"volsync.backube/cleanup": string(rdA.GetUID())},
					client.InNamespace(rdA.GetNamespace()),
				}
				Expect(k8sClient.List(ctx, snapsForCleanup, listOptions...)).To(Succeed())

				// Now modify one of the snapshots before calling the cleanup func
				snapA2.Labels["test-label"] = "modified"
				Expect(k8sClient.Update(ctx, snapA2)).To(Succeed())

				err = utils.CleanupSnapshotsWithLabelCheck(ctx, k8sClient, logger, rdA, snapsForCleanup)
			})

			It("Should not delete snapshots that have been modified", func() {
				Expect(err).To(HaveOccurred()) // Should get an error, snapA2 was modified
				Expect(kerrors.IsConflict(err)).To(BeTrue())
			})

			Context("When re-running cleanup (on next reconcile)", func() {
				It("Should cleanup successfully after reloading the objects", func() {
					// Re-load the list of snaps
					Expect(k8sClient.List(ctx, snapsForCleanup, listOptions...)).To(Succeed())

					// Now the func should succeed
					err = utils.CleanupSnapshotsWithLabelCheck(ctx, k8sClient, logger, rdA, snapsForCleanup)
					Expect(err).NotTo(HaveOccurred())

					remainingSnapList := &snapv1.VolumeSnapshotList{}
					Expect(k8sClient.List(ctx, remainingSnapList,
						client.InNamespace(testNamespace.GetName()))).To(Succeed())
					Expect(len(remainingSnapList.Items)).To(Equal(1)) // only snapB1 should be left

					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snapB1), snapB1)).To(Succeed())
				})
			})
		})
	})

	Describe("Relinquish snapshots", func() {
		Context("When some snapshots have the do-not-delete label", func() {
			BeforeEach(func() {
				// Mark snapA1 for cleanup and also add do-not-delete label
				utils.MarkForCleanup(rdA, snapA1)
				snapA1.Labels[utils.DoNotDeleteLabelKey] = utils.DoNotDeleteLabelValue
				Expect(k8sClient.Update(ctx, snapA1)).To(Succeed())

				// Mark snapA2 for cleanup only - should not get "relinquished/released"
				utils.MarkForCleanup(rdA, snapA2)
				Expect(k8sClient.Update(ctx, snapA2)).To(Succeed())
			})

			It("Should remove the cleanup label and replication destination ownership of the labelled snap", func() {
				Expect(utils.RelinquishOwnedSnapshotsWithDoNotDeleteLabel(ctx, k8sClient, logger, rdA)).To(Succeed())

				// SnapA1 should have cleanup label removed and ownership removed
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snapA1), snapA1)).To(Succeed())
				validateCleanupLabelAndOwnerRefRemoved(snapA1)

				// SnapA2 should still have cleanup label and ownership
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snapA2), snapA2)).To(Succeed())
				validateCleanupLabelAndOwnerRef(snapA2, rdA)

				// Run again and there should be no change
				Expect(utils.RelinquishOwnedSnapshotsWithDoNotDeleteLabel(ctx, k8sClient, logger, rdA)).To(Succeed())

				// Snap should not have been updated since cleanup label and ownership already removed
				snapA1reload := &snapv1.VolumeSnapshot{}
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snapA1), snapA1reload)).To(Succeed())
				Expect(snapA1.ResourceVersion).To(Equal(snapA1reload.ResourceVersion))
			})
		})
	})
})

// This assumes there was only 1 owner ref at the start
func validateCleanupLabelAndOwnerRefRemoved(obj client.Object) {
	labels := obj.GetLabels()
	_, ok := labels["volsync.backube/cleanup"]
	Expect(ok).To(BeFalse())                           // cleanup label should be removed
	Expect(len(obj.GetOwnerReferences())).To(Equal(0)) // Owner ref should be removed as well
}

// This assumes there was only 1 owner ref at the start
func validateCleanupLabelAndOwnerRef(obj client.Object, owner client.Object) {
	labels := obj.GetLabels()
	cleanupVal, ok := labels["volsync.backube/cleanup"]
	Expect(ok).To(BeTrue()) // cleanup label should exist
	Expect(cleanupVal).To(Equal(string(owner.GetUID())))

	Expect(len(obj.GetOwnerReferences())).To(Equal(1)) // Owner ref should exist
	ownerRef := obj.GetOwnerReferences()[0]
	Expect(ownerRef.Kind).To(Equal("ReplicationDestination"))
	Expect(ownerRef.Name).To(Equal(owner.GetName()))
}
