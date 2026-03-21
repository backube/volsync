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
	Describe("ParseTLSVersion", func() {
		It("Should convert the golang TLS version string into one sTunnel/openssl can use", func() {
			tlsVersion, err := ParseTLSVersion(ocpconfigv1.VersionTLS10)
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsVersion).To(Equal("TLSv1"))

			tlsVersion, err = ParseTLSVersion(ocpconfigv1.VersionTLS11)
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsVersion).To(Equal("TLSv1.1"))

			tlsVersion, err = ParseTLSVersion(ocpconfigv1.VersionTLS12)
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsVersion).To(Equal("TLSv1.2"))

			tlsVersion, err = ParseTLSVersion(ocpconfigv1.VersionTLS13)
			Expect(err).NotTo(HaveOccurred())
			Expect(tlsVersion).To(Equal("TLSv1.3"))

			// Unknown version
			var fakeVersion ocpconfigv1.TLSProtocolVersion = "VersionNotExisting"
			tlsVersion, err = ParseTLSVersion(fakeVersion)
			Expect(err).To(HaveOccurred())
			Expect(tlsVersion).To(Equal(""))
		})
	})

	Describe("ParseTLS13CipherSuitesForStunnelPSK", func() {
		var tlsProfileSpec ocpconfigv1.TLSProfileSpec
		var cipherSuites string

		JustBeforeEach(func() {
			cipherSuites = ParseTLS13CipherSuitesForStunnelPSK(tlsProfileSpec)
		})

		When("No ciphersuites are TLS 1.3", func() {
			BeforeEach(func() {
				tlsProfileSpec = ocpconfigv1.TLSProfileSpec{
					Ciphers: []string{
						"ECDHE-ECDSA-CHACHA20-POLY1305",
						"ECDHE-RSA-CHACHA20-POLY1305",
						"ECDHE-RSA-AES128-GCM-SHA256",
						"ECDHE-ECDSA-AES128-GCM-SHA256",
					},
				}
			})
			It("Should return an empty string", func() {
				Expect(cipherSuites).To(Equal(""))
			})
		})

		When("Some matching ciphersuites are TLS 1.3", func() {
			BeforeEach(func() {
				tlsProfileSpec = ocpconfigv1.TLSProfileSpec{
					Ciphers: []string{
						"ECDHE-ECDSA-CHACHA20-POLY1305",
						"ECDHE-RSA-CHACHA20-POLY1305",
						"TLS_AES_128_GCM_SHA256",
						"ECDHE-RSA-AES128-GCM-SHA256",
						"TLS_CHACHA20_POLY1305_SHA256",
						"ECDHE-ECDSA-AES128-GCM-SHA256",
					},
				}
			})
			It("Should return the matching ones, in order", func() {
				Expect(cipherSuites).To(Equal("TLS_AES_128_GCM_SHA256:TLS_CHACHA20_POLY1305_SHA256"))
			})
		})

		When("The only matching TLS 1.3 ciphersuites is TLS_AES_256_GCM_SHA384", func() {
			BeforeEach(func() {
				tlsProfileSpec = ocpconfigv1.TLSProfileSpec{
					Ciphers: []string{
						"ECDHE-ECDSA-CHACHA20-POLY1305",
						"TLS_AES_256_GCM_SHA384",
					},
				}
			})
			It("Should return an empty string (no match, using TLS_AES_256_GCM_SHA384 in sTunnel will fail", func() {
				Expect(cipherSuites).To(Equal(""))
			})
		})
	})
})
