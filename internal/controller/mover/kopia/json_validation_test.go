package kopia

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"k8s.io/utils/ptr"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

func TestJSONValidation(t *testing.T) {
	tests := []struct {
		name             string
		repositoryConfig *string
		wantError        bool
	}{
		{
			name:      "nil policy config",
			wantError: false,
		},
		{
			name:             "empty repository config",
			repositoryConfig: ptr.To(""),
			wantError:        false,
		},
		{
			name:             "valid JSON",
			repositoryConfig: ptr.To(`{"compression": "zstd"}`),
			wantError:        false,
		},
		{
			name:             "invalid JSON syntax",
			repositoryConfig: ptr.To(`{invalid json}`),
			wantError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mover := &Mover{
				logger: logr.Discard(),
			}

			if tt.repositoryConfig != nil {
				mover.policyConfig = &volsyncv1alpha1.KopiaPolicySpec{
					RepositoryConfig: tt.repositoryConfig,
				}
			}

			_, err := mover.validatePolicyConfig(context.Background())

			if tt.wantError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "invalid JSON")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
