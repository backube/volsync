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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Kopia additional args", func() {
	var m *Mover
	var logger logr.Logger

	BeforeEach(func() {
		logger = GinkgoLogr
		m = &Mover{
			logger: logger,
		}
	})

	Context("validateAdditionalArgs", func() {
		It("should accept valid additional arguments", func() {
			m.additionalArgs = []string{
				"--one-file-system",
				"--ignore-cache-dirs",
				"--parallel=8",
				"--upload-speed=100MB",
				"--compression=s2",
			}
			err := m.validateAdditionalArgs()
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject dangerous password flags", func() {
			m.additionalArgs = []string{
				"--one-file-system",
				"--password=secret",
			}
			err := m.validateAdditionalArgs()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--password"))
			Expect(err.Error()).To(ContainSubstring("not allowed"))
		})

		It("should reject config-file flags", func() {
			m.additionalArgs = []string{
				"--config-file=/path/to/config",
			}
			err := m.validateAdditionalArgs()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--config-file"))
		})

		It("should reject repository override flags", func() {
			m.additionalArgs = []string{
				"--repository=s3://different-bucket",
			}
			err := m.validateAdditionalArgs()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--repository"))
		})

		It("should reject credential-related flags", func() {
			dangerousArgs := []string{
				"--access-key=AKIAIOSFODNN7EXAMPLE",
				"--secret-access-key=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"--session-token=token",
				"--storage-account=myaccount",
				"--storage-key=mykey",
				"--credentials-file=/path/to/creds",
				"--key-id=keyid",
				"--key=secret",
			}

			for _, arg := range dangerousArgs {
				m.additionalArgs = []string{arg}
				err := m.validateAdditionalArgs()
				Expect(err).To(HaveOccurred(), "Expected error for arg: %s", arg)
			}
		})

		It("should reject override-username and override-hostname flags", func() {
			m.additionalArgs = []string{
				"--override-username=different-user",
			}
			err := m.validateAdditionalArgs()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--override-username"))

			m.additionalArgs = []string{
				"--override-hostname=different-host",
			}
			err = m.validateAdditionalArgs()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("--override-hostname"))
		})

		It("should handle flags with and without equals sign", func() {
			// With equals
			m.additionalArgs = []string{"--password=secret"}
			err := m.validateAdditionalArgs()
			Expect(err).To(HaveOccurred())

			// Without equals (space-separated value would be a separate arg)
			m.additionalArgs = []string{"--password"}
			err = m.validateAdditionalArgs()
			Expect(err).To(HaveOccurred())
		})

		It("should accept empty additional args", func() {
			m.additionalArgs = []string{}
			err := m.validateAdditionalArgs()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("addAdditionalArgsEnvVar", func() {
		var envVars []corev1.EnvVar

		BeforeEach(func() {
			envVars = []corev1.EnvVar{}
		})

		It("should add valid additional args as environment variable", func() {
			m.additionalArgs = []string{
				"--one-file-system",
				"--ignore-cache-dirs",
				"--parallel=8",
			}
			
			result := m.addAdditionalArgsEnvVar(envVars)
			
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("KOPIA_ADDITIONAL_ARGS"))
			
			// Check that args are joined with the delimiter
			expectedValue := strings.Join(m.additionalArgs, "|VOLSYNC_ARG_SEP|")
			Expect(result[0].Value).To(Equal(expectedValue))
		})

		It("should not add environment variable for empty args", func() {
			m.additionalArgs = []string{}
			
			result := m.addAdditionalArgsEnvVar(envVars)
			
			Expect(result).To(BeEmpty())
		})

		It("should not add environment variable for invalid args", func() {
			m.additionalArgs = []string{
				"--one-file-system",
				"--password=secret", // Invalid arg
			}
			
			result := m.addAdditionalArgsEnvVar(envVars)
			
			// Should not add the env var due to validation failure
			Expect(result).To(BeEmpty())
		})

		It("should preserve existing environment variables", func() {
			existingVar := corev1.EnvVar{
				Name:  "EXISTING_VAR",
				Value: "existing_value",
			}
			envVars = append(envVars, existingVar)
			
			m.additionalArgs = []string{"--one-file-system"}
			
			result := m.addAdditionalArgsEnvVar(envVars)
			
			Expect(result).To(HaveLen(2))
			Expect(result[0]).To(Equal(existingVar))
			Expect(result[1].Name).To(Equal("KOPIA_ADDITIONAL_ARGS"))
		})

		It("should handle args with special characters", func() {
			m.additionalArgs = []string{
				"--ignore=/path/with spaces/file.txt",
				"--description=This is a test",
				"--pattern=*.log",
			}
			
			result := m.addAdditionalArgsEnvVar(envVars)
			
			Expect(result).To(HaveLen(1))
			// Verify the args are properly delimited
			value := result[0].Value
			parts := strings.Split(value, "|VOLSYNC_ARG_SEP|")
			Expect(parts).To(HaveLen(3))
			Expect(parts[0]).To(Equal("--ignore=/path/with spaces/file.txt"))
			Expect(parts[1]).To(Equal("--description=This is a test"))
			Expect(parts[2]).To(Equal("--pattern=*.log"))
		})
	})
})