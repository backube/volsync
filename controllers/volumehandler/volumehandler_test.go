/*
Copyright 2021 The VolSync authors.

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

package volumehandler

import (
	"context"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/utils"
	//sc "github.com/backube/volsync/controllers"
)

var _ = Describe("Volumehandler", func() {
	var ctx = context.TODO()
	var ns *corev1.Namespace
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	BeforeEach(func() {
		// Create namespace for test
		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "vh-",
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		Expect(ns.Name).NotTo(BeEmpty())
	})
	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, ns)).To(Succeed())
	})

	Context("A VolumeHandler (from destination)", func() {
		var rd *volsyncv1alpha1.ReplicationDestination
		BeforeEach(func() {
			// Scaffold RD
			rd = &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mydest",
					Namespace: ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Rsync: &volsyncv1alpha1.ReplicationDestinationRsyncSpec{
						ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
							CopyMethod:              volsyncv1alpha1.CopyMethodSnapshot,
							Capacity:                nil,
							StorageClassName:        nil,
							AccessModes:             []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							VolumeSnapshotClassName: nil,
							DestinationPVC:          nil,
						},
					},
				},
			}
		})
		JustBeforeEach(func() {
			// ReplicationDestination should have been customized in the BeforeEach
			// at each level, so now we create it.
			Expect(k8sClient.Create(ctx, rd)).To(Succeed())
			// Wait for it to show up in the API server
			Eventually(func() error {
				inst := &volsyncv1alpha1.ReplicationDestination{}
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), inst)
			}, maxWait, interval).Should(Succeed())
		})

		When("capacity & sc are specified", func() {
			capacity := resource.MustParse("7Gi")
			customSC := "custom"
			BeforeEach(func() {
				rd.Spec.Rsync.Capacity = &capacity
				rd.Spec.Rsync.StorageClassName = &customSC
			})

			It("can be used to provision a temporary PVC", func() {
				vh, err := NewVolumeHandler(
					WithClient(k8sClient),
					WithOwner(rd),
					FromDestination(&rd.Spec.Rsync.ReplicationDestinationVolumeOptions),
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(vh).ToNot(BeNil())

				pvcName := "thepvc"
				new, err := vh.EnsureNewPVC(context.TODO(), logger, pvcName)
				Expect(err).ToNot(HaveOccurred())
				Expect(new).ToNot(BeNil())
				Expect(*new.Spec.StorageClassName).To(Equal(customSC))
				Expect(*(new.Spec.Resources.Requests.Storage())).To(Equal((capacity)))
				Expect(new.Name).To(Equal(pvcName))
			})
		})

		directCopyMethodTypes := []volsyncv1alpha1.CopyMethodType{
			volsyncv1alpha1.CopyMethodNone,
			volsyncv1alpha1.CopyMethodDirect,
		}
		for i := range directCopyMethodTypes {
			// Test both None and Direct (results should be the same)
			When("CopyMethod is "+string(directCopyMethodTypes[i]), func() {
				directCopyMethodType := directCopyMethodTypes[i]

				BeforeEach(func() {
					rd.Spec.Rsync.CopyMethod = directCopyMethodType
				})

				It("the preserved image is the PVC", func() {
					vh, err := NewVolumeHandler(
						WithClient(k8sClient),
						WithOwner(rd),
						FromDestination(&rd.Spec.Rsync.ReplicationDestinationVolumeOptions),
					)
					Expect(err).NotTo(HaveOccurred())
					Expect(vh).ToNot(BeNil())

					pvcSC := "pvcsc"
					pvc := &corev1.PersistentVolumeClaim{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "mypvc",
							Namespace: ns.Name,
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{
								corev1.ReadWriteMany,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"storage": resource.MustParse("2Gi"),
								},
							},
							StorageClassName: &pvcSC,
						},
					}
					Expect(k8sClient.Create(ctx, pvc)).To(Succeed())
					// Wait for it to show up in the API server
					Eventually(func() error {
						inst := &corev1.PersistentVolumeClaim{}
						return k8sClient.Get(ctx, types.NamespacedName{Name: "mypvc", Namespace: ns.Name}, inst)
					}, maxWait, interval).Should(Succeed())

					tlor, err := vh.EnsureImage(ctx, logger, pvc)
					Expect(err).NotTo(HaveOccurred())
					Expect(tlor.Kind).To(Equal(pvc.Kind))
					Expect(tlor.Name).To(Equal(pvc.Name))
					Expect(*tlor.APIGroup).To(Equal(corev1.SchemeGroupVersion.Group))
				})
			})
		}

		When("CopyMethod is Snapshot", func() {
			BeforeEach(func() {
				rd.Spec.Rsync.CopyMethod = volsyncv1alpha1.CopyMethodSnapshot
			})

			It("the preserved image is a snapshot of the PVC", func() {
				vh, err := NewVolumeHandler(
					WithClient(k8sClient),
					WithOwner(rd),
					FromDestination(&rd.Spec.Rsync.ReplicationDestinationVolumeOptions),
				)
				Expect(err).NotTo(HaveOccurred())
				Expect(vh).ToNot(BeNil())

				pvcSC := "pvcsc"
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "mypvc",
						Namespace: ns.Name,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteMany,
						},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								"storage": resource.MustParse("2Gi"),
							},
						},
						StorageClassName: &pvcSC,
					},
				}
				Expect(k8sClient.Create(ctx, pvc)).To(Succeed())
				// Wait for it to show up in the API server
				Eventually(func() error {
					inst := &corev1.PersistentVolumeClaim{}
					return k8sClient.Get(ctx, types.NamespacedName{Name: "mypvc", Namespace: ns.Name}, inst)
				}, maxWait, interval).Should(Succeed())

				tlor, err := vh.EnsureImage(ctx, logger, pvc)
				Expect(err).NotTo(HaveOccurred())
				// Since snapshot is not bound,
				Expect(tlor).To(BeNil())

				// Grab the snap and make it look bound
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)).To(Succeed())
				snapname := pvc.Annotations[snapshotAnnotation]
				snap := &snapv1.VolumeSnapshot{}
				Eventually(func() error {
					return k8sClient.Get(ctx, types.NamespacedName{Name: snapname, Namespace: ns.Name}, snap)
				}, maxWait, interval).Should(Succeed())

				// At this point the snapshot should have ownership set
				Expect(len(snap.GetOwnerReferences())).To(Equal(1))
				ownerRef := snap.GetOwnerReferences()[0]
				Expect(ownerRef.UID).To(Equal(rd.GetUID()))

				boundTo := "foo"
				snap.Status = &snapv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: &boundTo,
				}
				Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())

				// Also add the do-not-delete label to the snapshot to test that rd should disown
				snap.Labels = map[string]string{
					utils.DoNotDeleteLabelKey: "0", // value of label should not matter
				}
				Expect(k8sClient.Update(ctx, snap)).Should(Succeed())
				// Make sure the label update has propagated to the cache before proceeding
				Eventually(func() bool {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: snapname, Namespace: ns.Name}, snap)
					if err != nil {
						return false
					}
					_, ok := snap.Labels[utils.DoNotDeleteLabelKey]
					return ok
				}, maxWait, interval).Should(BeTrue())

				// Retry expecting success
				Eventually(func() *corev1.TypedLocalObjectReference {
					tlor, err = vh.EnsureImage(ctx, logger, pvc)
					if err != nil {
						return nil
					}
					return tlor
				}, maxWait, interval).ShouldNot(BeNil())
				Expect(tlor.Kind).To(Equal("VolumeSnapshot"))
				Expect(tlor.Name).To(Equal(snapname))
				Expect(*tlor.APIGroup).To(Equal(snapv1.SchemeGroupVersion.Group))

				// Because do-not-delete label was on the snapshot, ownership should be removed
				snapReloaded := &snapv1.VolumeSnapshot{}
				Eventually(func() int {
					err := k8sClient.Get(ctx, types.NamespacedName{Name: snapname, Namespace: ns.Name}, snapReloaded)
					if err != nil {
						return 1
					}
					return len(snapReloaded.GetOwnerReferences())
				}, maxWait, interval).Should(Equal(0)) // Owner ref should be removed
			})
		})
	})

	Context("A VolumeHandler (from source)", func() {
		var rs *volsyncv1alpha1.ReplicationSource
		var src *corev1.PersistentVolumeClaim

		// Making these 3 different values so we can test we're looking at the correct properties when
		// setting the src pvc requested storage size
		pvcRequestedSize := resource.MustParse("2Gi")
		pvcCapacity := resource.MustParse("3Gi")
		snapshotRestoreSize := resource.MustParse("4Gi")

		BeforeEach(func() {
			// Scaffold RS
			rs = &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mysrc",
					Namespace: ns.Name,
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Rsync: &volsyncv1alpha1.ReplicationSourceRsyncSpec{
						ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
							CopyMethod:              volsyncv1alpha1.CopyMethodSnapshot,
							Capacity:                nil,
							StorageClassName:        nil,
							AccessModes:             nil,
							VolumeSnapshotClassName: nil,
						},
					},
				},
			}
			// Create a source PVC to use
			srcSC := "srcsc"
			src = &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mypvc",
					Namespace: ns.Name,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteMany,
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							"storage": pvcRequestedSize,
						},
					},
					StorageClassName: &srcSC,
				},
			}
		})
		JustBeforeEach(func() {
			// ReplicationSource should have been customized in the BeforeEach
			// at each level, so now we create it.
			Expect(k8sClient.Create(ctx, rs)).To(Succeed())
			Expect(k8sClient.Create(ctx, src)).To(Succeed())
			// Wait for it to show up in the API server
			Eventually(func() error {
				inst := &volsyncv1alpha1.ReplicationSource{}
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), inst)
			}, maxWait, interval).Should(Succeed())
			Eventually(func() error {
				inst := &corev1.PersistentVolumeClaim{}
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(src), inst)
			}, maxWait, interval).Should(Succeed())
		})

		When("CopyMethod is Clone", func() {
			BeforeEach(func() {
				rs.Spec.Rsync.CopyMethod = volsyncv1alpha1.CopyMethodClone
			})

			When("When no capacity is specified in the rs spec", func() {
				var vh *VolumeHandler
				JustBeforeEach(func() {
					var err error
					vh, err = NewVolumeHandler(
						WithClient(k8sClient),
						WithOwner(rs),
						FromSource(&rs.Spec.Rsync.ReplicationSourceVolumeOptions),
					)
					Expect(err).NotTo(HaveOccurred())
					Expect(vh).ToNot(BeNil())
				})

				When("The src PVC does NOT have status.capacity set", func() {
					It("creates a temporary PVC from a source falling back to using src pvc requested size", func() {
						new, err := vh.EnsurePVCFromSrc(ctx, logger, src, "newpvc", true)
						Expect(err).ToNot(HaveOccurred())
						Expect(new).ToNot(BeNil())
						Expect(new.Name).To(Equal("newpvc"))
						// The clone should look just like the source
						Expect(new.Spec.StorageClassName).To(Equal(src.Spec.StorageClassName))
						Expect(*new.Spec.Resources.Requests.Storage()).To(Equal(pvcRequestedSize))
						Expect(new.Spec.AccessModes).To(Equal(src.Spec.AccessModes))
					})
				})

				When("The src PVC has status.capacity", func() {
					JustBeforeEach(func() {
						// Update the src pvc to set a capacity in the status - this capacity should then
						// get used to set the clone PVC requested storage size
						setPvcCapacityInStatus(ctx, src, pvcCapacity)
					})

					It("creates a temporary PVC from a source using src pvc capacity", func() {
						new, err := vh.EnsurePVCFromSrc(ctx, logger, src, "newpvc", true)
						Expect(err).ToNot(HaveOccurred())
						Expect(new).ToNot(BeNil())
						Expect(new.Name).To(Equal("newpvc"))
						// The clone should look just like the source
						Expect(new.Spec.StorageClassName).To(Equal(src.Spec.StorageClassName))
						Expect(*new.Spec.Resources.Requests.Storage()).To(Equal(pvcCapacity))
						Expect(new.Spec.AccessModes).To(Equal(src.Spec.AccessModes))
					})
				})
			})
			When("options are overridden", func() {
				newSC := "thenewsc"
				newCap := resource.MustParse("9Gi")
				newAccessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
				BeforeEach(func() {
					rs.Spec.Rsync.StorageClassName = &newSC
					rs.Spec.Rsync.Capacity = &newCap
					rs.Spec.Rsync.AccessModes = newAccessModes
				})
				It("is reflected in the cloned PVC", func() {
					vh, err := NewVolumeHandler(
						WithClient(k8sClient),
						WithOwner(rs),
						FromSource(&rs.Spec.Rsync.ReplicationSourceVolumeOptions),
					)
					Expect(err).NotTo(HaveOccurred())
					Expect(vh).ToNot(BeNil())

					new, err := vh.EnsurePVCFromSrc(ctx, logger, src, "newpvc", true)
					Expect(err).ToNot(HaveOccurred())
					Expect(new).ToNot(BeNil())
					Expect(new.Name).To(Equal("newpvc"))
					// The clone should look just like the source
					Expect(*new.Spec.StorageClassName).To(Equal(newSC))
					Expect(*new.Spec.Resources.Requests.Storage()).To(Equal(newCap))
					Expect(new.Spec.AccessModes).To(Equal(newAccessModes))

				})
			})
		})
		When("CopyMethod is Snapshot", func() {
			BeforeEach(func() {
				rs.Spec.Rsync.CopyMethod = volsyncv1alpha1.CopyMethodSnapshot
			})

			When("When no capacity is specified in the rs spec", func() {
				var vh *VolumeHandler
				var snap *snapv1.VolumeSnapshot
				const newPvcName = "newpvc"

				JustBeforeEach(func() {
					var err error
					vh, err = NewVolumeHandler(
						WithClient(k8sClient),
						WithOwner(rs),
						FromSource(&rs.Spec.Rsync.ReplicationSourceVolumeOptions),
					)
					Expect(err).NotTo(HaveOccurred())
					Expect(vh).ToNot(BeNil())

					// 1st try will not succeed since snapshot is not bound
					new, err := vh.EnsurePVCFromSrc(ctx, logger, src, newPvcName, true)
					Expect(err).ToNot(HaveOccurred())
					Expect(new).To(BeNil())

					// Grab the snap and make it look bound
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(src), src)).To(Succeed())
					snap = &snapv1.VolumeSnapshot{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{Name: newPvcName, Namespace: ns.Name}, snap)
					}, maxWait, interval).Should(Succeed())
					boundTo := "bar"
					snap.Status = &snapv1.VolumeSnapshotStatus{
						BoundVolumeSnapshotContentName: &boundTo,
					}
					Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())
					Eventually(func() bool {
						// Make sure the cache picks up the update
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(snap), snap)
						if err != nil {
							return false
						}
						return snap.Status != nil && snap.Status.BoundVolumeSnapshotContentName != nil
					}, maxWait, interval).Should(BeTrue())
					Expect(snap.Spec.VolumeSnapshotClassName).To(BeNil())
				})

				When("The snapshot is bound but readyToUse is not set", func() {
					// ReadyToUse is not set in the status, so it will be ignored by volumehandler
					It("Should ignore readyToUse and create the PVC", func() {
						new, err := vh.EnsurePVCFromSrc(ctx, logger, src, newPvcName, true)
						Expect(new).NotTo(BeNil())
						Expect(err).NotTo(HaveOccurred())
					})
				})

				When("The snapshot is bound but has readyToUse=false", func() {
					JustBeforeEach(func() {
						// Set readyToUse to false on the snapshot
						ready := false
						snap.Status.ReadyToUse = &ready
						Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())
						Eventually(func() bool {
							// Make sure the cache picks up the update
							err := k8sClient.Get(ctx, client.ObjectKeyFromObject(snap), snap)
							if err != nil {
								return false
							}
							return snap.Status != nil && snap.Status.ReadyToUse != nil && !*snap.Status.ReadyToUse
						}, maxWait, interval).Should(BeTrue())
					})

					It("Does not create a PVC when the snapshot is not ready", func() {
						// Retry EnsurePVCFromSRC (first attempt is in the BeforeEach()) expecting success
						new, err := vh.EnsurePVCFromSrc(ctx, logger, src, newPvcName, true)
						Expect(new).To(BeNil())
						Expect(err).NotTo(HaveOccurred())
					})
				})

				When("The snapshot is bound and readyToUse (status.ReadyToUse=true)", func() {
					JustBeforeEach(func() {
						// Set readyToUse to true on the snapshot
						ready := true
						snap.Status.ReadyToUse = &ready
						Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())
						Eventually(func() bool {
							// Make sure the cache picks up the update
							err := k8sClient.Get(ctx, client.ObjectKeyFromObject(snap), snap)
							if err != nil {
								return false
							}
							return snap.Status != nil && snap.Status.ReadyToUse != nil && *snap.Status.ReadyToUse
						}, maxWait, interval).Should(BeTrue())
					})

					When("Snapshot status.restoreSize is not set and no status.capacity on PVC", func() {
						It("creates a snapshot and temporary PVC from a source using the request size from the src pvc", func() {
							// Retry EnsurePVCFromSRC (first attempt is in the BeforeEach()) expecting success
							var new *corev1.PersistentVolumeClaim
							var err error
							Eventually(func() *corev1.PersistentVolumeClaim {
								new, err = vh.EnsurePVCFromSrc(ctx, logger, src, newPvcName, true)
								if err != nil {
									return nil
								}
								return new
							}, maxWait, interval).ShouldNot(BeNil())
							Expect(new).ToNot(BeNil())
							Expect(new.Name).To(Equal(newPvcName))
							// The PVC from snapshot should look just like the source
							Expect(new.Spec.StorageClassName).To(Equal(src.Spec.StorageClassName))
							Expect(*new.Spec.Resources.Requests.Storage()).To(Equal(pvcRequestedSize))
							Expect(new.Spec.AccessModes).To(Equal(src.Spec.AccessModes))
						})
					})

					When("Snapshot status.restoreSize is not set but PVC status.capacity is set", func() {
						JustBeforeEach(func() {
							// Update the src pvc to set a capacity in the status - this capacity should then
							// get used to set the PVC from snapshot requested storage size
							setPvcCapacityInStatus(ctx, src, pvcCapacity)
						})

						It("creates a snapshot and temporary PVC from a source using the capacity from the src pvc", func() {
							// Retry EnsurePVCFromSRC (first attempt is in the BeforeEach()) expecting success
							var new *corev1.PersistentVolumeClaim
							var err error
							Eventually(func() *corev1.PersistentVolumeClaim {
								new, err = vh.EnsurePVCFromSrc(ctx, logger, src, newPvcName, true)
								if err != nil {
									return nil
								}
								return new
							}, maxWait, interval).ShouldNot(BeNil())
							Expect(new).ToNot(BeNil())
							Expect(new.Name).To(Equal(newPvcName))
							// The PVC from snapshot should look just like the source,
							// using capacity from src PVC to determine the storage size, not the requested storage size
							Expect(new.Spec.StorageClassName).To(Equal(src.Spec.StorageClassName))
							Expect(*new.Spec.Resources.Requests.Storage()).To(Equal(pvcCapacity))
							Expect(new.Spec.AccessModes).To(Equal(src.Spec.AccessModes))
						})
					})

					When("Snapshot status.restoreSize set", func() {
						JustBeforeEach(func() {
							// Update the snapshot to set a restoreSize in the status - this should then
							// get used to set the PVC from snapshot requested storage size
							snap := &snapv1.VolumeSnapshot{}
							Expect(k8sClient.Get(ctx,
								types.NamespacedName{Name: newPvcName, Namespace: ns.Name}, snap)).NotTo(HaveOccurred())

							snap.Status.RestoreSize = &snapshotRestoreSize

							Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())
							Eventually(func() bool {
								// Make sure the cache has picked up the update
								err := k8sClient.Get(ctx, client.ObjectKeyFromObject(snap), snap)
								if err != nil {
									return false
								}
								return snap.Status.RestoreSize != nil && *snap.Status.RestoreSize == snapshotRestoreSize
							}, maxWait, interval).Should(BeTrue())
						})

						It("creates a snapshot and temporary PVC from a source using the capacity from the src pvc", func() {
							// Retry EnsurePVCFromSRC (first attempt is in the BeforeEach()) expecting success
							var new *corev1.PersistentVolumeClaim
							var err error
							Eventually(func() *corev1.PersistentVolumeClaim {
								new, err = vh.EnsurePVCFromSrc(ctx, logger, src, newPvcName, true)
								if err != nil {
									return nil
								}
								return new
							}, maxWait, interval).ShouldNot(BeNil())
							Expect(new).ToNot(BeNil())
							Expect(new.Name).To(Equal(newPvcName))
							// The PVC from snapshot should look just like the source,
							// using restoreSize from the snapshot to determine the storage size
							Expect(new.Spec.StorageClassName).To(Equal(src.Spec.StorageClassName))
							Expect(*new.Spec.Resources.Requests.Storage()).To(Equal(snapshotRestoreSize))
							Expect(new.Spec.AccessModes).To(Equal(src.Spec.AccessModes))
						})
					})
				})
			})
			When("options are overridden", func() {
				newSC := "thenewsc"
				newCap := resource.MustParse("7Gi")
				newAccessModes := []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
				newVSC := "newvsc"
				BeforeEach(func() {
					rs.Spec.Rsync.StorageClassName = &newSC
					rs.Spec.Rsync.Capacity = &newCap
					rs.Spec.Rsync.AccessModes = newAccessModes
					rs.Spec.Rsync.VolumeSnapshotClassName = &newVSC
				})
				It("is reflected in the new PVC", func() {
					vh, err := NewVolumeHandler(
						WithClient(k8sClient),
						WithOwner(rs),
						FromSource(&rs.Spec.Rsync.ReplicationSourceVolumeOptions),
					)
					Expect(err).NotTo(HaveOccurred())
					Expect(vh).ToNot(BeNil())

					// 1st try will not succeed since snapshot is not bound
					new, err := vh.EnsurePVCFromSrc(ctx, logger, src, "newpvc", true)
					Expect(err).ToNot(HaveOccurred())
					Expect(new).To(BeNil())

					// Grab the snap and make it look bound
					Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(src), src)).To(Succeed())
					snap := &snapv1.VolumeSnapshot{}
					Eventually(func() error {
						return k8sClient.Get(ctx, types.NamespacedName{Name: "newpvc", Namespace: ns.Name}, snap)
					}, maxWait, interval).Should(Succeed())
					boundTo := "foo2"
					snap.Status = &snapv1.VolumeSnapshotStatus{
						BoundVolumeSnapshotContentName: &boundTo,
					}
					Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())
					Expect(*snap.Spec.VolumeSnapshotClassName).To(Equal(newVSC))

					// Retry expecting success
					Eventually(func() *corev1.PersistentVolumeClaim {
						new, err = vh.EnsurePVCFromSrc(ctx, logger, src, "newpvc", true)
						if err != nil {
							return nil
						}
						return new
					}, maxWait, interval).ShouldNot(BeNil())
					Expect(err).ToNot(HaveOccurred())
					Expect(new).ToNot(BeNil())
					Expect(new.Name).To(Equal("newpvc"))
					// The clone should look just like the source
					Expect(*new.Spec.StorageClassName).To(Equal(newSC))
					Expect(*new.Spec.Resources.Requests.Storage()).To(Equal(newCap))
					Expect(new.Spec.AccessModes).To(Equal(newAccessModes))
				})
			})
		})
	})
})

func setPvcCapacityInStatus(ctx context.Context, pvc *corev1.PersistentVolumeClaim, pvcCapacity resource.Quantity) {
	// Update the pvc to set a capacity in the status
	pvc.Status.Capacity = corev1.ResourceList{
		"storage": pvcCapacity,
	}
	Expect(k8sClient.Status().Update(ctx, pvc)).To(Succeed())
	Eventually(func() bool {
		// Make sure the cache has picked up the update
		err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)
		if err != nil {
			return false
		}
		return pvc.Status.Capacity != nil && pvc.Status.Capacity["storage"] == pvcCapacity
	}, maxWait, interval).Should(BeTrue())
}
