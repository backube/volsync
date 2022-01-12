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
	"io/ioutil"
	"os"
	"reflect"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Replication relationships can create/save/load", func() {
	var dirname string
	var cmd *cobra.Command
	BeforeEach(func() {
		var err error
		// Create temp directory for relationship files
		dirname, err = ioutil.TempDir("", "relation")
		Expect(err).NotTo(HaveOccurred())

		cmd = &cobra.Command{}
		cmd.Flags().StringP("relationship", "r", "test-name", "")
		cmd.Flags().String("config-dir", dirname, "")
	})
	AfterEach(func() {
		os.RemoveAll(dirname)
	})
	It("can be round-triped", func() {
		By("creating a new relationship")
		rr, err := newReplicationRelationship(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(rr.data.Version).To(Equal(1))
		Expect(rr.data.Destination).To(BeNil())
		Expect(rr.data.Source).To(BeNil())

		By("saving the relationship")
		// Assign some values to test round-trip
		caps := resource.MustParse("1Gi")
		rr.data.Source = &replicationRelationshipSource{
			Cluster:   "cluster",
			Namespace: "the-ns",
			PVCName:   "a-pvc",
			RSName:    "an-rs",
			Source: volsyncv1alpha1.ReplicationSourceRsyncSpec{
				ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
					CopyMethod:              volsyncv1alpha1.CopyMethodClone,
					Capacity:                &caps,
					StorageClassName:        pointer.String("scn"),
					AccessModes:             []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					VolumeSnapshotClassName: pointer.String("vscn"),
				},
			},
		}
		capd := resource.MustParse("99Gi")
		rr.data.Destination = &replicationRelationshipDestination{
			Cluster:   "c2",
			Namespace: "n2",
			RDName:    "rd2",
			Destination: volsyncv1alpha1.ReplicationDestinationRsyncSpec{
				ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
					CopyMethod:              volsyncv1alpha1.CopyMethodSnapshot,
					Capacity:                &capd,
					StorageClassName:        pointer.String("scn2"),
					AccessModes:             []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
					VolumeSnapshotClassName: pointer.String("vscn2"),
					DestinationPVC:          pointer.String("dpvc"),
				},
				ServiceType: (*corev1.ServiceType)(pointer.String(string(corev1.ServiceTypeClusterIP))),
			},
		}
		Expect(rr.Save()).To(Succeed())

		By("loading it back in, they should match")
		rr2, err := loadReplicationRelationship(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(reflect.DeepEqual(rr2.data, rr.data)).To(BeTrue())
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
		dirname, err = ioutil.TempDir("", "relation")
		Expect(err).NotTo(HaveOccurred())
		// Create a new generic relationship
		rel, err := createRelationship(dirname, "test", ReplicationRelationshipType)
		Expect(err).NotTo(HaveOccurred())
		repRel = &replicationRelationship{
			Relationship: *rel,
			data: replicationRelationshipData{
				Version:     1,
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
				repRel.data.Source = &replicationRelationshipSource{
					RSName:    "xxx",
					Namespace: "zzz",
				}
				repRel.data.Destination = &replicationRelationshipDestination{
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
			repRel.data.Source = &replicationRelationshipSource{
				RSName:    rs.Name,
				Namespace: srcNs.Name,
			}
			repRel.data.Destination = &replicationRelationshipDestination{
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
})
