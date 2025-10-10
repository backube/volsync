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
	"testing"

	"github.com/go-logr/logr"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

// TestDiscoverSourcePVC tests the auto-discovery functionality
//
//nolint:funlen
func TestDiscoverSourcePVC(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := volsyncv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("Failed to add scheme: %v", err)
	}

	logger := logr.Discard()
	kb := &Builder{}

	tests := []struct {
		name            string
		sourceName      string
		sourceNamespace string
		objects         []client.Object
		expectedPVC     string
		setupMock       func() client.Client
	}{
		{
			name:            "returns empty when source name is empty",
			sourceName:      "",
			sourceNamespace: "test-ns",
			expectedPVC:     "",
		},
		{
			name:            "returns empty when source namespace is empty",
			sourceName:      "test-source",
			sourceNamespace: "",
			expectedPVC:     "",
		},
		{
			name:            "returns empty when ReplicationSource not found",
			sourceName:      "nonexistent",
			sourceNamespace: "test-ns",
			objects:         []client.Object{},
			expectedPVC:     "",
		},
		{
			name:            "returns empty when ReplicationSource doesn't use Kopia",
			sourceName:      "rsync-source",
			sourceNamespace: "test-ns",
			objects: []client.Object{
				&volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "rsync-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "test-pvc",
						Rsync:     &volsyncv1alpha1.ReplicationSourceRsyncSpec{},
					},
				},
			},
			expectedPVC: "",
		},
		{
			name:            "returns PVC when ReplicationSource uses Kopia",
			sourceName:      "kopia-source",
			sourceNamespace: "test-ns",
			objects: []client.Object{
				&volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kopia-source",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						SourcePVC: "my-data-pvc",
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "kopia-repo",
						},
					},
				},
			},
			expectedPVC: "my-data-pvc",
		},
		{
			name:            "returns empty when Kopia source has no PVC",
			sourceName:      "kopia-source-no-pvc",
			sourceNamespace: "test-ns",
			objects: []client.Object{
				&volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kopia-source-no-pvc",
						Namespace: "test-ns",
					},
					Spec: volsyncv1alpha1.ReplicationSourceSpec{
						Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
							Repository: "kopia-repo",
						},
					},
				},
			},
			expectedPVC: "",
		},
		{
			name:            "handles permission errors gracefully",
			sourceName:      "test-source",
			sourceNamespace: "test-ns",
			expectedPVC:     "",
			setupMock: func() client.Client {
				return &mockClientWithErrorUnit{
					err: kerrors.NewForbidden(
						schema.GroupResource{Group: "volsync.backube", Resource: "replicationsources"},
						"test-source",
						errors.New("user cannot get resource"),
					),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c client.Client
			if tt.setupMock != nil {
				c = tt.setupMock()
			} else {
				c = fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()
			}

			result := kb.discoverSourcePVC(c, tt.sourceName, tt.sourceNamespace, logger)
			if result != tt.expectedPVC {
				t.Errorf("discoverSourcePVC() = %q, want %q", result, tt.expectedPVC)
			}
		})
	}
}

// mockClientWithErrorUnit is a mock client that returns a specific error for unit tests
type mockClientWithErrorUnit struct {
	client.Client
	err error
}

func (m *mockClientWithErrorUnit) Get(
	_ context.Context, _ client.ObjectKey, _ client.Object, _ ...client.GetOption,
) error {
	return m.err
}
