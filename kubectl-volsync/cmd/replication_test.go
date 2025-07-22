/*
Copyright © 2021 The VolSync authors

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/
package cmd

import (
	"context"
	"os"
	"reflect"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Replication relationships can create/save/load", func() {
	var dirname string
	var cmd *cobra.Command
	var replicationRelationshipFile string

	BeforeEach(func() {
		var err error
		// Create temp directory for relationship files
		dirname, err = os.MkdirTemp("", "relation")
		Expect(err).NotTo(HaveOccurred())

		cmd = &cobra.Command{}
		initReplicationCreateCmd(cmd) // Init createCmd as the replicationrelationship is created via the Create cmd

		cmd.Flags().StringP("relationship", "r", "test-name", "")
		cmd.Flags().String("config-dir", dirname, "")

		replicationRelationshipFile = dirname + "/test-name.yaml"
	})
	AfterEach(func() {
		os.RemoveAll(dirname)
	})

	It("can be round-triped", func() {
		By("creating a new relationship")
		rr, err := newReplicationRelationship(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(rr.data.Version).To(Equal(2))
		Expect(rr.data.Destination).To(BeNil())
		Expect(rr.data.Source).To(BeNil())

		By("saving the relationship")
		// Assign some values to test round-trip
		caps := resource.MustParse("1Gi")
		rr.data.Source = &replicationRelationshipSourceV2{
			Cluster:   "cluster",
			Namespace: "the-ns",
			PVCName:   "a-pvc",
			RSName:    "an-rs",
			ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
				CopyMethod:              volsyncv1alpha1.CopyMethodClone,
				Capacity:                &caps,
				StorageClassName:        ptr.To("scn"),
				AccessModes:             []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				VolumeSnapshotClassName: ptr.To("vscn"),
			},
		}
		capd := resource.MustParse("99Gi")
		rr.data.Destination = &replicationRelationshipDestinationV2{
			Cluster:   "c2",
			Namespace: "n2",
			RDName:    "rd2",
			ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
				CopyMethod:              volsyncv1alpha1.CopyMethodSnapshot,
				Capacity:                &capd,
				StorageClassName:        ptr.To("scn2"),
				AccessModes:             []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
				VolumeSnapshotClassName: ptr.To("vscn2"),
				DestinationPVC:          ptr.To("dpvc"),
			},
			ServiceType: (*corev1.ServiceType)(ptr.To(string(corev1.ServiceTypeClusterIP))),
		}
		Expect(rr.Save()).To(Succeed())

		// Verify ReplicationRelationship file was created
		_, err = os.Stat(replicationRelationshipFile)
		Expect(err).ToNot(HaveOccurred())

		By("loading it back in, they should match")
		rr2, err := loadReplicationRelationship(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(reflect.DeepEqual(rr2.data, rr.data)).To(BeTrue())
	})

	It("Should be able to load a relationship file at v1 and convert to v2", func() {
		const v1ReplicationRelationshipFileContents string = `data:
    destination:
        cluster: "cluster-a"
        destination:
            replicationdestinationvolumeoptions:
                accessmodes: []
                copymethod: Direct
                destinationpvc: data-dest
            servicetype: ClusterIP
        namespace: test-75543-a
        rdname: data-dest
    source:
        cluster: "cluster-b"
        namespace: test-75543-b
        pvcname: data-source
        rsname: data-source-htfcq
        source:
            address: 10.96.42.19
            replicationsourcevolumeoptions:
                accessmodes: []
                copymethod: Snapshot
            sshkeys: data-source-htfcq
        trigger:
            manual: "2024-04-20T20:58:05-04:00"
    version: 1
id: 1e7a650f-043e-4fca-b2e2-b152655e11bd
type: replication`

		// Write out the v1 replication relationship file to the expected location
		Expect(os.WriteFile(replicationRelationshipFile, []byte(v1ReplicationRelationshipFileContents), 0600)).To(Succeed())

		// Now load the file and expect it gets converted to v2 correctly
		rr, err := loadReplicationRelationship(cmd)
		Expect(err).NotTo(HaveOccurred())

		Expect(rr.data.Version).To(Equal(2))
		Expect(rr.data.IsRsyncTLS).To(BeFalse())
		Expect(rr.data.Source).NotTo(BeNil())
		Expect(rr.data.Destination).NotTo(BeNil())

		expectedDestPVC := "data-dest"
		expectedServiceType := corev1.ServiceTypeClusterIP

		dest := rr.data.Destination
		Expect(dest.Cluster).To(Equal("cluster-a"))
		Expect(dest.Namespace).To(Equal("test-75543-a"))
		Expect(dest.RDName).To(Equal("data-dest"))
		Expect(dest.ReplicationDestinationVolumeOptions).To(Equal(volsyncv1alpha1.ReplicationDestinationVolumeOptions{
			AccessModes:    []corev1.PersistentVolumeAccessMode{},
			CopyMethod:     volsyncv1alpha1.CopyMethodDirect,
			DestinationPVC: &expectedDestPVC,
		}))
		Expect(dest.ServiceType).To(Equal(&expectedServiceType))

		source := rr.data.Source
		Expect(source.Cluster).To(Equal("cluster-b"))
		Expect(source.Namespace).To(Equal("test-75543-b"))
		Expect(source.PVCName).To(Equal("data-source"))
		Expect(source.RSName).To(Equal("data-source-htfcq"))
		Expect(source.ReplicationSourceVolumeOptions).To(Equal(volsyncv1alpha1.ReplicationSourceVolumeOptions{
			AccessModes: []corev1.PersistentVolumeAccessMode{},
			CopyMethod:  volsyncv1alpha1.CopyMethodSnapshot,
		}))
		Expect(source.Trigger).To(Equal(volsyncv1alpha1.ReplicationSourceTriggerSpec{
			Manual: "2024-04-20T20:58:05-04:00",
		}))
	})

	Context("When using rsync (the default)", func() {
		var rr *replicationRelationship
		BeforeEach(func() {
			var err error
			rr, err = newReplicationRelationship(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.data.Version).To(Equal(2))
			Expect(rr.data.Destination).To(BeNil())
			Expect(rr.data.Source).To(BeNil())

			Expect(rr.data.IsRsyncTLS).To(BeFalse())
		})
		It("Should use rsync when a new replicationrelationship is created", func() {
			// Check replicationHandler is the Rsync replicationHandler
			_, ok := rr.rh.(*replicationHandlerRsync)
			Expect(ok).To(BeTrue())
		})
		It("Should use rsync when a replicationrelationship is loaded that specifies rsync", func() {
			// BeforeEach created a replicationRelationship with rsync - save it and then load it back in
			Expect(rr.Save()).To(Succeed())

			// load and then test for rsync
			rrReloaded, err := loadReplicationRelationship(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(rrReloaded.data.IsRsyncTLS).To(BeFalse())

			_, ok := rrReloaded.rh.(*replicationHandlerRsync)
			Expect(ok).To(BeTrue())
		})
	})
	Context("When using rsync-tls", func() {
		var rr *replicationRelationship
		BeforeEach(func() {
			// Make sure rsynctls flag is set
			Expect(cmd.Flags().Set("rsynctls", "true")).To(Succeed())

			var err error
			rr, err = newReplicationRelationship(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(rr.data.Version).To(Equal(2))
			Expect(rr.data.Destination).To(BeNil())
			Expect(rr.data.Source).To(BeNil())

			Expect(rr.data.IsRsyncTLS).To(BeTrue())
		})
		It("Should use rsync-tls when a new replicationrelationship is created", func() {
			// Check replicationHandler is the RsyncTLS replicationHandler
			_, ok := rr.rh.(*replicationHandlerRsyncTLS)
			Expect(ok).To(BeTrue())
		})
		It("Should use rsync-tls when a replicationrelationship is loaded that specifies rsync", func() {
			// BeforeEach created a replicationRelationship with rsync - save it and then load it back in
			Expect(rr.Save()).To(Succeed())

			// load and then test for rsync
			rrReloaded, err := loadReplicationRelationship(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(rrReloaded.data.IsRsyncTLS).To(BeTrue())

			_, ok := rrReloaded.rh.(*replicationHandlerRsyncTLS)
			Expect(ok).To(BeTrue())
		})
	})
})

var _ = Describe("Replication relationships", func() {
	var ctx context.Context
	var dirname string
	var repRel *replicationRelationship
	BeforeEach(func() {
		ctx = context.TODO()
		var err error
		// Create temp directory for relationship files
		dirname, err = os.MkdirTemp("", "relation")
		Expect(err).NotTo(HaveOccurred())
		// Create a new generic relationship
		rel, err := createRelationship(dirname, "test", ReplicationRelationshipType)
		Expect(err).NotTo(HaveOccurred())
		repRel = &replicationRelationship{
			Relationship: *rel,
			data: replicationRelationshipDataV2{
				Version:     2,
				Source:      nil,
				Destination: nil,
			},
		}
	})
	AfterEach(func() {
		os.RemoveAll(dirname)
	})

	When("the cluster is empty", func() {
		When("trying to delete", func() {
			It("succeeds if no resources are defined", func() {
				Expect(repRel.DeleteSource(ctx, k8sClient)).To(Succeed())
				Expect(repRel.DeleteDestination(ctx, k8sClient)).To(Succeed())
			})
			It("succeeds if the cluster is empty", func() {
				repRel.data.Source = &replicationRelationshipSourceV2{
					RSName:    "xxx",
					Namespace: "zzz",
				}
				repRel.data.Destination = &replicationRelationshipDestinationV2{
					RDName:    "xxx",
					Namespace: "zzz",
				}
				Expect(repRel.DeleteSource(ctx, k8sClient)).To(Succeed())
				Expect(repRel.DeleteDestination(ctx, k8sClient)).To(Succeed())
			})
		})
	})
	When("there are existing resources in the cluster", func() {
		var srcNs, dstNs *corev1.Namespace
		var rs *volsyncv1alpha1.ReplicationSource
		var rd *volsyncv1alpha1.ReplicationDestination
		BeforeEach(func() {
			// Create Namespaces
			srcNs = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "src-",
				},
			}
			Expect(k8sClient.Create(ctx, srcNs)).To(Succeed())
			Expect(srcNs.Name).NotTo(BeEmpty())
			dstNs = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "dst-",
				},
			}
			Expect(k8sClient.Create(ctx, dstNs)).To(Succeed())
			Expect(dstNs.Name).NotTo(BeEmpty())

			rs = &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "thesource",
					Namespace: srcNs.Name,
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					Rsync: &volsyncv1alpha1.ReplicationSourceRsyncSpec{},
				},
			}
			repRel.AddIDLabel(rs)
			Expect(k8sClient.Create(ctx, rs)).To(Succeed())
			rd = &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "thedestination",
					Namespace: dstNs.Name,
					Labels: map[string]string{
						"some": "label",
					},
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Rsync: &volsyncv1alpha1.ReplicationDestinationRsyncSpec{},
				},
			}
			repRel.AddIDLabel(rd)
			Expect(k8sClient.Create(ctx, rd)).To(Succeed())
			repRel.data.Source = &replicationRelationshipSourceV2{
				RSName:    rs.Name,
				Namespace: srcNs.Name,
			}
			repRel.data.Destination = &replicationRelationshipDestinationV2{
				RDName:    rd.Name,
				Namespace: dstNs.Name,
			}
		})
		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, srcNs)).To(Succeed())
			Expect(k8sClient.Delete(ctx, dstNs)).To(Succeed())
		})
		It("cleans them up when trying to delete", func() {
			rd2 := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "thedestination2",
					Namespace: dstNs.Name,
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Rsync: &volsyncv1alpha1.ReplicationDestinationRsyncSpec{},
				},
			}
			// Note: we didn't add the relationship label, therefore it
			// won't get deleted
			Expect(k8sClient.Create(ctx, rd2)).To(Succeed())
			// Set the relationship such that the Replication objs should be deleted
			repRel.data.Source.RSName = ""
			repRel.data.Destination.RDName = ""
			Expect(repRel.DeleteSource(ctx, k8sClient)).To(Succeed())
			Expect(repRel.DeleteDestination(ctx, k8sClient)).To(Succeed())

			newRs := volsyncv1alpha1.ReplicationSource{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rs), &newRs)).NotTo(Succeed())
			newRd := volsyncv1alpha1.ReplicationDestination{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rd), &newRd)).NotTo(Succeed())
			// extra one should still be there since it doesn't have the relationship ID label
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(rd2), &newRd)).To(Succeed())
		})
	})
	Context("ensureDestinationPVC makes sure there is a destination PVC", func() {
		var destNS *corev1.Namespace
		var srcPVC *corev1.PersistentVolumeClaim
		BeforeEach(func() {
			destNS = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{GenerateName: "test-"},
			}
			Expect(k8sClient.Create(ctx, destNS)).To(Succeed())
			srcCap := resource.MustParse("7Gi")
			srcPVC = &corev1.PersistentVolumeClaim{
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: srcCap,
						},
					},
				},
			}
		})
		When("destination size and accessMode aren't specified", func() {
			BeforeEach(func() {
				repRel.data.Destination = &replicationRelationshipDestinationV2{
					Cluster:   "",
					Namespace: destNS.Name,
					RDName:    "test",
				}
			})
			It("uses the values from the Source PVC", func() {
				dstPVC, err := repRel.ensureDestinationPVC(ctx, k8sClient, srcPVC)
				Expect(err).NotTo(HaveOccurred())
				Expect(dstPVC).NotTo(BeNil())
				Expect(dstPVC.Spec.AccessModes).To(ConsistOf(srcPVC.Spec.AccessModes))
				Expect(*dstPVC.Spec.Resources.Requests.Storage()).To(Equal(*srcPVC.Spec.Resources.Requests.Storage()))
			})
		})
		When("destination size and accessMode are provided", func() {
			var newCap resource.Quantity
			BeforeEach(func() {
				newCap = resource.MustParse("99Gi")
				repRel.data.Destination = &replicationRelationshipDestinationV2{
					Cluster:   "",
					Namespace: destNS.Name,
					RDName:    "test",
					ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
						Capacity:    &newCap,
					},
				}
			})
			It("uses the provided values", func() {
				dstPVC, err := repRel.ensureDestinationPVC(ctx, k8sClient, srcPVC)
				Expect(err).NotTo(HaveOccurred())
				Expect(dstPVC).NotTo(BeNil())
				Expect(dstPVC.Spec.AccessModes).To(ConsistOf([]corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}))
				Expect(*dstPVC.Spec.Resources.Requests.Storage()).To(Equal(resource.MustParse("99Gi")))
			})
		})
		When("the destination PVC exists", func() {
			var existingPVC *corev1.PersistentVolumeClaim
			BeforeEach(func() {
				existingPVC = &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: destNS.Name,
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("45Gi"),
							},
						},
						StorageClassName: ptr.To("thesc"),
					},
				}
				Expect(k8sClient.Create(ctx, existingPVC)).To(Succeed())
				repRel.data.Destination = &replicationRelationshipDestinationV2{
					Cluster:   "",
					Namespace: destNS.Name,
					RDName:    "test",
					//Destination: volsyncv1alpha1.ReplicationDestinationRsyncSpec{},
				}
			})
			It("will not be modified", func() {
				dstPVC, err := repRel.ensureDestinationPVC(ctx, k8sClient, srcPVC)
				Expect(err).NotTo(HaveOccurred())
				Expect(dstPVC).NotTo(BeNil())
				Expect(dstPVC.Spec.AccessModes).To(ConsistOf(existingPVC.Spec.AccessModes))
				Expect(*dstPVC.Spec.Resources.Requests.Storage()).To(Equal(*existingPVC.Spec.Resources.Requests.Storage()))
			})
		})
	})
})
