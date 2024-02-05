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

package errors_test

import (
	"errors"
	"fmt"

	vsErrors "github.com/backube/volsync/controllers/errors"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("errors tests", func() {
	Describe("CopyTriggerTimeoutError", func() {
		var errSt error
		BeforeEach(func() {
			errSt = &vsErrors.CopyTriggerTimeoutError{
				SourcePVC: "pvc-a",
			}
		})

		When("An error is a CopyTriggerTimeoutError", func() {
			It("Should be comparable with errors.As()", func() {
				var copyTriggerTimeoutError *vsErrors.CopyTriggerTimeoutError
				Expect(errors.As(errSt, &copyTriggerTimeoutError)).To(BeTrue())
			})
			It("Should print out the sourcePVC", func() {
				Expect(errSt.Error()).To(ContainSubstring("pvc-a"))
				Expect(errSt.Error()).To(ContainSubstring("Timed out waiting for copy-trigger"))
			})
		})
		When("An error wraps a CopyTriggerTimeoutError", func() {
			var errWrap error
			BeforeEach(func() {
				errWrap = fmt.Errorf("Some new error, wrapping: %w", errSt)
			})
			It("Should be comparable with errors.As()", func() {
				var copyTriggerTimeoutError *vsErrors.CopyTriggerTimeoutError
				Expect(errors.As(errWrap, &copyTriggerTimeoutError)).To(BeTrue())
			})
			It("errors.As() should give us the wrapped copyTriggerTimeoutError", func() {
				var copyTriggerTimeoutError *vsErrors.CopyTriggerTimeoutError
				Expect(errors.As(errWrap, &copyTriggerTimeoutError)).To(BeTrue())
				Expect(copyTriggerTimeoutError.Error()).To(ContainSubstring("pvc-a"))
			})
		})
		When("An error is not a CopyTriggerTimeoutError", func() {
			It("errors.As should return false when comparing to CopyTriggerTimeoutError", func() {
				notStError := fmt.Errorf("This is another error")

				var copyTriggerTimeoutError *vsErrors.CopyTriggerTimeoutError
				Expect(errors.As(notStError, &copyTriggerTimeoutError)).To(BeFalse())
			})
		})
	})
})
