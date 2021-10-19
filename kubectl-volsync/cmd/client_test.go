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
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("XClusterName", func() {
	It("should parse 2 components into namespace and name", func() {
		xcn, err := ParseXClusterName("thens/thename")
		Expect(err).ToNot(HaveOccurred())
		Expect(xcn.Cluster).To(Equal(""))
		Expect(xcn.Namespace).To(Equal("thens"))
		Expect(xcn.Name).To(Equal("thename"))
	})
	It("should parse 3 components into cluster, namespace, name", func() {
		xcn, err := ParseXClusterName("thecl/thens/thename")
		Expect(err).ToNot(HaveOccurred())
		Expect(xcn.Cluster).To(Equal("thecl"))
		Expect(xcn.Namespace).To(Equal("thens"))
		Expect(xcn.Name).To(Equal("thename"))
	})
	It("should return an error if there are the wrong number of components", func() {
		xcn, err := ParseXClusterName("toofew")
		Expect(err).To(HaveOccurred())
		Expect(xcn).To(BeNil())

		xcn, err = ParseXClusterName("too/many/compo/nent/s")
		Expect(err).To(HaveOccurred())
		Expect(xcn).To(BeNil())
	})
	It("should parse from a relationship", func() {
		r, err := CreateRelationship("/tmp", "foo", "thetype")
		Expect(err).ToNot(HaveOccurred())
		r.Set("xc.cluster", "cn")
		r.Set("xc.namespace", "nn")
		r.Set("xc.name", "n")
		xcn, err := XClusterNameFromRelationship(r, "xc")
		Expect(err).ToNot(HaveOccurred())
		Expect(xcn).ToNot(BeNil())
		Expect(xcn.Cluster).To(Equal("cn"))
		Expect(xcn.Namespace).To(Equal("nn"))
		Expect(xcn.Name).To(Equal("n"))

		r.Set("struct", *xcn)
		xcn2, err := XClusterNameFromRelationship(r, "struct")
		Expect(err).ToNot(HaveOccurred())
		Expect(xcn2).ToNot(BeNil())
		Expect(xcn2.Name).To(Equal(xcn.Name))
	})
})
