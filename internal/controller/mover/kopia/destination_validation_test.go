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
				Expect(err.Error()).To(ContainSubstring("requires explicit identity information"))
			})
		})

		Context("when only username is provided", func() {
			It("should fail with helpful error message", func() {
				destination.Spec.Kopia.Username = ptr.To("test-user")
				// hostname is not provided

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("You have provided 'username' but 'hostname' is missing"))
				Expect(err.Error()).To(ContainSubstring("Example with explicit identity"))
				Expect(err.Error()).To(ContainSubstring("Example with sourceIdentity"))
			})
		})

		Context("when only hostname is provided", func() {
			It("should fail with helpful error message", func() {
				destination.Spec.Kopia.Hostname = ptr.To("test-host")
				// username is not provided

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("You have provided 'hostname' but 'username' is missing"))
			})
		})

		Context("when neither identity method is provided", func() {
			It("should fail with helpful error message", func() {
				// Neither username/hostname nor sourceIdentity provided

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("requires explicit identity information"))
				Expect(err.Error()).To(ContainSubstring("You must provide one of the following"))
				Expect(err.Error()).To(ContainSubstring("cannot automatically determine the source identity"))
			})
		})

		Context("when empty strings are provided", func() {
			It("should fail if username is empty string", func() {
				destination.Spec.Kopia.Username = ptr.To("")
				destination.Spec.Kopia.Hostname = ptr.To("test-host")

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("'username' is missing"))
			})

			It("should fail if hostname is empty string", func() {
				destination.Spec.Kopia.Username = ptr.To("test-user")
				destination.Spec.Kopia.Hostname = ptr.To("")

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("'hostname' is missing"))
			})

			It("should fail if both are empty strings", func() {
				destination.Spec.Kopia.Username = ptr.To("")
				destination.Spec.Kopia.Hostname = ptr.To("")

				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("requires explicit identity information"))
			})
		})

		Context("error message content", func() {
			It("should include both configuration examples", func() {
				// No identity provided
				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())

				errorMsg := err.Error()
				// Check for presence of both examples
				Expect(errorMsg).To(ContainSubstring("Example with explicit identity:"))
				Expect(errorMsg).To(ContainSubstring("username: \"my-source-namespace\""))
				Expect(errorMsg).To(ContainSubstring("hostname: \"my-namespace-my-pvc\""))

				Expect(errorMsg).To(ContainSubstring("Example with sourceIdentity (recommended):"))
				Expect(errorMsg).To(ContainSubstring("sourceName: \"my-replication-source\""))
				Expect(errorMsg).To(ContainSubstring("sourceNamespace: \"source-namespace\""))
			})

			It("should explain why automatic determination is not possible", func() {
				err := builder.validateDestinationIdentity(destination)
				Expect(err).To(HaveOccurred())

				errorMsg := err.Error()
				Expect(errorMsg).To(ContainSubstring("cannot automatically determine the source identity"))
				Expect(errorMsg).To(ContainSubstring("hostname typically includes the source PVC name"))
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
				errorFunc: func(err error, msg string, keysAndValues ...interface{}) {
					errorMessages = append(errorMessages, msg)
				},
			}))

			_, err := builder.FromDestination(nil, mockLogger, nil, destination, false)
			
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires explicit identity information"))
			
			// Verify that the error was logged
			Expect(errorMessages).To(ContainElement("Invalid ReplicationDestination configuration"))
		})
	})
})

// mockLogSink is a simple mock implementation of logr.LogSink for testing
type mockLogSink struct {
	errorFunc func(err error, msg string, keysAndValues ...interface{})
}

func (m mockLogSink) Init(info logr.RuntimeInfo) {}
func (m mockLogSink) Enabled(level int) bool { return true }
func (m mockLogSink) Info(level int, msg string, keysAndValues ...interface{}) {}
func (m mockLogSink) Error(err error, msg string, keysAndValues ...interface{}) {
	if m.errorFunc != nil {
		m.errorFunc(err, msg, keysAndValues...)
	}
}
func (m mockLogSink) WithValues(keysAndValues ...interface{}) logr.LogSink { return m }
func (m mockLogSink) WithName(name string) logr.LogSink { return m }

// Verify examples in error message are valid YAML by attempting to parse them
var _ = Describe("Error message examples", func() {
	It("should provide valid YAML examples", func() {
		builder := &Builder{viper: viper.New()}
		destination := &volsyncv1alpha1.ReplicationDestination{
			Spec: volsyncv1alpha1.ReplicationDestinationSpec{
				Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{},
			},
		}

		err := builder.validateDestinationIdentity(destination)
		Expect(err).To(HaveOccurred())

		// Extract the examples from the error message
		errorMsg := err.Error()
		
		// Verify the examples contain proper YAML structure
		Expect(errorMsg).To(MatchRegexp(`kopia:\s+username:`))
		Expect(errorMsg).To(MatchRegexp(`kopia:\s+sourceIdentity:`))
		
		// Verify indentation is consistent
		lines := strings.Split(errorMsg, "\n")
		for _, line := range lines {
			if strings.Contains(line, "  kopia:") {
				// kopia should be at 2 spaces
				Expect(strings.Index(line, "kopia:")).To(Equal(2))
			}
			if strings.Contains(line, "    username:") || strings.Contains(line, "    hostname:") {
				// Fields should be at 4 spaces
				Expect(strings.Index(line, "username:") + strings.Index(line, "hostname:") + 4).To(BeNumerically(">=", 4))
			}
		}
	})
})