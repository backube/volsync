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

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

//nolint:funlen
func TestSourceIdentityGeneration(t *testing.T) {
	tests := []struct {
		name             string
		destination      *volsyncv1alpha1.ReplicationDestination
		expectedUsername string
		expectedHostname string
	}{
		{
			name: "sourceIdentity generates correct username and hostname",
			destination: &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "restore-app",
					Namespace: "restore-ns",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
						Repository: "repo-secret",
						SourceIdentity: &volsyncv1alpha1.KopiaSourceIdentity{
							SourceName:      "webapp-backup",
							SourceNamespace: "production",
						},
					},
				},
			},
			expectedUsername: "webapp-backup-production",
			expectedHostname: "production-webapp-backup",
		},
		{
			name: "explicit username/hostname override sourceIdentity",
			destination: &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "restore-app",
					Namespace: "restore-ns",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
						Repository: "repo-secret",
						Username:   ptr.To("custom-user"),
						Hostname:   ptr.To("custom-host"),
						SourceIdentity: &volsyncv1alpha1.KopiaSourceIdentity{
							SourceName:      "webapp-backup",
							SourceNamespace: "production",
						},
					},
				},
			},
			expectedUsername: "custom-user",
			expectedHostname: "custom-host",
		},
		{
			name: "no sourceIdentity uses default generation",
			destination: &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "restore-app",
					Namespace: "restore-ns",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
						Repository: "repo-secret",
					},
				},
			},
			expectedUsername: "restore-app-restore-ns",
			expectedHostname: "restore-ns-restore-app",
		},
		{
			name: "sourceIdentity with destination PVC name in hostname (no source PVC specified)",
			destination: &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "restore-app",
					Namespace: "restore-ns",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
						Repository: "repo-secret",
						ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
							DestinationPVC: ptr.To("data-pvc"),
						},
						SourceIdentity: &volsyncv1alpha1.KopiaSourceIdentity{
							SourceName:      "webapp",
							SourceNamespace: "prod",
						},
					},
				},
			},
			expectedUsername: "webapp-prod",
			expectedHostname: "prod-webapp",
		},
		{
			name: "sourceIdentity with source PVC name generates matching hostname",
			destination: &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "restore-app",
					Namespace: "restore-ns",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
						Repository: "repo-secret",
						ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
							DestinationPVC: ptr.To("restore-data-pvc"), // Different from source
						},
						SourceIdentity: &volsyncv1alpha1.KopiaSourceIdentity{
							SourceName:      "webapp",
							SourceNamespace: "prod",
							SourcePVCName:   "original-data-pvc", // Source PVC name
						},
					},
				},
			},
			expectedUsername: "webapp-prod",
			expectedHostname: "prod-webapp", // Includes object name for uniqueness
		},
		{
			name: "partial sourceIdentity falls back to defaults",
			destination: &volsyncv1alpha1.ReplicationDestination{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "restore-app",
					Namespace: "restore-ns",
				},
				Spec: volsyncv1alpha1.ReplicationDestinationSpec{
					Kopia: &volsyncv1alpha1.ReplicationDestinationKopiaSpec{
						Repository: "repo-secret",
						SourceIdentity: &volsyncv1alpha1.KopiaSourceIdentity{
							SourceName: "webapp", // Missing SourceNamespace
						},
					},
				},
			},
			expectedUsername: "restore-app-restore-ns",
			expectedHostname: "restore-ns-restore-app",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate what the builder does
			var username, hostname string

			// First check for explicit username/hostname (highest priority)
			if tt.destination.Spec.Kopia.Username != nil && *tt.destination.Spec.Kopia.Username != "" {
				username = *tt.destination.Spec.Kopia.Username
			}
			if tt.destination.Spec.Kopia.Hostname != nil && *tt.destination.Spec.Kopia.Hostname != "" {
				hostname = *tt.destination.Spec.Kopia.Hostname
			}

			// If not explicitly set, check sourceIdentity helper
			if username == "" && tt.destination.Spec.Kopia.SourceIdentity != nil &&
				tt.destination.Spec.Kopia.SourceIdentity.SourceName != "" &&
				tt.destination.Spec.Kopia.SourceIdentity.SourceNamespace != "" {
				si := tt.destination.Spec.Kopia.SourceIdentity
				username = generateUsername(nil, si.SourceName, si.SourceNamespace)
			}
			if hostname == "" && tt.destination.Spec.Kopia.SourceIdentity != nil &&
				tt.destination.Spec.Kopia.SourceIdentity.SourceName != "" &&
				tt.destination.Spec.Kopia.SourceIdentity.SourceNamespace != "" {
				si := tt.destination.Spec.Kopia.SourceIdentity
				var pvcNameToUse *string
				if si.SourcePVCName != "" {
					pvcNameToUse = &si.SourcePVCName
				} else {
					pvcNameToUse = tt.destination.Spec.Kopia.DestinationPVC
				}
				hostname = generateHostname(nil, pvcNameToUse,
					si.SourceNamespace, si.SourceName)
			}

			// Finally, fall back to default generation
			if username == "" {
				username = generateUsername(nil,
					tt.destination.GetName(), tt.destination.GetNamespace())
			}
			if hostname == "" {
				hostname = generateHostname(nil,
					tt.destination.Spec.Kopia.DestinationPVC,
					tt.destination.GetNamespace(), tt.destination.GetName())
			}

			assert.Equal(t, tt.expectedUsername, username, "Username mismatch")
			assert.Equal(t, tt.expectedHostname, hostname, "Hostname mismatch")
		})
	}
}
