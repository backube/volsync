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
)

func TestAddAdditionalArgsEnvVar(t *testing.T) {
	tests := []struct {
		name           string
		additionalArgs []string
		expectedEnvVar string
		expectNoEnvVar bool
	}{
		{
			name: "single argument",
			additionalArgs: []string{
				"--one-file-system",
			},
			expectedEnvVar: "--one-file-system",
		},
		{
			name: "multiple arguments",
			additionalArgs: []string{
				"--one-file-system",
				"--parallel=8",
				"--ignore-cache-dirs",
			},
			expectedEnvVar: "--one-file-system|VOLSYNC_ARG_SEP|--parallel=8|VOLSYNC_ARG_SEP|--ignore-cache-dirs",
		},
		{
			name: "arguments with equals",
			additionalArgs: []string{
				"--compression=zstd",
				"--upload-speed=100MB",
			},
			expectedEnvVar: "--compression=zstd|VOLSYNC_ARG_SEP|--upload-speed=100MB",
		},
		{
			name: "arguments with special characters",
			additionalArgs: []string{
				"--exclude=*.tmp",
				"--exclude=cache/",
			},
			expectedEnvVar: "--exclude=*.tmp|VOLSYNC_ARG_SEP|--exclude=cache/",
		},
		{
			name: "arguments that would have been blocked before",
			additionalArgs: []string{
				"--password=secret",
				"--config-file=/etc/kopia",
				"--username=myuser",
			},
			expectedEnvVar: "--password=secret|VOLSYNC_ARG_SEP|--config-file=/etc/kopia|VOLSYNC_ARG_SEP|--username=myuser",
		},
		{
			name:           "empty args",
			additionalArgs: []string{},
			expectNoEnvVar: true,
		},
		{
			name:           "nil args",
			additionalArgs: nil,
			expectNoEnvVar: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Mover{
				logger:         logr.Discard(),
				additionalArgs: tt.additionalArgs,
			}

			envVars := []corev1.EnvVar{}
			result := m.addAdditionalArgsEnvVar(envVars)

			if tt.expectNoEnvVar {
				// Check that no KOPIA_ADDITIONAL_ARGS was added
				for _, env := range result {
					if env.Name == "KOPIA_ADDITIONAL_ARGS" {
						t.Errorf("expected no KOPIA_ADDITIONAL_ARGS env var, but found: %s", env.Value)
					}
				}
			} else {
				// Check that KOPIA_ADDITIONAL_ARGS was added with correct value
				found := false
				for _, env := range result {
					if env.Name == "KOPIA_ADDITIONAL_ARGS" {
						found = true
						if env.Value != tt.expectedEnvVar {
							t.Errorf("expected env var value '%s', got '%s'", tt.expectedEnvVar, env.Value)
						}
					}
				}
				if !found {
					t.Errorf("expected KOPIA_ADDITIONAL_ARGS env var, but not found")
				}
			}
		})
	}
}

func TestAdditionalArgsIntegration(t *testing.T) {
	// Test that additional args are properly passed through the builder
	t.Run("ReplicationSource includes additional args", func(t *testing.T) {
		m := &Mover{
			logger: logr.Discard(),
			additionalArgs: []string{
				"--one-file-system",
				"--parallel=8",
			},
		}

		envVars := m.addAdditionalArgsEnvVar([]corev1.EnvVar{})
		
		if len(envVars) != 1 {
			t.Errorf("expected 1 env var, got %d", len(envVars))
		}
		
		if envVars[0].Name != "KOPIA_ADDITIONAL_ARGS" {
			t.Errorf("expected env var name KOPIA_ADDITIONAL_ARGS, got %s", envVars[0].Name)
		}
		
		expected := "--one-file-system|VOLSYNC_ARG_SEP|--parallel=8"
		if envVars[0].Value != expected {
			t.Errorf("expected env var value '%s', got '%s'", expected, envVars[0].Value)
		}
	})

	t.Run("No validation - all args are allowed", func(t *testing.T) {
		m := &Mover{
			logger: logr.Discard(),
			additionalArgs: []string{
				"--password=secret",
				"--config-file=/custom/config",
				"--repository=s3://bucket",
			},
		}

		envVars := m.addAdditionalArgsEnvVar([]corev1.EnvVar{})
		
		// Should include all args without validation
		if len(envVars) != 1 {
			t.Errorf("expected 1 env var, got %d", len(envVars))
		}
		
		expected := "--password=secret|VOLSYNC_ARG_SEP|--config-file=/custom/config|VOLSYNC_ARG_SEP|--repository=s3://bucket"
		if envVars[0].Value != expected {
			t.Errorf("expected env var value '%s', got '%s'", expected, envVars[0].Value)
		}
	})
}