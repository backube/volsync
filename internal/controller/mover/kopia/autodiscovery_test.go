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
	"errors"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Kopia Auto-discovery", func() {
	var (
		logger logr.Logger
		scheme *runtime.Scheme
		kb     *Builder
	)

	BeforeEach(func() {
		logger = logr.Discard()
		scheme = runtime.NewScheme()
		Expect(volsyncv1alpha1.AddToScheme(scheme)).To(Succeed())
		kb = &Builder{}
	})

	Describe("discoverSourcePVC", func() {
		It("should return empty string when source name is empty", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			result := kb.discoverSourcePVC(c, "", "test-ns", logger)
			Expect(result).To(Equal(""))
		})

		It("should return empty string when source namespace is empty", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			result := kb.discoverSourcePVC(c, "test-source", "", logger)
			Expect(result).To(Equal(""))
		})

		It("should return empty string when ReplicationSource is not found", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			result := kb.discoverSourcePVC(c, "nonexistent", "test-ns", logger)
			Expect(result).To(Equal(""))
		})

		It("should return empty string when ReplicationSource doesn't use Kopia", func() {
			source := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-ns",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					SourcePVC: "test-pvc",
					// Not using Kopia - using Rsync instead
					Rsync: &volsyncv1alpha1.ReplicationSourceRsyncSpec{},
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source).Build()
			result := kb.discoverSourcePVC(c, "test-source", "test-ns", logger)
			Expect(result).To(Equal(""))
		})

		It("should return source PVC when ReplicationSource uses Kopia", func() {
			source := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-ns",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					SourcePVC: "my-data-pvc",
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: "kopia-repo",
					},
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source).Build()
			result := kb.discoverSourcePVC(c, "test-source", "test-ns", logger)
			Expect(result).To(Equal("my-data-pvc"))
		})

		It("should return empty string when Kopia ReplicationSource has no source PVC", func() {
			source := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-ns",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					// No SourcePVC specified
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: "kopia-repo",
					},
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source).Build()
			result := kb.discoverSourcePVC(c, "test-source", "test-ns", logger)
			Expect(result).To(Equal(""))
		})

		It("should handle permission errors gracefully", func() {
			// Create a mock client that returns a forbidden error
			mockClient := &mockClientWithError{
				err: kerrors.NewForbidden(
					schema.GroupResource{Group: "volsync.backube", Resource: "replicationsources"},
					"test-source",
					errors.New("user cannot get resource"),
				),
			}
			result := kb.discoverSourcePVC(mockClient, "test-source", "test-ns", logger)
			Expect(result).To(Equal(""))
		})
	})

	Describe("Integration with FromDestination", func() {
		It("should auto-discover source PVC when using sourceIdentity", func() {
			// Create a ReplicationSource that will be discovered
			source := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-backup",
					Namespace: "production",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					SourcePVC: "app-data-pvc",
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: "kopia-repo",
					},
				},
			}

			// Create a ReplicationDestination that uses sourceIdentity
			destination := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-restore",
					Namespace: "production",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
						ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
							DestinationPVC: ptr.To("restore-pvc"),
						},
						Repository: "kopia-repo",
						SourceIdentity: &volsyncv1alpha1.KopiaSourceIdentity{
							SourceName:      "app-backup",
							SourceNamespace: "production",
							// No SourcePVCName - should be auto-discovered
						},
					},
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source).Build()

			// Build the mover
			builder, err := newBuilder(nil, nil)
			Expect(err).ToNot(HaveOccurred())

			mover, err := builder.FromDestination(c, logger, nil, destination, false)
			Expect(err).ToNot(HaveOccurred())
			Expect(mover).ToNot(BeNil())

			// Verify the hostname was generated using the discovered PVC name
			kopiaMover, ok := mover.(*Mover)
			Expect(ok).To(BeTrue(), "mover should be of type *Mover")
			// The hostname should be generated from namespace and discovered PVC name
			expectedHostname := generateHostname(nil, ptr.To("app-data-pvc"), "production", "app-backup")
			Expect(kopiaMover.hostname).To(Equal(expectedHostname))
		})

		It("should use explicit sourcePVCName over auto-discovery", func() {
			// Create a ReplicationSource with a different PVC name
			source := &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-backup",
					Namespace: "production",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					SourcePVC: "actual-pvc",
					Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
						Repository: "kopia-repo",
					},
				},
			}

			// Create a ReplicationDestination with explicit sourcePVCName
			destination := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-restore",
					Namespace: "production",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
						ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
							DestinationPVC: ptr.To("restore-pvc"),
						},
						Repository: "kopia-repo",
						SourceIdentity: &volsyncv1alpha1.KopiaSourceIdentity{
							SourceName:      "app-backup",
							SourceNamespace: "production",
							SourcePVCName:   "explicit-pvc", // Explicitly specified
						},
					},
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(source).Build()

			builder, err := newBuilder(nil, nil)
			Expect(err).ToNot(HaveOccurred())

			mover, err := builder.FromDestination(c, logger, nil, destination, false)
			Expect(err).ToNot(HaveOccurred())
			Expect(mover).ToNot(BeNil())

			// Verify the explicit PVC name was used, not the discovered one
			kopiaMover, ok := mover.(*Mover)
			Expect(ok).To(BeTrue(), "mover should be of type *Mover")
			expectedHostname := generateHostname(nil, ptr.To("explicit-pvc"), "production", "app-backup")
			Expect(kopiaMover.hostname).To(Equal(expectedHostname))
		})

		It("should fallback to destination PVC when auto-discovery fails", func() {
			// No ReplicationSource exists

			destination := &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-restore",
					Namespace: "production",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
						ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
							DestinationPVC: ptr.To("fallback-pvc"),
						},
						Repository: "kopia-repo",
						SourceIdentity: &volsyncv1alpha1.KopiaSourceIdentity{
							SourceName:      "nonexistent",
							SourceNamespace: "production",
							// No SourcePVCName and auto-discovery will fail
						},
					},
				},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).Build()

			builder, err := newBuilder(nil, nil)
			Expect(err).ToNot(HaveOccurred())

			mover, err := builder.FromDestination(c, logger, nil, destination, false)
			Expect(err).ToNot(HaveOccurred())
			Expect(mover).ToNot(BeNil())

			// Verify fallback to destination PVC
			kopiaMover, ok := mover.(*Mover)
			Expect(ok).To(BeTrue(), "mover should be of type *Mover")
			expectedHostname := generateHostname(nil, ptr.To("fallback-pvc"), "production", "nonexistent")
			Expect(kopiaMover.hostname).To(Equal(expectedHostname))
		})
	})
})

// mockClientWithError is a mock client that returns a specific error
type mockClientWithError struct {
	client.Client
	err error
}

func (m *mockClientWithError) Get(_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption) error {
	return m.err
}
