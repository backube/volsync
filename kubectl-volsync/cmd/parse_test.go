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
	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var _ = Describe("CLI parsing functions", func() {
	var fset *pflag.FlagSet
	const flagname = "flagname"
	BeforeEach(func() {
		fset = pflag.NewFlagSet("test-set", pflag.ContinueOnError)
	})
	Context("Volume accessModes can be parsed", func() {
		BeforeEach(func() {
			fset.StringSlice(flagname, nil, "usage")
		})
		It("returns an empty list if the flag is not set", func() {
			amlist, err := parseAccessModes(fset, flagname)
			Expect(err).NotTo(HaveOccurred())
			Expect(amlist).To(BeEmpty())
		})
		It("returns a list of the provided modes", func() {
			Expect(fset.Set(flagname, "ReadWriteOnce,ReadWriteMany")).To(Succeed())
			amlist, err := parseAccessModes(fset, flagname)
			Expect(err).NotTo(HaveOccurred())
			Expect(amlist).To(ConsistOf(corev1.ReadWriteOnce, corev1.ReadWriteMany))
		})
		It("returns an error if an invalid mode is specified", func() {
			Expect(fset.Set(flagname, "ReadWriteOnce,GarbageMode")).To(Succeed())
			amlist, err := parseAccessModes(fset, flagname)
			Expect(err).To(HaveOccurred())
			Expect(amlist).To(BeEmpty())
		})
	})
	Context("Capacity can be parsed", func() {
		BeforeEach(func() {
			fset.String(flagname, "", "usage")
		})
		It("returns nil if the capacity is not set", func() {
			capParsed, err := parseCapacity(fset, flagname)
			Expect(err).NotTo(HaveOccurred())
			Expect(capParsed).To(BeNil())
		})
		It("returns an error if the capacity is not parsable", func() {
			Expect(fset.Set(flagname, "Garbage")).To(Succeed())
			capParsed, err := parseCapacity(fset, flagname)
			Expect(err).To(HaveOccurred())
			Expect(capParsed).To(BeNil())
		})
		It("properly parses a valid Quantity", func() {
			expected := resource.MustParse("123Gi")
			Expect(fset.Set(flagname, expected.String())).To(Succeed())
			capParsed, err := parseCapacity(fset, flagname)
			Expect(err).NotTo(HaveOccurred())
			Expect(capParsed).NotTo(BeNil())
			Expect(*capParsed).To(Equal(expected))
		})
	})
	Context("CopyMethod can be parsed", func() {
		BeforeEach(func() {
			fset.String(flagname, "", "usage")
		})
		It("returns nil if the method is not set", func() {
			cm, err := parseCopyMethod(fset, flagname, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm).To(BeNil())
		})
		It("returns an error if the method is not a valid method", func() {
			Expect(fset.Set(flagname, "Invalid")).To(Succeed())
			cm, err := parseCopyMethod(fset, flagname, true)
			Expect(err).To(HaveOccurred())
			Expect(cm).To(BeNil())
		})
		It("only allows Clone if enabled", func() {
			Expect(fset.Set(flagname, "Clone")).To(Succeed())
			// Not allowed
			cm, err := parseCopyMethod(fset, flagname, false)
			Expect(err).To(HaveOccurred())
			Expect(cm).To(BeNil())
			// Allowed
			cm, err = parseCopyMethod(fset, flagname, true)
			Expect(err).NotTo(HaveOccurred())
			Expect(cm).NotTo(BeNil())
			Expect(*cm).To(Equal(volsyncv1alpha1.CopyMethodClone))
		})
	})
})
