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
	"os"
	"time"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	volumepopulatorv1beta1 "github.com/kubernetes-csi/volume-data-source-validator/client/apis/volumepopulator/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers/utils"
)

const (
	duration4s  = 4 * time.Second
	duration10s = 10 * time.Second
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

				// Do NOT notify on delete
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
	var namespace *corev1.Namespace
	var pvcs []*corev1.PersistentVolumeClaim

	pvcCap := resource.MustParse("1Gi")

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
		pvcs = make([]*corev1.PersistentVolumeClaim, 4)
		for i := 0; i < 4; i++ {
			pvcs[i] = &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("vp-test-pvc-%d", i),
					Namespace: namespace.GetName(),
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
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
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					// RD using ExternalSpec so will not be reconciled by the rd controller
					External: &volsyncv1alpha1.ReplicationDestinationExternalSpec{},
				},
			}
			createWithCacheReload(ctx, k8sClient, replicationDestination)
		})

		Context("When the replicationDestination is not used by any pvc datasourceref", func() {
			It("mapFunc should not return any reconcile requests", func() {
				requests := mapFuncReplicationDestinationToVolumePopulatorPVC(ctx, k8sClient, replicationDestination)
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
				requests := mapFuncReplicationDestinationToVolumePopulatorPVC(ctx, k8sClient, replicationDestination)
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
					requests := mapFuncReplicationDestinationToVolumePopulatorPVC(ctx, k8sClient, replicationDestination)
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
			storageClass = createTestStorageClassWithCacheReload(ctx, "sc-for-mapfunc-test-", true)
		})
		AfterEach(func() {
			// Cleanup
			deleteWithCacheReload(ctx, k8sClient, storageClass)
		})

		Context("When the storageclass is not used by any pvc datasourceref", func() {
			It("mapFunc should not return any reconcile requests", func() {
				requests := mapFuncStorageClassToVolumePopulatorPVC(ctx, k8sClient, storageClass)
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
				requests := mapFuncStorageClassToVolumePopulatorPVC(ctx, k8sClient, storageClass)
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
					requests := mapFuncStorageClassToVolumePopulatorPVC(ctx, k8sClient, storageClass)
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
						requests := mapFuncStorageClassToVolumePopulatorPVC(ctx, k8sClient, storageClass)
						Expect(len(requests)).To(Equal(1))
						Expect(requests[0].Namespace).To(Equal(namespace.GetName()))
						Expect(requests[0].Name).To(Equal(pvcs[2].GetName()))
					})
				})
			})
		})
	})
})

var _ = Describe("VolumePopulator - VolumePopulator CRD detection & ensuring CR functions", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	Context("When the VolumePopulator CRD is not present", func() {
		It("should not be detected", func() {
			isPresent, err := isVolumePopulatorCRDPresent(ctx, k8sClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(isPresent).To(BeFalse())
		})
	})

	Context("When the VolumePopulator CRD is present", func() {
		// Use direct client for these tests - so we can delete CRD and not worry about the cache
		// Using cached client means after deleting the CRD we can still do a list on the deleted resource without err
		var volumePopulatorCRD *apiextensionsv1.CustomResourceDefinition
		BeforeEach(func() {
			//nolint:lll
			// https://raw.githubusercontent.com/kubernetes-csi/volume-data-source-validator/v1.3.0/client/config/crd/populator.storage.k8s.io_volumepopulators.yaml
			bytes, err := os.ReadFile("test/populator.storage.k8s.io_volumepopulators.yaml")
			// Make sure we successfully read the file
			Expect(err).NotTo(HaveOccurred())
			Expect(len(bytes)).To(BeNumerically(">", 0))
			volumePopulatorCRD = &apiextensionsv1.CustomResourceDefinition{}
			err = yaml.Unmarshal(bytes, volumePopulatorCRD)
			Expect(err).NotTo(HaveOccurred())
			// Parsed yaml correctly
			Expect(volumePopulatorCRD.Name).To(Equal("volumepopulators.populator.storage.k8s.io"))
			Expect(k8sDirectClient.Create(ctx, volumePopulatorCRD)).NotTo(HaveOccurred())
			Eventually(func() bool {
				// Getting sccs list should return empty list (no error)
				vpList := volumepopulatorv1beta1.VolumePopulatorList{}
				err := k8sDirectClient.List(ctx, &vpList)
				if err != nil {
					return false
				}
				return len(vpList.Items) == 0
			}, 5*time.Second).Should(BeTrue())
		})
		AfterEach(func() {
			Expect(k8sDirectClient.Delete(ctx, volumePopulatorCRD)).To(Succeed())
			Eventually(func() bool {
				// CRD can take a while to cleanup and leak into subsequent tests that run in the same process, wait
				// to ensure it's gone
				reloadErr := k8sDirectClient.Get(ctx, client.ObjectKeyFromObject(volumePopulatorCRD), volumePopulatorCRD)
				if !kerrors.IsNotFound(reloadErr) {
					return false // SCC CRD is still there, keep trying
				}

				// Doublecheck sccs gone
				vpList := volumepopulatorv1beta1.VolumePopulatorList{}
				getVPListErr := k8sDirectClient.List(ctx, &vpList)
				return kerrors.IsNotFound(getVPListErr)
			}, 60*time.Second, 250*time.Millisecond).Should(BeTrue())
		})

		It("should be detected", func() {
			isPresent, err := isVolumePopulatorCRDPresent(ctx, k8sDirectClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(isPresent).To(BeTrue())
		})

		Context("When the Volsync VolumePopulator CR does not exist", func() {
			var vpCR *volumepopulatorv1beta1.VolumePopulator

			BeforeEach(func() {
				// Run func to ensure the CR gets created
				Expect(EnsureVolSyncVolumePopulatorCRIfCRDPresent(ctx, k8sDirectClient, logger)).To(Succeed())

				Eventually(func() error {
					vpCR = &volumepopulatorv1beta1.VolumePopulator{
						ObjectMeta: metav1.ObjectMeta{
							Name: VolPopCRName,
						},
					}
					return k8sDirectClient.Get(ctx, client.ObjectKeyFromObject(vpCR), vpCR)
				}, maxWait, interval).Should(Succeed())
			})

			It("EnsureVolSyncVolumePopulatorCR should create it", func() {
				Expect(vpCR.GetName()).To(Equal(VolPopCRName))
				Expect(vpCR.SourceKind.Group).To(Equal("volsync.backube"))
				Expect(vpCR.SourceKind.Kind).To(Equal("ReplicationDestination"))
			})

			Context("When the Volsync VolumePopulator CR already exists", func() {
				It("EnsureVolSyncVolumePopulatorCR should create it", func() {
					// Run func again, should still succeed
					Expect(EnsureVolSyncVolumePopulatorCRIfCRDPresent(ctx, k8sDirectClient, logger)).To(Succeed())
				})
			})
		})
	})
})

var _ = Describe("VolumePopulator", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))
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
			Spec: volsyncv1alpha1.ReplicationDestinationSpec{
				// RD using ExternalSpec so will not be reconciled by the rd controller
				// this test will artificially update the status to simulate updates
				External: &volsyncv1alpha1.ReplicationDestinationExternalSpec{},
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

	Context("When a PVC is created that has a dataSourceRef with replicationdestination and a storageClassName", func() {
		pvcCap := resource.MustParse("2Gi")

		BeforeEach(func() {
			// Set PVC spec to use ReplicationDestination as dataSourceRef
			pvc.Spec = corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: pvcCap,
					},
				},
				DataSourceRef: &corev1.TypedObjectReference{
					APIGroup: &volsyncv1alpha1.GroupVersion.Group,
					Kind:     "ReplicationDestination",
					Name:     rd.GetName(),
				},
			}
		})

		JustBeforeEach(func() {
			createWithCacheReload(ctx, k8sClient, rd)
			createWithCacheReload(ctx, k8sClient, pvc)
		})

		Context("When a storageclassname is specified in the pvc spec", func() {
			var storageClassName string

			BeforeEach(func() {
				storageClassName = "vp-test-storageclass-" + utilrand.String(5)
				pvc.Spec.StorageClassName = &storageClassName
			})

			Context("When the storageClass does not exist", func() {
				It("Should not create pvcPrime (not start populating pvc)", func() {
					Consistently(func() *corev1.PersistentVolumeClaim {
						pvcPrime, err := GetVolumePopulatorPVCPrime(ctx, k8sClient, pvc)
						Expect(err).NotTo(HaveOccurred())
						return pvcPrime
					}, duration4s, interval).Should(BeNil())
				})
			})

			Context("When the storageClass exists and latestImage snap is set on the RD", func() {
				var pvcPrime *corev1.PersistentVolumeClaim
				var snap *snapv1.VolumeSnapshot

				BeforeEach(func() {
					// Create storageclass
					createTestStorageClassWithCacheReload(ctx, storageClassName, false)
				})
				AfterEach(func() {
					// Clean up the storageclass
					sc := &storagev1.StorageClass{
						ObjectMeta: metav1.ObjectMeta{
							Name: storageClassName,
						},
					}
					deleteWithCacheReload(ctx, k8sClient, sc)
				})

				JustBeforeEach(func() {
					// Before latestImage is set, no pvcPrime should be created
					Consistently(func() *corev1.PersistentVolumeClaim {
						pvcPrime, err := GetVolumePopulatorPVCPrime(ctx, k8sClient, pvc)
						Expect(err).NotTo(HaveOccurred())
						return pvcPrime
					}, duration4s, interval).Should(BeNil())

					// After latestImage appears on RD, pvcPrime should get created
					// Create volumesnapshot
					fakePvcForSnapName := "testing-fake-pvc1"
					snap = &snapv1.VolumeSnapshot{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "test-snap-vp-testing-",
							Namespace:    namespace.GetName(),
						},
						Spec: snapv1.VolumeSnapshotSpec{
							Source: snapv1.VolumeSnapshotSource{
								PersistentVolumeClaimName: &fakePvcForSnapName,
							},
						},
					}
					createWithCacheReload(ctx, k8sClient, snap)

					Eventually(func() error {
						// Re-load RD as rd controller may have modified it
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), rd)).To(Succeed())

						// Update status to have a latestImage pointint to snapshot
						rd.Status = &volsyncv1alpha1.ReplicationDestinationStatus{
							LatestImage: &corev1.TypedLocalObjectReference{
								APIGroup: &snapv1.SchemeGroupVersion.Group,
								Kind:     "VolumeSnapshot",
								Name:     snap.GetName(),
							},
						}
						return k8sClient.Status().Update(ctx, rd)
					}, maxWait, interval).Should(Succeed())

					Eventually(func() *corev1.PersistentVolumeClaim {
						var err error
						pvcPrime, err = GetVolumePopulatorPVCPrime(ctx, k8sClient, pvc)
						Expect(err).NotTo(HaveOccurred())
						return pvcPrime
					}, maxWait, interval).ShouldNot(BeNil())

					// PVCPrime should be owned by the pvc
					Expect(len(pvcPrime.GetOwnerReferences())).To(Equal(1))
					Expect(pvcPrime.GetOwnerReferences()[0].UID).To(Equal(pvc.GetUID()))
					Expect(pvcPrime.GetOwnerReferences()[0].Controller).NotTo(BeNil())
					Expect(*pvcPrime.GetOwnerReferences()[0].Controller).To(Equal(true))

					// PVCPrime should have the spec set correctly
					Expect(pvcPrime.Spec.AccessModes).To(Equal(pvc.Spec.AccessModes))
					Expect(pvcPrime.Spec.Resources).To(Equal(pvc.Spec.Resources))
					Expect(pvcPrime.Spec.StorageClassName).To(Equal(pvc.Spec.StorageClassName))
					Expect(pvcPrime.Spec.VolumeMode).To(Equal(pvc.Spec.VolumeMode))
					Expect(pvcPrime.Spec.DataSourceRef).NotTo(BeNil())
					Expect(pvcPrime.Spec.DataSourceRef.APIGroup).NotTo(BeNil())
					Expect(*pvcPrime.Spec.DataSourceRef.APIGroup).To(Equal(snapv1.SchemeGroupVersion.Group))
					Expect(pvcPrime.Spec.DataSourceRef.Kind).To(Equal("VolumeSnapshot"))
					Expect(pvcPrime.Spec.DataSourceRef.Name).To(Equal(snap.GetName()))

					Eventually(func() error {
						// re-load the snapshot to prevent timing issues, as the controller will need to create
						// pvcPrime before adding it as an ownerRef on the snapshot
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(snap), snap)
						if err != nil {
							return err
						}

						// Snapshot should have a label indicating it's in-use by volume populator
						v, ok := snap.GetLabels()[getSnapshotInUseLabelKey(pvc)]
						if !ok {
							return fmt.Errorf("Snapshot missing volumepopulator in-use label")
						}
						if v != pvc.GetName() {
							return fmt.Errorf("Snapshot volume populator in-use label does not match the pvc."+
								" pvc label value is: %s", v)
						}

						return nil
					}, maxWait, interval).Should(Succeed())
				})

				It("pvcPrime should be created", func() {
					// See checks in BeforeEach() for detailed checks of pvcPrime
					Expect(pvcPrime).NotTo(BeNil())
				})

				Context("When pvcPrime gets a PV bound", func() {
					var pv *corev1.PersistentVolume

					JustBeforeEach(func() {
						// Outer beforeEach will go through steps which create pvcPrime - now
						// manually update it to simulate a PV being bound

						// Create PV for the test
						volMode := corev1.PersistentVolumeFilesystem
						pv = &corev1.PersistentVolume{
							ObjectMeta: metav1.ObjectMeta{
								GenerateName: "pv-for-volpop-",
							},
							Spec: corev1.PersistentVolumeSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
								Capacity: corev1.ResourceList{
									corev1.ResourceStorage: pvcCap,
								},
								VolumeMode: &volMode,
								//StorageClassName:              "manual",
								StorageClassName:              storageClassName,
								PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
								PersistentVolumeSource: corev1.PersistentVolumeSource{
									CSI: &corev1.CSIPersistentVolumeSource{
										Driver:       "fakedriver",
										VolumeHandle: "my-vol-handle",
									},
								},
							},
						}
						createWithCacheReload(ctx, k8sClient, pv)

						// Now update pvcPrime to indicate pv is bound to it - volumepopulator controller
						// should then update the PV to bind it to pvc instead of pvcPrime
						Eventually(func() error {
							// re-load pvcPrime
							Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvcPrime), pvcPrime)).To(Succeed())

							pvcPrime.Spec.VolumeName = pv.GetName()

							return k8sClient.Update(ctx, pvcPrime)
						}, duration10s, interval).Should(Succeed())

						// Make sure the vol pop controller has updated the claimRef on the PV created for pvcPrime
						// to rebind it to the original pvc (rebindPVC step)
						Eventually(func() bool {
							// re-load PV
							Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pv), pv)).To(Succeed())

							return pv.Spec.ClaimRef != nil &&
								pv.Spec.ClaimRef.Name == pvc.GetName()
						}, duration10s, interval).Should(BeTrue())

						Expect(pv.Spec.ClaimRef.Name).To(Equal(pvc.GetName()))
						Expect(pv.Spec.ClaimRef.Namespace).To(Equal(pvc.GetNamespace()))
						Expect(pv.Spec.ClaimRef.UID).To(Equal(pvc.GetUID()))

						// re-load pvc just in case here
						Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)).To(Succeed())
						Expect(pv.Spec.ClaimRef.ResourceVersion).To(Equal(pvc.GetResourceVersion()))

						// Make sure pvcPrime isn't cleaned up yet
						Consistently(func() *corev1.PersistentVolumeClaim {
							pvcPrimeReloaded, err := GetVolumePopulatorPVCPrime(ctx, k8sClient, pvc)
							Expect(err).NotTo(HaveOccurred())
							return pvcPrimeReloaded
						}, duration4s, interval).ShouldNot(BeNil())
					})

					It("The snapshot should have correct owner reference of pvcPrime", func() {
						// This test was originally done before the pvcPrime gets bound
						// but moving here - we can have a timing issue during test where
						// the cache is not updated in time for ensureOwnerReferenceOnSnapshots()
						// to see the snapshot with our "volsync.backube/volpop-pvc-xxxx" (in-use) label
						// It also may not get reconciled again in the test env since no one is updating
						// pvcPrime - doing so here means pvcPrime is updated due to the PV being bound and
						// a reconcile should get triggered.
						Eventually(func() error {
							// re-load the snapshot to prevent timing issues, as the controller will need to create
							// pvcPrime before adding it as an ownerRef on the snapshot
							err := k8sClient.Get(ctx, client.ObjectKeyFromObject(snap), snap)
							if err != nil {
								return err
							}
							// Snapshot should get ownerRef pointing to pvcPrime
							ownerRefs := snap.GetOwnerReferences()
							if len(ownerRefs) != 1 {
								oRefErr := fmt.Errorf("Snapshot owner references incorrect - len(ownerRefs): %d", len(ownerRefs))
								logger.Error(oRefErr, "Snapshot missing owner ref", "snap", *snap)
								return oRefErr
							}
							if ownerRefs[0].UID != pvcPrime.UID {
								return fmt.Errorf("Snapshot ownerRef UID: %s does not match pvcPrime.UID: %s",
									ownerRefs[0].UID, pvcPrime.UID)
							}

							return nil
						}, maxWait, interval).Should(Succeed())
					})

					Context("When pvcPrime loses claim (bind controller rebinds the PV to pvc from pvcPrime)", func() {
						type snapCleanupTest struct {
							hasDoNotDeleteLabel           bool
							ownedByReplicationDestination bool
						}
						testSnapWithDoNotDelete := map[string]snapCleanupTest{
							"has do-not-delete label":                  {hasDoNotDeleteLabel: true, ownedByReplicationDestination: false},
							"does not have do-not-delete label":        {hasDoNotDeleteLabel: false, ownedByReplicationDestination: false},
							"is still owned by replicationdestination": {hasDoNotDeleteLabel: false, ownedByReplicationDestination: true},
						}
						for i := range testSnapWithDoNotDelete {
							Context(fmt.Sprintf("When snapshot %s", i), func() {
								snapHasDoNotDelete := testSnapWithDoNotDelete[i].hasDoNotDeleteLabel
								snapOwnedByRD := testSnapWithDoNotDelete[i].ownedByReplicationDestination

								It("Should cleanup pvcPrime (and volumesnapshot if necessary)", func() {
									var snapOwnerRefCountBefore int
									if snapHasDoNotDelete {
										// Put a do-not-delete label on the snapshot before we do the bind process
										// to test the cleanup (volume populator should not cleanup if this label is present)
										Eventually(func() error {
											Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snap), snap)).To(Succeed())
											utils.MarkDoNotDelete(snap)
											return k8sClient.Update(ctx, snap)
										}, duration10s, interval).Should(Succeed())
									}

									if snapOwnedByRD {
										Eventually(func() error {
											Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snap), snap)).To(Succeed())
											if err := ctrl.SetControllerReference(rd, snap, k8sClient.Scheme()); err != nil {
												return err
											}
											return k8sClient.Update(ctx, snap)
										}, duration10s, interval).Should(Succeed())
									}

									snapOwnerRefCountBefore = len(snap.GetOwnerReferences())
									expectedSnapOwnerRefCountBefore := 1
									if snapOwnedByRD {
										expectedSnapOwnerRefCountBefore++
									}
									Expect(snapOwnerRefCountBefore).To(Equal(expectedSnapOwnerRefCountBefore))

									// Update pvcPrime spec to simulate the status update indicating claim lost
									Eventually(func() error {
										// re-load pvcPrime
										Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvcPrime), pvcPrime)).To(Succeed())
										pvcPrime.Status.Phase = corev1.ClaimLost
										return k8sClient.Status().Update(ctx, pvcPrime)
									}, duration10s, interval).Should(Succeed())

									// Update pv to indicate it's bound to a PV
									Eventually(func() error {
										Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pvc), pvc)).To(Succeed())
										pvc.Spec.VolumeName = pv.GetName()
										return k8sClient.Status().Update(ctx, pvc)
									}, duration10s, interval).Should(Succeed())

									// Now wait for the controller to reconcile and cleanup pvcPrime
									Eventually(func() bool {
										err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pvcPrime), pvcPrime)
										// Note pvcs won't actually get deleted in envtest because of finalizer
										// return success if it's marked for deletion
										return (err != nil && kerrors.IsNotFound(err)) || !pvcPrime.GetDeletionTimestamp().IsZero()
									}, maxWait, interval).Should(BeTrue())

									// volumesnapshot should be left behind or garbage collected depending on
									// whether it is marked as do-not-delete or if owned by a replicationdestination
									Eventually(func() error {
										// Also check that the volumesnapshot was updated properly
										Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(snap), snap)).To(Succeed())

										// GC would normally happen, but envtest will leave it behind
										ownerRefs := snap.GetOwnerReferences()

										foundPvcPrimeOwnerRef := false
										for _, ref := range ownerRefs {
											if ref.UID == pvcPrime.UID {
												foundPvcPrimeOwnerRef = true
											}
										}

										if snapHasDoNotDelete {
											// Owner ref should be removed
											if len(ownerRefs) != (snapOwnerRefCountBefore - 1) {
												return fmt.Errorf("ownerRefs should have been removed. "+
													"snapOwnerRefCountBefore: %d, ownerRefs: %+v",
													snapOwnerRefCountBefore, ownerRefs)
											}
											if foundPvcPrimeOwnerRef {
												return fmt.Errorf("ownerRefs for pvcPrime still exists. ownerRefs: %+v",
													ownerRefs)
											}
										} else if !snapOwnedByRD {
											// Note in a real env, after pvcPrime is deleted, GC will remove
											// the owner ref on the snapshot that points to pvcPrime - however in
											// envtest GC doesn't run

											// Check that the owner ref is still there - this means GC will cleanup
											// in real env
											if len(ownerRefs) != snapOwnerRefCountBefore {
												return fmt.Errorf("ownerRefs expected not to have changed. "+
													"snapOwnerRefCountBefore: %d, ownerRefs: %+v",
													snapOwnerRefCountBefore, ownerRefs)
											}
											if !foundPvcPrimeOwnerRef {
												return fmt.Errorf("ownerRef for pvcPrime was removed. ownerRefs: %+v",
													ownerRefs)
											}
										}

										// Snap label should be removed in all cases
										_, ok := snap.GetLabels()[getSnapshotInUseLabelKey(pvc)]
										if ok {
											return fmt.Errorf("snapshot still has volume populator in-use label. "+
												"snapshot labels: %+v", snap.GetLabels())
										}
										return nil
									}, maxWait, interval).Should(Succeed())
								})
							})
						}
					})
				})
			})
		})
	})
})

func createTestStorageClassWithCacheReload(ctx context.Context,
	storageClassName string, generateName bool) *storagev1.StorageClass {
	scObjMeta := metav1.ObjectMeta{
		Name: storageClassName,
	}
	if generateName {
		scObjMeta = metav1.ObjectMeta{
			GenerateName: storageClassName,
		}
	}
	storageClass := &storagev1.StorageClass{
		ObjectMeta:  scObjMeta,
		Provisioner: "my-provisioner",
		Parameters:  map[string]string{"type": "testtype"},
	}
	createWithCacheReload(ctx, k8sClient, storageClass)

	return storageClass
}
