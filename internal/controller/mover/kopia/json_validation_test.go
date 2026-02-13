//go:build !disable_kopia

/*
Copyright 2024 The VolSync authors.

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

package kopia

import (
	"context"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("JSON Validation", func() {
	Describe("validatePolicyConfig", func() {
		var (
			ctx   context.Context
			mover *Mover
		)

		BeforeEach(func() {
			ctx = context.Background()
			mover = &Mover{
				logger: logr.Discard(),
			}
		})

		Context("when policy config is nil", func() {
			It("should return no error", func() {
				mover.policyConfig = nil
				_, err := mover.validatePolicyConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when repository config is empty", func() {
			It("should return no error", func() {
				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					RepositoryConfig: ptr.To(""),
				}
				_, err := mover.validatePolicyConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when repository config contains valid JSON", func() {
			It("should return no error", func() {
				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					RepositoryConfig: ptr.To(`{"compression": "zstd"}`),
				}
				_, err := mover.validatePolicyConfig(ctx)
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when repository config contains invalid JSON syntax", func() {
			It("should return an error containing 'invalid JSON'", func() {
				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					RepositoryConfig: ptr.To(`{invalid json}`),
				}
				_, err := mover.validatePolicyConfig(ctx)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("invalid JSON"))
			})
		})
	})
})
