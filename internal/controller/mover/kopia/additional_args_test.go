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
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Additional Args", func() {
	Describe("addAdditionalArgsEnvVar", func() {
		Context("when a single argument is provided", func() {
			It("should add the argument as env var value", func() {
				m := &Mover{
					logger: logr.Discard(),
					additionalArgs: []string{
						"--one-file-system",
					},
				}

				envVars := []corev1.EnvVar{}
				result := m.addAdditionalArgsEnvVar(envVars)

				Expect(result).To(HaveLen(1))
				Expect(result[0].Name).To(Equal("KOPIA_ADDITIONAL_ARGS"))
				Expect(result[0].Value).To(Equal("--one-file-system"))
			})
		})

		Context("when multiple arguments are provided", func() {
			It("should join arguments with VOLSYNC_ARG_SEP separator", func() {
				m := &Mover{
					logger: logr.Discard(),
					additionalArgs: []string{
						"--one-file-system",
						"--parallel=8",
						"--ignore-cache-dirs",
					},
				}

				envVars := []corev1.EnvVar{}
				result := m.addAdditionalArgsEnvVar(envVars)

				Expect(result).To(HaveLen(1))
				Expect(result[0].Name).To(Equal("KOPIA_ADDITIONAL_ARGS"))
				Expect(result[0].Value).To(Equal("--one-file-system|VOLSYNC_ARG_SEP|--parallel=8|VOLSYNC_ARG_SEP|--ignore-cache-dirs"))
			})
		})

		Context("when arguments contain equals signs", func() {
			It("should preserve equals signs in argument values", func() {
				m := &Mover{
					logger: logr.Discard(),
					additionalArgs: []string{
						"--compression=zstd",
						"--upload-speed=100MB",
					},
				}

				envVars := []corev1.EnvVar{}
				result := m.addAdditionalArgsEnvVar(envVars)

				Expect(result).To(HaveLen(1))
				Expect(result[0].Name).To(Equal("KOPIA_ADDITIONAL_ARGS"))
				Expect(result[0].Value).To(Equal("--compression=zstd|VOLSYNC_ARG_SEP|--upload-speed=100MB"))
			})
		})

		Context("when arguments contain special characters", func() {
			It("should preserve special characters in argument values", func() {
				m := &Mover{
					logger: logr.Discard(),
					additionalArgs: []string{
						"--exclude=*.tmp",
						"--exclude=cache/",
					},
				}

				envVars := []corev1.EnvVar{}
				result := m.addAdditionalArgsEnvVar(envVars)

				Expect(result).To(HaveLen(1))
				Expect(result[0].Name).To(Equal("KOPIA_ADDITIONAL_ARGS"))
				Expect(result[0].Value).To(Equal("--exclude=*.tmp|VOLSYNC_ARG_SEP|--exclude=cache/"))
			})
		})

		Context("when arguments that would have been blocked before are provided", func() {
			It("should allow all arguments without validation", func() {
				m := &Mover{
					logger: logr.Discard(),
					additionalArgs: []string{
						"--password=secret",
						"--config-file=/etc/kopia",
						"--username=myuser",
					},
				}

				envVars := []corev1.EnvVar{}
				result := m.addAdditionalArgsEnvVar(envVars)

				Expect(result).To(HaveLen(1))
				Expect(result[0].Name).To(Equal("KOPIA_ADDITIONAL_ARGS"))
				Expect(result[0].Value).To(Equal("--password=secret|VOLSYNC_ARG_SEP|--config-file=/etc/kopia|VOLSYNC_ARG_SEP|--username=myuser"))
			})
		})

		Context("when empty args are provided", func() {
			It("should not add KOPIA_ADDITIONAL_ARGS env var", func() {
				m := &Mover{
					logger:         logr.Discard(),
					additionalArgs: []string{},
				}

				envVars := []corev1.EnvVar{}
				result := m.addAdditionalArgsEnvVar(envVars)

				for _, env := range result {
					Expect(env.Name).NotTo(Equal("KOPIA_ADDITIONAL_ARGS"))
				}
			})
		})

		Context("when nil args are provided", func() {
			It("should not add KOPIA_ADDITIONAL_ARGS env var", func() {
				m := &Mover{
					logger:         logr.Discard(),
					additionalArgs: nil,
				}

				envVars := []corev1.EnvVar{}
				result := m.addAdditionalArgsEnvVar(envVars)

				for _, env := range result {
					Expect(env.Name).NotTo(Equal("KOPIA_ADDITIONAL_ARGS"))
				}
			})
		})
	})

	Describe("Additional Args Integration", func() {
		Context("when ReplicationSource includes additional args", func() {
			It("should properly pass args through the builder", func() {
				m := &Mover{
					logger: logr.Discard(),
					additionalArgs: []string{
						"--one-file-system",
						"--parallel=8",
					},
				}

				envVars := m.addAdditionalArgsEnvVar([]corev1.EnvVar{})

				Expect(envVars).To(HaveLen(1))
				Expect(envVars[0].Name).To(Equal("KOPIA_ADDITIONAL_ARGS"))

				expected := "--one-file-system|VOLSYNC_ARG_SEP|--parallel=8"
				Expect(envVars[0].Value).To(Equal(expected))
			})
		})

		Context("when no validation is applied", func() {
			It("should allow all args without validation", func() {
				m := &Mover{
					logger: logr.Discard(),
					additionalArgs: []string{
						"--password=secret",
						"--config-file=/custom/config",
						"--repository=s3://bucket",
					},
				}

				envVars := m.addAdditionalArgsEnvVar([]corev1.EnvVar{})

				Expect(envVars).To(HaveLen(1))

				expected := "--password=secret|VOLSYNC_ARG_SEP|--config-file=/custom/config|VOLSYNC_ARG_SEP|--repository=s3://bucket"
				Expect(envVars[0].Value).To(Equal(expected))
			})
		})
	})
})
