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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Discovery", func() {
	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(volsyncv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	Describe("discoverSourceInfo", func() {
		var (
			kb     *Builder
			logger logr.Logger
		)

		BeforeEach(func() {
			kb = &Builder{}
			logger = logr.Discard()
		})

		Context("when discovering PVC name, sourcePathOverride, and repository", func() {
			It("should discover all fields from a valid ReplicationSource", func() {
				existingSource := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "source-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							SourcePathOverride: ptr.To("/custom/path"),
							Repository:         "test-repo-secret",
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(existingSource).
					Build()

				info := kb.discoverSourceInfo(fakeClient, "test-source", "test-ns", logger)

				Expect(info.pvcName).To(Equal("source-pvc"))
				Expect(info.sourcePathOverride).NotTo(BeNil())
				Expect(*info.sourcePathOverride).To(Equal("/custom/path"))
				Expect(info.repository).To(Equal("test-repo-secret"))
			})
		})

		Context("when discovering PVC name without sourcePathOverride or repository", func() {
			It("should discover only PVC name", func() {
				existingSource := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "source-pvc",
						Kopia:     &volsyncv1alpha1.ReplicationSourceKopiaSpec{},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(existingSource).
					Build()

				info := kb.discoverSourceInfo(fakeClient, "test-source", "test-ns", logger)

				Expect(info.pvcName).To(Equal("source-pvc"))
				Expect(info.sourcePathOverride).To(BeNil())
				Expect(info.repository).To(BeEmpty())
			})
		})

		Context("when source doesn't exist", func() {
			It("should return empty info", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				info := kb.discoverSourceInfo(fakeClient, "missing-source", "test-ns", logger)

				Expect(info.pvcName).To(BeEmpty())
				Expect(info.sourcePathOverride).To(BeNil())
				Expect(info.repository).To(BeEmpty())
			})
		})

		Context("when source doesn't use Kopia", func() {
			It("should return empty info", func() {
				existingSource := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "source-pvc",
						// No Kopia spec
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(existingSource).
					Build()

				info := kb.discoverSourceInfo(fakeClient, "test-source", "test-ns", logger)

				Expect(info.pvcName).To(BeEmpty())
				Expect(info.sourcePathOverride).To(BeNil())
				Expect(info.repository).To(BeEmpty())
			})
		})

		Context("when source name is empty", func() {
			It("should return empty info", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				info := kb.discoverSourceInfo(fakeClient, "", "test-ns", logger)

				Expect(info.pvcName).To(BeEmpty())
				Expect(info.sourcePathOverride).To(BeNil())
				Expect(info.repository).To(BeEmpty())
			})
		})

		Context("when namespace is empty", func() {
			It("should return empty info", func() {
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					Build()

				info := kb.discoverSourceInfo(fakeClient, "test-source", "", logger)

				Expect(info.pvcName).To(BeEmpty())
				Expect(info.sourcePathOverride).To(BeNil())
				Expect(info.repository).To(BeEmpty())
			})
		})

		Context("when only repository is specified", func() {
			It("should discover only repository when PVC and sourcePathOverride are empty", func() {
				existingSource := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "repo-only-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						// No SourcePVC specified
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "repo-secret-name",
							// No sourcePathOverride specified
						},
					},
				}

				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(existingSource).
					Build()

				info := kb.discoverSourceInfo(fakeClient, "repo-only-source", "test-ns", logger)

				Expect(info.pvcName).To(BeEmpty())
				Expect(info.sourcePathOverride).To(BeNil())
				Expect(info.repository).To(Equal("repo-secret-name"))
			})
		})
	})

	Describe("sourcePathOverride in destination environment variables", func() {
		Context("when sourcePathOverride is set for destination", func() {
			It("should include KOPIA_SOURCE_PATH_OVERRIDE env var", func() {
				owner := &volsyncv1alpha1.ReplicationDestination{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-destination",
						Namespace: "test-ns",
					},
				}

				m := &Mover{
					sourcePathOverride: ptr.To("/custom/restore/path"),
					isSource:           false,
					repositoryName:     "test-repo",
					username:           "test-user",
					hostname:           "test-host",
					owner:              owner,
				}

				secret := &corev1.Secret{
					Data: map[string][]byte{
						"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
						"KOPIA_PASSWORD":   []byte("password"),
					},
				}

				envVars := m.buildEnvironmentVariables(secret)

				var found bool
				var actualValue string
				for _, env := range envVars {
					if env.Name == "KOPIA_SOURCE_PATH_OVERRIDE" {
						found = true
						actualValue = env.Value
						break
					}
				}

				Expect(found).To(BeTrue(), "Expected KOPIA_SOURCE_PATH_OVERRIDE env var to be present")
				Expect(actualValue).To(Equal("/custom/restore/path"))
			})
		})

		Context("when sourcePathOverride is set for source", func() {
			It("should include KOPIA_SOURCE_PATH_OVERRIDE env var", func() {
				owner := &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-source",
						Namespace: "test-ns",
					},
				}

				m := &Mover{
					sourcePathOverride: ptr.To("/custom/backup/path"),
					isSource:           true,
					repositoryName:     "test-repo",
					username:           "test-user",
					hostname:           "test-host",
					owner:              owner,
				}

				secret := &corev1.Secret{
					Data: map[string][]byte{
						"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
						"KOPIA_PASSWORD":   []byte("password"),
					},
				}

				envVars := m.buildEnvironmentVariables(secret)

				var found bool
				var actualValue string
				for _, env := range envVars {
					if env.Name == "KOPIA_SOURCE_PATH_OVERRIDE" {
						found = true
						actualValue = env.Value
						break
					}
				}

				Expect(found).To(BeTrue(), "Expected KOPIA_SOURCE_PATH_OVERRIDE env var to be present")
				Expect(actualValue).To(Equal("/custom/backup/path"))
			})
		})

		Context("when sourcePathOverride is nil", func() {
			It("should not include KOPIA_SOURCE_PATH_OVERRIDE env var", func() {
				owner := &volsyncv1alpha1.ReplicationDestination{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-destination",
						Namespace: "test-ns",
					},
				}

				m := &Mover{
					sourcePathOverride: nil,
					isSource:           false,
					repositoryName:     "test-repo",
					username:           "test-user",
					hostname:           "test-host",
					owner:              owner,
				}

				secret := &corev1.Secret{
					Data: map[string][]byte{
						"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
						"KOPIA_PASSWORD":   []byte("password"),
					},
				}

				envVars := m.buildEnvironmentVariables(secret)

				var found bool
				for _, env := range envVars {
					if env.Name == "KOPIA_SOURCE_PATH_OVERRIDE" {
						found = true
						break
					}
				}

				Expect(found).To(BeFalse(), "Did not expect KOPIA_SOURCE_PATH_OVERRIDE env var to be present")
			})
		})
	})

	Describe("discoverSourcePVC backward compatibility", func() {
		It("should return PVC name from discoverSourceInfo", func() {
			source := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-ns",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					SourcePVC: "source-pvc",
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						SourcePathOverride: ptr.To("/custom/path"),
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(source).
				Build()

			kb := &Builder{}
			logger := logr.Discard()

			pvcName := kb.discoverSourcePVC(fakeClient, "test-source", "test-ns", logger)

			Expect(pvcName).To(Equal("source-pvc"))
		})
	})
})
