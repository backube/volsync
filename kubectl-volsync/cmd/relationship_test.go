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
	"io/ioutil"
	"os"
	"path"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
)

var _ = Describe("Relationships", func() {
	var dirname string
	BeforeEach(func() {
		var err error
		dirname, err = ioutil.TempDir("", "relation")
		Expect(err).NotTo(HaveOccurred())
	})
	AfterEach(func() {
		os.RemoveAll(dirname)
	})
	It("Load()-ing fails if the relationship doesn't exist", func() {
		rel, err := loadRelationship(dirname, "noexist", "sometype")
		Expect(err).To(HaveOccurred())
		Expect(rel).To(BeNil())
	})
	When("a new relationship is created", func() {
		var rname string
		var rel *Relationship
		var rtype RelationshipType = "thetype"
		BeforeEach(func() {
			rname = utilrand.String(5)
			var err error
			rel, err = createRelationship(dirname, rname, rtype)
			Expect(err).ToNot(HaveOccurred())
			Expect(rel).ToNot(BeNil())
		})
		It("Save() creates a relationship file in the specified directory", func() {
			filepath := path.Join(dirname, rname+".yaml")
			// No file
			_, err := os.Stat(filepath)
			Expect(os.IsNotExist(err)).To(BeTrue())

			Expect(rel.Save()).To(Succeed())

			// File exists
			info, err := os.Stat(filepath)
			Expect(err).ToNot(HaveOccurred())
			Expect(info.Mode().IsRegular())
		})
		It("Fails if one already exists", func() {
			_ = rel.Save()
			r2, err := createRelationship(dirname, rname, "type")
			Expect(err).To(HaveOccurred())
			Expect(r2).To(BeNil())
		})
		It("Delete() removes the relationship file in the specified directory", func() {
			// Hasn't been saved yet
			Expect(rel.Delete()).NotTo(Succeed())

			Expect(rel.Save()).To(Succeed())

			Expect(rel.Delete()).To(Succeed())
			filepath := path.Join(dirname, rname+".yaml")
			// No file
			_, err := os.Stat(filepath)
			Expect(os.IsNotExist(err)).To(BeTrue())
		})
		It("Retains its name", func() {
			Expect(rel.Name()).To(Equal(rname))
		})
		It("retains its type", func() {
			Expect(rel.Type()).To(Equal(rtype))
		})
		It("can only be re-loaded if the type matches", func() {
			Expect(rel.Save()).To(Succeed())

			rel2, err := loadRelationship(dirname, rname, "anothertype")
			Expect(err).To(HaveOccurred())
			Expect(rel2).To(BeNil())

			rel2, err = loadRelationship(dirname, rname, rtype)
			Expect(err).ToNot(HaveOccurred())
			Expect(rel2).ToNot(BeNil())
		})
		It("preserves its data", func() {
			rel.Set("akey", 7)
			Expect(rel.Save()).To(Succeed())
			rel2, err := loadRelationship(dirname, rname, rtype)
			Expect(err).ToNot(HaveOccurred())
			Expect(rel2).ToNot(BeNil())
			Expect(rel2.GetInt("akey")).To(Equal(7))
		})
		It("preserves its id", func() {
			relID := rel.GetString("id")
			Expect(rel.Save()).To(Succeed())

			rel2, err := loadRelationship(dirname, rname, rtype)
			Expect(err).ToNot(HaveOccurred())
			Expect(rel2).ToNot(BeNil())

			id := rel2.GetString("id")
			Expect(id).To(Equal(relID))
			rel2Id, err := uuid.Parse(id)
			Expect(err).ToNot(HaveOccurred())
			Expect(rel2Id.String()).To(Equal(relID))
		})
		It("can be (de-)serialized w/ resource.Quantity", func() {
			type rdata struct {
				AnInt    int
				AString  string
				Quantity *resource.Quantity
				RdataPtr *rdata
			}

			twoGi := resource.MustParse("2Gi")
			initial := rdata{
				AnInt:    7,
				AString:  "foo",
				Quantity: nil,
				RdataPtr: &rdata{
					AString:  "bar",
					Quantity: &twoGi,
				},
			}

			// We must set the Quantity's value explicitly
			Expect(rel.SetData(initial)).To(Succeed())
			rel.Set("data.rdataptr.quantity", twoGi.String())
			Expect(rel.Save()).To(Succeed())

			rel2, err := loadRelationship(dirname, rname, rtype)
			Expect(err).ToNot(HaveOccurred())
			Expect(rel2).ToNot(BeNil())

			var loaded rdata
			Expect(rel2.GetData(&loaded)).To(Succeed())

			Expect(initial.AnInt).To(Equal(loaded.AnInt))
			Expect(initial.AString).To(Equal(loaded.AString))
			Expect(initial.Quantity).To(Equal(loaded.Quantity))
			Expect(loaded.RdataPtr).NotTo(BeNil())
			Expect(initial.RdataPtr.AnInt).To(Equal(loaded.RdataPtr.AnInt))
			Expect(initial.RdataPtr.AString).To(Equal(loaded.RdataPtr.AString))
			Expect(initial.RdataPtr.Quantity.String()).To(Equal(loaded.RdataPtr.Quantity.String()))
			Expect(initial.RdataPtr.RdataPtr).To(BeNil())
			Expect(loaded.RdataPtr.RdataPtr).To(BeNil())
		})
		It("can set the ID label on an object", func() {
			pvcNoLabels := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			}
			rel.AddIDLabel(pvcNoLabels)
			Expect(pvcNoLabels.Labels).To(HaveKeyWithValue(RelationshipLabelKey, rel.ID().String()))
			pvcLabels := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					Labels: map[string]string{
						"one":   "two",
						"three": "four",
					},
				},
			}
			rel.AddIDLabel(pvcLabels)
			Expect(pvcLabels.Labels).To(HaveKeyWithValue(RelationshipLabelKey, rel.ID().String()))
		})
	})
})
