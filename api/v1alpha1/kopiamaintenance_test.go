/*
Copyright 2025 The VolSync authors.

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

package v1alpha1

import (
	"testing"

	"k8s.io/utils/ptr"
)


func TestKopiaMaintenance_GetRepositorySecret(t *testing.T) {
	tests := []struct {
		name           string
		spec           KopiaMaintenanceSpec
		wantSecretName string
	}{
		{
			name: "repository with secret",
			spec: KopiaMaintenanceSpec{
				Repository: KopiaRepositorySpec{
					Repository: "test-secret",
				},
			},
			wantSecretName: "test-secret",
		},
		{
			name: "repository with different secret",
			spec: KopiaMaintenanceSpec{
				Repository: KopiaRepositorySpec{
					Repository: "another-secret",
				},
			},
			wantSecretName: "another-secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			km := &KopiaMaintenance{
				Spec: tt.spec,
			}
			gotSecretName := km.GetRepositorySecret()
			if gotSecretName != tt.wantSecretName {
				t.Errorf("KopiaMaintenance.GetRepositorySecret() = %v, want %v", gotSecretName, tt.wantSecretName)
			}
		})
	}
}

func TestKopiaMaintenance_Validate(t *testing.T) {
	tests := []struct {
		name    string
		spec    KopiaMaintenanceSpec
		wantErr bool
	}{
		{
			name: "valid repository",
			spec: KopiaMaintenanceSpec{
				Repository: KopiaRepositorySpec{
					Repository: "test-secret",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid - empty repository name",
			spec: KopiaMaintenanceSpec{
				Repository: KopiaRepositorySpec{
					Repository: "",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			km := &KopiaMaintenance{
				Spec: tt.spec,
			}
			err := km.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("KopiaMaintenance.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}


func TestKopiaMaintenance_GetEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
		want    bool
	}{
		{
			name:    "default enabled",
			enabled: nil,
			want:    true,
		},
		{
			name:    "explicitly enabled",
			enabled: ptr.To(true),
			want:    true,
		},
		{
			name:    "explicitly disabled",
			enabled: ptr.To(false),
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			km := &KopiaMaintenance{
				Spec: KopiaMaintenanceSpec{
					Enabled: tt.enabled,
				},
			}
			if got := km.GetEnabled(); got != tt.want {
				t.Errorf("KopiaMaintenance.GetEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKopiaMaintenance_GetSchedule(t *testing.T) {
	tests := []struct {
		name     string
		schedule string
		want     string
	}{
		{
			name:     "default schedule",
			schedule: "",
			want:     "0 2 * * *",
		},
		{
			name:     "custom schedule",
			schedule: "0 4 * * 0",
			want:     "0 4 * * 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			km := &KopiaMaintenance{
				Spec: KopiaMaintenanceSpec{
					Schedule: tt.schedule,
				},
			}
			if got := km.GetSchedule(); got != tt.want {
				t.Errorf("KopiaMaintenance.GetSchedule() = %v, want %v", got, tt.want)
			}
		})
	}
}