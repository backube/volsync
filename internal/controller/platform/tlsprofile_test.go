/*
Copyright 2026 The VolSync authors.

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

package platform

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ocpconfigv1 "github.com/openshift/api/config/v1"
)

var _ = Describe("Test tlsprofile helper funcs", func() {
	Describe("ParseTLSVersionForStunnelPSK - should use TLS 1.3", func() {
		It("Should convert the golang TLS version string into one sTunnel/openssl can use", func() {
			tlsVersion, err := ParseTLSVersionForStunnelPSK(ocpconfigv1.VersionTLS10)
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsVersion).To(Equal("TLSv1.3"))

			tlsVersion, err = ParseTLSVersionForStunnelPSK(ocpconfigv1.VersionTLS11)
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsVersion).To(Equal("TLSv1.3"))

			tlsVersion, err = ParseTLSVersionForStunnelPSK(ocpconfigv1.VersionTLS12)
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsVersion).To(Equal("TLSv1.3"))

			tlsVersion, err = ParseTLSVersionForStunnelPSK(ocpconfigv1.VersionTLS13)
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsVersion).To(Equal("TLSv1.3"))

			// Unknown version
			var fakeVersion ocpconfigv1.TLSProtocolVersion = "VersionNotExisting"
			tlsVersion, err = ParseTLSVersionForStunnelPSK(fakeVersion)
			Expect(err).To(HaveOccurred())
			Expect(tlsVersion).To(Equal(""))
		})
	})
})
