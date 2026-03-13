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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Source Identity Generation", func() {
	type sourceIdentityTestCase struct {
		name             string
		destination      *volsyncv1alpha1.ReplicationDestination
		expectedUsername string
		expectedHostname string
	}

	DescribeTable("generates correct username and hostname",
		func(tc sourceIdentityTestCase) {
			// Simulate what the builder does
			var username, hostname string

			// First check for explicit username/hostname (highest priority)
			if tc.destination.Spec.Kopia.Username != nil && *tc.destination.Spec.Kopia.Username != "" {
				username = *tc.destination.Spec.Kopia.Username
			}
			if tc.destination.Spec.Kopia.Hostname != nil && *tc.destination.Spec.Kopia.Hostname != "" {
				hostname = *tc.destination.Spec.Kopia.Hostname
			}

			// If not explicitly set, check sourceIdentity helper
			if username == "" && tc.destination.Spec.Kopia.SourceIdentity != nil &&
				tc.destination.Spec.Kopia.SourceIdentity.SourceName != "" &&
				tc.destination.Spec.Kopia.SourceIdentity.SourceNamespace != "" {
				si := tc.destination.Spec.Kopia.SourceIdentity
				username = generateUsername(nil, si.SourceName, si.SourceNamespace)
			}
			if hostname == "" && tc.destination.Spec.Kopia.SourceIdentity != nil &&
				tc.destination.Spec.Kopia.SourceIdentity.SourceName != "" &&
				tc.destination.Spec.Kopia.SourceIdentity.SourceNamespace != "" {
				si := tc.destination.Spec.Kopia.SourceIdentity
				var pvcNameToUse *string
				if si.SourcePVCName != "" {
					pvcNameToUse = &si.SourcePVCName
				} else {
					pvcNameToUse = tc.destination.Spec.Kopia.DestinationPVC
				}
				hostname = generateHostname(nil, pvcNameToUse,
					si.SourceNamespace, si.SourceName)
			}

			// Finally, fall back to default generation
			if username == "" {
				username = generateUsername(nil,
					tc.destination.GetName(), tc.destination.GetNamespace())
			}
			if hostname == "" {
				hostname = generateHostname(nil,
					tc.destination.Spec.Kopia.DestinationPVC,
					tc.destination.GetNamespace(), tc.destination.GetName())
			}

			Expect(username).To(Equal(tc.expectedUsername), "Username mismatch")
			Expect(hostname).To(Equal(tc.expectedHostname), "Hostname mismatch")
		},
		Entry("sourceIdentity generates correct username and hostname", sourceIdentityTestCase{
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
			expectedUsername: "webapp-backup", // Username is object name only (simplified)
			expectedHostname: "production",    // Hostname is namespace only
		}),
		Entry("explicit username/hostname override sourceIdentity", sourceIdentityTestCase{
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
		}),
		Entry("no sourceIdentity uses default generation", sourceIdentityTestCase{
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
			expectedUsername: "restore-app", // Username is object name only (simplified)
			expectedHostname: "restore-ns",  // Hostname is namespace only
		}),
		Entry("sourceIdentity with destination PVC name in hostname (no source PVC specified)", sourceIdentityTestCase{
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
			expectedUsername: "webapp", // Username is object name only (simplified)
			expectedHostname: "prod",   // Hostname is namespace only
		}),
		Entry("sourceIdentity with source PVC name generates matching hostname", sourceIdentityTestCase{
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
			expectedUsername: "webapp", // Username is object name only (simplified)
			expectedHostname: "prod",   // Hostname is namespace only (PVC ignored)
		}),
		Entry("partial sourceIdentity falls back to defaults", sourceIdentityTestCase{
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
			expectedUsername: "restore-app", // Username is object name only (simplified)
			expectedHostname: "restore-ns",  // Hostname is namespace only
		}),
	)
})
