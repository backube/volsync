/*
Copyright 2021 The Scribe authors.

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
	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Restic mover", func() {
	Context("When a retain policy is omitted", func() {
		It("has forget options that keep only the last backup", func() {
			var policy *scribev1alpha1.ResticRetainPolicy = nil
			forget := generateForgetOptions(policy)
			Expect(forget).To(MatchRegexp("^\\s*--keep-last\\s+1\\s*$"))
		})
	})
	Context("When a retain policy is empty", func() {
		It("has forget options that keep only the last backup", func() {
			policy := &scribev1alpha1.ResticRetainPolicy{}
			forget := generateForgetOptions(policy)
			Expect(forget).To(MatchRegexp("^\\s*--keep-last\\s+1\\s*$"))
		})
	})
	Context("When a retain policy is specified", func() {
		It("has forget options that correspond", func() {
			one := int32(1)
			two := int32(2)
			three := int32(3)
			four := int32(4)
			five := int32(5)
			policy := &scribev1alpha1.ResticRetainPolicy{
				Hourly:  &five,
				Daily:   &four,
				Weekly:  &three,
				Monthly: &two,
				Yearly:  &one,
			}
			forget := generateForgetOptions(policy)
			Expect(forget).NotTo(MatchRegexp("--keep-last"))
			Expect(forget).NotTo(MatchRegexp("--within"))
			Expect(forget).To(MatchRegexp("(^|\\s)--keep-hourly\\s+5(\\s|$)"))
			Expect(forget).To(MatchRegexp("(^|\\s)--keep-daily\\s+4(\\s|$)"))
			Expect(forget).To(MatchRegexp("(^|\\s)--keep-weekly\\s+3(\\s|$)"))
			Expect(forget).To(MatchRegexp("(^|\\s)--keep-monthly\\s+2(\\s|$)"))
			Expect(forget).To(MatchRegexp("(^|\\s)--keep-yearly\\s+1(\\s|$)"))
		})
		It("permits time-based retention", func() {
			duration := "5m3w1d"
			policy := &scribev1alpha1.ResticRetainPolicy{
				Within: &duration,
			}
			forget := generateForgetOptions(policy)
			Expect(forget).To(MatchRegexp("^\\s*--keep-within\\s+5m3w1d\\s*$"))
		})
	})
})
