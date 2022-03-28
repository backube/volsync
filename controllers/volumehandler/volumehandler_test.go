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
				boundTo := "foo"
				snap.Status = &snapv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: &boundTo,
				}
				Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())

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
			})
		})
	})

	Context("A VolumeHandler (from source)", func() {
		var rs *volsyncv1alpha1.ReplicationSource
		var src *corev1.PersistentVolumeClaim
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
							"storage": resource.MustParse("2Gi"),
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
			It("creates a temporary PVC from a source", func() {
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
				Expect(new.Spec.StorageClassName).To(Equal(src.Spec.StorageClassName))
				Expect(new.Spec.Resources.Requests.Storage()).To(Equal(src.Spec.Resources.Requests.Storage()))
				Expect(new.Spec.AccessModes).To(Equal(src.Spec.AccessModes))
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
			It("creates a temporary PVC from a source", func() {
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
				boundTo := "bar"
				snap.Status = &snapv1.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: &boundTo,
				}
				Expect(k8sClient.Status().Update(ctx, snap)).To(Succeed())
				Expect(snap.Spec.VolumeSnapshotClassName).To(BeNil())

				// Retry expecting success
				Eventually(func() *corev1.PersistentVolumeClaim {
					new, err = vh.EnsurePVCFromSrc(ctx, logger, src, "newpvc", true)
					if err != nil {
						return nil
					}
					return new
				}, maxWait, interval).ShouldNot(BeNil())
				Expect(new).ToNot(BeNil())
				Expect(new.Name).To(Equal("newpvc"))
				// The clone should look just like the source
				Expect(new.Spec.StorageClassName).To(Equal(src.Spec.StorageClassName))
				Expect(new.Spec.Resources.Requests.Storage()).To(Equal(src.Spec.Resources.Requests.Storage()))
				Expect(new.Spec.AccessModes).To(Equal(src.Spec.AccessModes))
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
