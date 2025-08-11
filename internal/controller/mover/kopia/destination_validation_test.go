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

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/viper"
	"k8s.io/utils/ptr"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Kopia ReplicationDestination validation", func() {
	var (
		builder     *Builder
		destination *volsyncv1alpha1.ReplicationDestination
	)

	BeforeEach(func() {
		// Create a builder with test configuration
		testViper := viper.New()
		testViper.Set(kopiaContainerImageFlag, "test-image:latest")
		builder = &Builder{
			viper: testViper,
		}

		// Create a basic ReplicationDestination with Kopia spec
		destination = &volsyncv1alpha1.ReplicationDestination{
			Spec: volsyncv1alpha1.ReplicationDestinationSpec{
				Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
					Repository: "test-repo",
				},
			},
		}
	})

	Describe("validateDestinationIdentity", func() {
		Context("when both username and hostname are provided", func() {
			It("should succeed", func() {
				destination.Spec.Kopia.Username = ptr.To("test-user")
				destination.Spec.Kopia.Hostname = ptr.To("test-host")

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(BeNil())
			})

			It("should succeed even if sourceIdentity is also provided", func() {
				destination.Spec.Kopia.Username = ptr.To("test-user")
				destination.Spec.Kopia.Hostname = ptr.To("test-host")
				destination.Spec.Kopia.SourceIdentity = &volsyncv1alpha1.KopiaSourceIdentity{
					SourceName: "source1",
				}

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(BeNil())
			})
		})

		Context("when sourceIdentity is provided", func() {
			It("should succeed with just sourceName", func() {
				destination.Spec.Kopia.SourceIdentity = &volsyncv1alpha1.KopiaSourceIdentity{
					SourceName: "my-source",
				}

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(BeNil())
			})

			It("should succeed with sourceName and sourceNamespace", func() {
				destination.Spec.Kopia.SourceIdentity = &volsyncv1alpha1.KopiaSourceIdentity{
					SourceName:      "my-source",
					SourceNamespace: "source-ns",
				}

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(BeNil())
			})

			It("should succeed with all sourceIdentity fields", func() {
				destination.Spec.Kopia.SourceIdentity = &volsyncv1alpha1.KopiaSourceIdentity{
					SourceName:         "my-source",
					SourceNamespace:    "source-ns",
					SourcePVCName:      "source-pvc",
					SourcePathOverride: ptr.To("/custom/path"),
				}

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(BeNil())
			})

			It("should fail if sourceIdentity exists but sourceName is empty", func() {
				destination.Spec.Kopia.SourceIdentity = &volsyncv1alpha1.KopiaSourceIdentity{
					SourceNamespace: "source-ns",
					// sourceName is empty
				}

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing identity configuration"))
			})
		})

		Context("when only username is provided", func() {
			It("should fail with helpful error message", func() {
				destination.Spec.Kopia.Username = ptr.To("test-user")
				// hostname is not provided

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing 'hostname'"))
				Expect(err.Error()).To(ContainSubstring("you provided 'username' but both are required"))
				Expect(err.Error()).To(ContainSubstring("https://volsync.readthedocs.io"))
			})
		})

		Context("when only hostname is provided", func() {
			It("should fail with helpful error message", func() {
				destination.Spec.Kopia.Hostname = ptr.To("test-host")
				// username is not provided

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing 'username'"))
				Expect(err.Error()).To(ContainSubstring("you provided 'hostname' but both are required"))
			})
		})

		Context("when neither identity method is provided", func() {
			It("should fail with helpful error message", func() {
				// Neither username/hostname nor sourceIdentity provided

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing identity configuration"))
				Expect(err.Error()).To(ContainSubstring("Provide either"))
				Expect(err.Error()).To(ContainSubstring("https://volsync.readthedocs.io"))
			})
		})

		Context("when empty strings are provided", func() {
			It("should fail if username is empty string", func() {
				destination.Spec.Kopia.Username = ptr.To("")
				destination.Spec.Kopia.Hostname = ptr.To("test-host")

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing 'username'"))
			})

			It("should fail if hostname is empty string", func() {
				destination.Spec.Kopia.Username = ptr.To("test-user")
				destination.Spec.Kopia.Hostname = ptr.To("")

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing 'hostname'"))
			})

			It("should fail if both are empty strings", func() {
				destination.Spec.Kopia.Username = ptr.To("")
				destination.Spec.Kopia.Hostname = ptr.To("")

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("missing identity configuration"))
			})
		})

		Context("error message content", func() {
			It("should include concise instructions", func() {
				// No identity provided
				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())

				errorMsg := err.Error()
				// Check for concise format
				Expect(errorMsg).To(ContainSubstring("Provide either:"))
				Expect(errorMsg).To(ContainSubstring("Both 'username' and 'hostname' fields"))
				Expect(errorMsg).To(ContainSubstring("'sourceIdentity' section with 'sourceName'"))
			})

			It("should include documentation link", func() {
				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())

				errorMsg := err.Error()
				Expect(errorMsg).To(ContainSubstring("https://volsync.readthedocs.io"))
			})
		})

		Context("when Kopia spec is nil", func() {
			It("should not return an error", func() {
				destination.Spec.Kopia = nil
				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(BeNil())
			})
		})
	})

	Describe("FromDestination integration", func() {
		It("should fail early if validation fails", func() {
			// Set up a destination without proper identity
			destination.Spec.Kopia = &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
				Repository: "test-repo",
				// No identity information provided
			}

			// Mock logger that captures errors
			errorMessages := []string{}
			mockLogger := logr.New(logr.LogSink(mockLogSink{
				errorFunc: func(_ error, msg string, _ ...interface{}) {
					errorMessages = append(errorMessages, msg)
				},
			}))

			_, err := builder.FromDestination(nil, mockLogger, nil, destination, false)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("missing identity configuration"))

			// Verify that the error was logged
			Expect(errorMessages).To(ContainElement("Invalid ReplicationDestination configuration"))
		})
	})
})

// mockLogSink is a simple mock implementation of logr.LogSink for testing
type mockLogSink struct {
	errorFunc func(err error, msg string, keysAndValues ...interface{})
}

func (m mockLogSink) Init(_ logr.RuntimeInfo)                {}
func (m mockLogSink) Enabled(_ int) bool                     { return true }
func (m mockLogSink) Info(_ int, _ string, _ ...interface{}) {}
func (m mockLogSink) Error(err error, msg string, keysAndValues ...interface{}) {
	if m.errorFunc != nil {
		m.errorFunc(err, msg, keysAndValues...)
	}
}
func (m mockLogSink) WithValues(_ ...interface{}) logr.LogSink { return m }
func (m mockLogSink) WithName(_ string) logr.LogSink           { return m }

// Verify error messages are concise and helpful
var _ = Describe("Error message format", func() {
	It("should provide concise error messages with documentation link", func() {
		builder := &Builder{viper: viper.New()}
		destination := &volsyncv1alpha1.ReplicationDestination{
			Spec: volsyncv1alpha1.ReplicationDestinationSpec{
				Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{},
			},
		}

		err := builder.validateDestinationIdentity(destination)
		Expect(err).To(HaveOccurred())

		errorMsg := err.Error()

		// Verify the error message is concise
		lines := strings.Split(errorMsg, "\n")
		Expect(len(lines)).To(BeNumerically("<=", 7), "Error message should be concise")

		// Verify it contains key information
		Expect(errorMsg).To(ContainSubstring("Kopia ReplicationDestination error"))
		Expect(errorMsg).To(ContainSubstring("Provide either:"))
		Expect(errorMsg).To(ContainSubstring("https://volsync.readthedocs.io"))

		// Verify it doesn't contain verbose YAML examples
		Expect(errorMsg).NotTo(ContainSubstring("Example with explicit identity:"))
		Expect(errorMsg).NotTo(ContainSubstring("Example with sourceIdentity"))
	})
})
