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
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

func TestDiscoverSourceInfo(t *testing.T) {
	// Create a scheme with our types
	scheme := runtime.NewScheme()
	_ = volsyncv1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tests := []struct {
		name                       string
		sourceName                 string
		sourceNamespace            string
		existingSource             *volsyncv1alpha1.ReplicationSource
		expectedPVCName            string
		expectedSourcePathOverride *string
		expectError                bool
	}{
		{
			name:            "discovers both PVC name and sourcePathOverride",
			sourceName:      "test-source",
			sourceNamespace: "test-ns",
			existingSource: &volsyncv1alpha1.ReplicationSource{
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
			},
			expectedPVCName:            "source-pvc",
			expectedSourcePathOverride: ptr.To("/custom/path"),
		},
		{
			name:            "discovers PVC name without sourcePathOverride",
			sourceName:      "test-source",
			sourceNamespace: "test-ns",
			existingSource: &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-ns",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					SourcePVC: "source-pvc",
					Kopia:     &volsyncv1alpha1.ReplicationSourceKopiaSpec{},
				},
			},
			expectedPVCName:            "source-pvc",
			expectedSourcePathOverride: nil,
		},
		{
			name:                       "returns empty when source doesn't exist",
			sourceName:                 "missing-source",
			sourceNamespace:            "test-ns",
			existingSource:             nil,
			expectedPVCName:            "",
			expectedSourcePathOverride: nil,
		},
		{
			name:            "returns empty when source doesn't use Kopia",
			sourceName:      "test-source",
			sourceNamespace: "test-ns",
			existingSource: &volsyncv1alpha1.ReplicationSource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-source",
					Namespace: "test-ns",
				},
				Spec: volsyncv1alpha1.ReplicationSourceSpec{
					SourcePVC: "source-pvc",
					// No Kopia spec
				},
			},
			expectedPVCName:            "",
			expectedSourcePathOverride: nil,
		},
		{
			name:                       "handles empty source name",
			sourceName:                 "",
			sourceNamespace:            "test-ns",
			expectedPVCName:            "",
			expectedSourcePathOverride: nil,
		},
		{
			name:                       "handles empty namespace",
			sourceName:                 "test-source",
			sourceNamespace:            "",
			expectedPVCName:            "",
			expectedSourcePathOverride: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client
			var objs []client.Object
			if tt.existingSource != nil {
				objs = append(objs, tt.existingSource)
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objs...).
				Build()

			// Create builder
			kb := &Builder{}
			logger := logr.Discard()

			// Call the function
			info := kb.discoverSourceInfo(fakeClient, tt.sourceName, tt.sourceNamespace, logger)

			// Check PVC name
			if info.pvcName != tt.expectedPVCName {
				t.Errorf("Expected PVC name %q, got %q", tt.expectedPVCName, info.pvcName)
			}

			// Check sourcePathOverride
			if tt.expectedSourcePathOverride == nil && info.sourcePathOverride != nil {
				t.Errorf("Expected nil sourcePathOverride, got %q", *info.sourcePathOverride)
			} else if tt.expectedSourcePathOverride != nil && info.sourcePathOverride == nil {
				t.Errorf("Expected sourcePathOverride %q, got nil", *tt.expectedSourcePathOverride)
			} else if tt.expectedSourcePathOverride != nil && info.sourcePathOverride != nil {
				if *tt.expectedSourcePathOverride != *info.sourcePathOverride {
					t.Errorf("Expected sourcePathOverride %q, got %q",
						*tt.expectedSourcePathOverride, *info.sourcePathOverride)
				}
			}
		})
	}
}

// TestSourcePathOverrideInDestinationEnvVars tests that sourcePathOverride is properly
// included in destination environment variables
func TestSourcePathOverrideInDestinationEnvVars(t *testing.T) {
	tests := []struct {
		name               string
		sourcePathOverride *string
		isSource           bool
		expectEnvVar       bool
		expectedValue      string
	}{
		{
			name:               "includes sourcePathOverride for destination",
			sourcePathOverride: ptr.To("/custom/restore/path"),
			isSource:           false,
			expectEnvVar:       true,
			expectedValue:      "/custom/restore/path",
		},
		{
			name:               "includes sourcePathOverride for source",
			sourcePathOverride: ptr.To("/custom/backup/path"),
			isSource:           true,
			expectEnvVar:       true,
			expectedValue:      "/custom/backup/path",
		},
		{
			name:               "no env var when sourcePathOverride is nil",
			sourcePathOverride: nil,
			isSource:           false,
			expectEnvVar:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock owner object
			var owner client.Object
			if tt.isSource {
				owner = &volsyncv1alpha1.ReplicationSource{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-source",
						Namespace: "test-ns",
					},
				}
			} else {
				owner = &volsyncv1alpha1.ReplicationDestination{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-destination",
						Namespace: "test-ns",
					},
				}
			}

			m := &Mover{
				sourcePathOverride: tt.sourcePathOverride,
				isSource:           tt.isSource,
				repositoryName:     "test-repo",
				username:           "test-user",
				hostname:           "test-host",
				owner:              owner,
			}

			// Create a mock secret
			secret := &corev1.Secret{
				Data: map[string][]byte{
					"KOPIA_REPOSITORY": []byte("s3://bucket/path"),
					"KOPIA_PASSWORD":   []byte("password"),
				},
			}

			envVars := m.buildEnvironmentVariables(secret)

			// Check if KOPIA_SOURCE_PATH_OVERRIDE is present
			found := false
			var actualValue string
			for _, env := range envVars {
				if env.Name == "KOPIA_SOURCE_PATH_OVERRIDE" {
					found = true
					actualValue = env.Value
					break
				}
			}

			if tt.expectEnvVar && !found {
				t.Errorf("Expected KOPIA_SOURCE_PATH_OVERRIDE env var, but not found")
			}
			if !tt.expectEnvVar && found {
				t.Errorf("Did not expect KOPIA_SOURCE_PATH_OVERRIDE env var, but found with value %q", actualValue)
			}
			if tt.expectEnvVar && found && actualValue != tt.expectedValue {
				t.Errorf("Expected KOPIA_SOURCE_PATH_OVERRIDE value %q, got %q", tt.expectedValue, actualValue)
			}
		})
	}
}

// TestDiscoverSourcePVCCompatibility tests the backward compatibility wrapper
func TestDiscoverSourcePVCCompatibility(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = volsyncv1alpha1.AddToScheme(scheme)

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

	// Call the deprecated function
	pvcName := kb.discoverSourcePVC(fakeClient, "test-source", "test-ns", logger)

	// Should still return the PVC name
	if pvcName != "source-pvc" {
		t.Errorf("Expected PVC name 'source-pvc', got %q", pvcName)
	}
}
