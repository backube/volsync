/*
Copyright 2023 The VolSync authors.

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

package controllers

import (
	"context"
	"fmt"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("VolumePopulator - helper funcs", func() {
	Describe("pvcHasReplicationDestinationDataSourceRef", func() {
		var pvc *corev1.PersistentVolumeClaim
		BeforeEach(func() {
			pvc = &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vp-pvc",
					Namespace: "vp-pvc-ns",
				},
			}
		})
		Context("When a PVC has no dataSourceRef", func() {
			It("Should return false", func() {
				Expect(pvcHasReplicationDestinationDataSourceRef(pvc)).To(BeFalse())
			})
		})
		Context("When a PVC has a datasourceRef with different APIgroup", func() {
			diffAPIGroup := "somethingelse"
			It("Should return false", func() {
				pvc.Spec.DataSourceRef = &corev1.TypedObjectReference{
					APIGroup: &diffAPIGroup,
					Kind:     "ReplicationDestination",
					Name:     "myrd",
				}
				Expect(pvcHasReplicationDestinationDataSourceRef(pvc)).To(BeFalse())
			})
		})
		Context("When a PVC has a datasourceRef with Kind != ReplicationDestination", func() {
			It("Should return false", func() {
				pvc.Spec.DataSourceRef = &corev1.TypedObjectReference{
					APIGroup: &volsyncv1alpha1.GroupVersion.Group,
					Kind:     "anotherkind",
					Name:     "myrd",
				}
				Expect(pvcHasReplicationDestinationDataSourceRef(pvc)).To(BeFalse())
			})
		})
		Context("When a PVC has a datasourceRef of ReplicationDestination but no name", func() {
			It("Should return false", func() {
				pvc.Spec.DataSourceRef = &corev1.TypedObjectReference{
					APIGroup: &volsyncv1alpha1.GroupVersion.Group,
					Kind:     "ReplicationDestination",
				}
				Expect(pvcHasReplicationDestinationDataSourceRef(pvc)).To(BeFalse())
			})
		})
		Context("When a PVC has a correct datasourceRef pointing to a ReplicationDestination", func() {
			It("Should return true", func() {
				pvc.Spec.DataSourceRef = &corev1.TypedObjectReference{
					APIGroup: &volsyncv1alpha1.GroupVersion.Group,
					Kind:     "ReplicationDestination",
					Name:     "myrd",
				}
				Expect(pvcHasReplicationDestinationDataSourceRef(pvc)).To(BeTrue())
			})
		})
	})
})

var _ = Describe("VolumePopulator - Predicates", func() {
	Describe("PVCForVolPopPredicate", func() {
		pred := pvcForVolumePopulatorFilterPredicate()
		Context("When the pvc does not have a dataSourceRef", func() {
			It("should return false", func() {
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pvc-for-predtest1",
						Namespace: "p1ns",
					},
				}
				pvcNew := pvc.DeepCopy()
				pvcNew.Annotations = map[string]string{"a": "b"}

				Expect(pred.Create(event.CreateEvent{Object: pvc})).Should(BeFalse())
				Expect(pred.Delete(event.DeleteEvent{Object: pvc})).Should(BeFalse())
				Expect(pred.Update(event.UpdateEvent{ObjectOld: pvc, ObjectNew: pvcNew})).Should(BeFalse())
				Expect(pred.Generic(event.GenericEvent{Object: pvc})).Should(BeFalse())
			})
		})

		Context("When the pvc does not have a ReplicationDestination in dataSourceRef", func() {
			It("should return false", func() {
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pvc-for-predtest1",
						Namespace: "p1ns",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						DataSourceRef: &corev1.TypedObjectReference{
							APIGroup: &volsyncv1alpha1.GroupVersion.Group,
							Kind:     "AnotherKind",
							Name:     "blah",
						},
					},
				}
				pvcNew := pvc.DeepCopy()
				pvcNew.Annotations = map[string]string{"a": "b"}

				Expect(pred.Create(event.CreateEvent{Object: pvc})).Should(BeFalse())
				Expect(pred.Delete(event.DeleteEvent{Object: pvc})).Should(BeFalse())
				Expect(pred.Update(event.UpdateEvent{ObjectOld: pvc, ObjectNew: pvcNew})).Should(BeFalse())
				Expect(pred.Generic(event.GenericEvent{Object: pvc})).Should(BeFalse())
			})
		})

		Context("When the pvc has a ReplicationDestination in dataSourceRef", func() {
			It("should return true for Create/Update/Generic", func() {
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pvc-for-predtest1",
						Namespace: "p1ns",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						DataSourceRef: &corev1.TypedObjectReference{
							APIGroup: &volsyncv1alpha1.GroupVersion.Group,
							Kind:     "ReplicationDestination",
							Name:     "blah",
						},
					},
				}
				pvcNew := pvc.DeepCopy()
				pvcNew.Annotations = map[string]string{"a": "b"}

				// Want to be notified on create/update/generic
				Expect(pred.Create(event.CreateEvent{Object: pvc})).Should(BeTrue())
				Expect(pred.Update(event.UpdateEvent{ObjectOld: pvc, ObjectNew: pvcNew})).Should(BeTrue())
				Expect(pred.Generic(event.GenericEvent{Object: pvc})).Should(BeTrue())

				// Do NOT notify on delete//TODO: should we notify in case we need to cleanup pvcPrime&snapshot?
				Expect(pred.Delete(event.DeleteEvent{Object: pvc})).Should(BeFalse())
			})
		})
	})

	Describe("ReplicationDestination predicate", func() {
		pred := replicationDestinationPredicate()
		It("should return true for Create/Update/Generic", func() {
			rd := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rd1",
					Namespace: "ns1",
				},
			}
			rdNew := rd.DeepCopy()
			rdNew.Annotations = map[string]string{"a": "b"}

			// Want to be notified on create/update/generic
			Expect(pred.Create(event.CreateEvent{Object: rd})).Should(BeTrue())
			Expect(pred.Update(event.UpdateEvent{ObjectOld: rd, ObjectNew: rdNew})).Should(BeTrue())
			Expect(pred.Generic(event.GenericEvent{Object: rd})).Should(BeTrue())

			// Do NOT notify on delete
			Expect(pred.Delete(event.DeleteEvent{Object: rd})).Should(BeFalse())
		})
	})

	Describe("StorageClass predicate", func() {
		pred := storageClassPredicate()
		It("should return true for Create only", func() {
			sc := &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sc1",
					Namespace: "ns1",
				},
			}
			scNew := sc.DeepCopy()
			scNew.Annotations = map[string]string{"a": "b"}

			// Want to be notified on create only
			Expect(pred.Create(event.CreateEvent{Object: sc})).Should(BeTrue())

			// Don't need to reconcile for these
			Expect(pred.Update(event.UpdateEvent{ObjectOld: sc, ObjectNew: scNew})).Should(BeFalse())
			Expect(pred.Generic(event.GenericEvent{Object: sc})).Should(BeFalse())
			Expect(pred.Delete(event.DeleteEvent{Object: sc})).Should(BeFalse())
		})
	})
})

var _ = Describe("VolumePopulator - map functions", func() {
	var ctx = context.Background()
	var namespace *corev1.Namespace
	var pvcs []*corev1.PersistentVolumeClaim

	BeforeEach(func() {
		// Each test is run in its own namespace
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "volsync-volpop-test-",
			},
		}
		createWithCacheReload(ctx, k8sClient, namespace)
		Expect(namespace.Name).NotTo(BeEmpty())

		// PVCs for the tests - create later in JustBeforeEach so tests can modify before creating
		pvcCap := resource.MustParse("1Gi")
		pvcs = make([]*corev1.PersistentVolumeClaim, 4)
		for i := 0; i < 4; i++ {
			pvcs[i] = &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("vp-test-pvc-%d", i),
					Namespace: namespace.GetName(),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: pvcCap,
						},
					},
				},
			}
		}
	})
	JustBeforeEach(func() {
		// Create the PVCs
		for _, pvc := range pvcs {
			createWithCacheReload(ctx, k8sClient, pvc)
		}
	})

	AfterEach(func() {
		// cleanup the ns
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	Describe("mapFuncReplicationDestinationToVolumePopulatorPVC", func() {
		var replicationDestination *volsyncv1alpha1.ReplicationDestination

		BeforeEach(func() {
			// ReplicationDestination for the tests
			replicationDestination = &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "rd-for-mapfunc-test",
					Namespace: namespace.GetName(),
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{},
			}
			createWithCacheReload(ctx, k8sClient, replicationDestination)
		})

		Context("When the replicationDestination is not used by any pvc datasourceref", func() {
			It("mapFunc should not return any reconcile requests", func() {
				requests := mapFuncReplicationDestinationToVolumePopulatorPVC(k8sClient, replicationDestination)
				Expect(len(requests)).To(BeZero())
			})
		})

		Context("When the replicationDestination is used pvc datasourcerefs", func() {
			BeforeEach(func() {
				rdDataSourceRef := &corev1.TypedObjectReference{
					APIGroup: &volsyncv1alpha1.GroupVersion.Group,
					Kind:     "ReplicationDestination",
					Name:     replicationDestination.GetName(),
				}

				// pvc0, pvc2 to point to our replicationsource
				pvcs[0].Spec.DataSourceRef = rdDataSourceRef
				pvcs[2].Spec.DataSourceRef = rdDataSourceRef
			})
			It("mapFunc should return a reconcile request for each PVC that references the ReplicationDestination", func() {
				requests := mapFuncReplicationDestinationToVolumePopulatorPVC(k8sClient, replicationDestination)
				Expect(len(requests)).To(Equal(2))
				for _, req := range requests {
					Expect(req.Namespace).To(Equal(namespace.GetName()))
					Expect(req.Name == pvcs[0].GetName() || req.Name == pvcs[2].GetName()).To(BeTrue())
				}
			})

			Context("When a pvc is already bound", func() {
				BeforeEach(func() {
					// Simulate this pvc is already bound - we don't need to be notified then, vol pop work is done
					pvcs[0].Spec.VolumeName = "pvc-fakefakefake"
				})
				It("mapFunc should return a reconcile req only unbound PVCs that reference the ReplicationDestination", func() {
					requests := mapFuncReplicationDestinationToVolumePopulatorPVC(k8sClient, replicationDestination)
					Expect(len(requests)).To(Equal(1))
					Expect(requests[0].Namespace).To(Equal(namespace.GetName()))
					Expect(requests[0].Name).To(Equal(pvcs[2].GetName()))
				})
			})
		})
	})

	Describe("mapFuncStorageClassToVolumePopulatorPVC", func() {
		var storageClass *storagev1.StorageClass

		BeforeEach(func() {
			// StorageClass for the tests
			storageClass = &storagev1.StorageClass{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "storageclass-for-mapfunc-test-",
				},
				Provisioner: "my-provisioner",
			}
			createWithCacheReload(ctx, k8sClient, storageClass)
		})
		AfterEach(func() {
			// Cleanup
			Expect(k8sClient.Delete(ctx, storageClass)).To(Succeed())
		})

		Context("When the storageclass is not used by any pvc datasourceref", func() {
			It("mapFunc should not return any reconcile requests", func() {
				requests := mapFuncStorageClassToVolumePopulatorPVC(k8sClient, storageClass)
				Expect(len(requests)).To(BeZero())
			})
		})

		Context("When the storageclass is used by pvcs", func() {
			BeforeEach(func() {
				// pvc1, pvc2 use the storageclass
				pvcs[1].Spec.StorageClassName = &storageClass.Name
				pvcs[2].Spec.StorageClassName = &storageClass.Name
			})
			It("mapFunc should not return any reconcile requests", func() {
				// Should not return any since the index func filters out pvcs that don't have a ReplicationDestination
				// in the dataSourceRef
				requests := mapFuncStorageClassToVolumePopulatorPVC(k8sClient, storageClass)
				Expect(len(requests)).To(BeZero())
			})

			Context("When pvcs use the storageclass and have an RD in the dataSourceRef", func() {
				BeforeEach(func() {
					rdDataSourceRef := &corev1.TypedObjectReference{
						APIGroup: &volsyncv1alpha1.GroupVersion.Group,
						Kind:     "ReplicationDestination",
						Name:     "fake-rd-name", // Doesn't actually have to exist for this test
					}
					pvcs[1].Spec.DataSourceRef = rdDataSourceRef
					pvcs[2].Spec.DataSourceRef = rdDataSourceRef
				})

				It("mapFunc should return a reconcile req for each PVC using the storageclass", func() {
					requests := mapFuncStorageClassToVolumePopulatorPVC(k8sClient, storageClass)
					Expect(len(requests)).To(Equal(2))
					for _, req := range requests {
						Expect(req.Namespace).To(Equal(namespace.GetName()))
						Expect(req.Name == pvcs[1].GetName() || req.Name == pvcs[2].GetName()).To(BeTrue())
					}
				})

				Context("When a pvc is already bound", func() {
					BeforeEach(func() {
						// Simulate this pvc is already bound - we don't need to be notified then, vol pop work is done
						pvcs[1].Spec.VolumeName = "pvc-fakefakefake"
					})

					It("mapFunc should return a reconcile request for only unbound PVCs", func() {
						requests := mapFuncStorageClassToVolumePopulatorPVC(k8sClient, storageClass)
						Expect(len(requests)).To(Equal(1))
						Expect(requests[0].Namespace).To(Equal(namespace.GetName()))
						Expect(requests[0].Name).To(Equal(pvcs[2].GetName()))
					})
				})
			})
		})
	})
})

var _ = Describe("VolumePopulator", func() {
	var ctx = context.Background()
	var namespace *corev1.Namespace
	var rd *volsyncv1alpha1.ReplicationDestination
	var pvc *corev1.PersistentVolumeClaim

	BeforeEach(func() {
		// Each test is run in its own namespace
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "volsync-volpop-test-",
			},
		}
		createWithCacheReload(ctx, k8sClient, namespace)
		Expect(namespace.Name).NotTo(BeEmpty())

		// Scaffold the ReplicationDestination, but don't create so that it can
		// be customized per test scenario.
		rd = &volsyncv1alpha1.ReplicationDestination{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rd-for-volpop",
				Namespace: namespace.Name,
			},
		}

		// Scaffold a PVC but don't create so that it can be customized
		// per tests scenario
		pvc = &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pvc-using-volpop",
				Namespace: namespace.Name,
			},
		}
	})
	AfterEach(func() {
		// All resources are namespaced, so this should clean it all up
		Expect(k8sClient.Delete(ctx, namespace)).To(Succeed())
	})

	Context("When a PVC is created that does not have a dataSourceRef pointing to a replication destination", func() {
		JustBeforeEach(func() {
			// Create the pvc with dataSourceRef that uses a different volume populator
			//pvc.Spec.DataSourceRef
		})
		It("VolumePopulator controller should ignore the PVC", func() {
			//TODO:
		})
	})

	Context("When a PVC is created that has a dataSourceRef pointing to a replication destination", func() {
		JustBeforeEach(func() {
			createWithCacheReload(ctx, k8sClient, rd)
			// Now update the status of the rd to simulate an image
			rd.Status = &volsyncv1alpha1.ReplicationDestinationStatus{
				LatestImage: &corev1.TypedLocalObjectReference{
					APIGroup: &snapv1.SchemeGroupVersion.Group,
				},
			}

			createWithCacheReload(ctx, k8sClient, pvc)
		})
	})
})
