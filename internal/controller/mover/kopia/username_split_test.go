//go:build !disable_kopia

package kopia

import (
	"strings"

	"k8s.io/utils/ptr"
)

// Split helper functions for username test cases

type usernameTestCase struct {
	name        string
	username    *string
	objectName  string
	expected    string
	description string
}

// getGenerateUsernameTestCases returns all test cases split into smaller functions
func getGenerateUsernameTestCases() []usernameTestCase {
	var cases []usernameTestCase
	cases = append(cases, getCustomUsernameCases2()...)
	cases = append(cases, getValidNameCases()...)
	cases = append(cases, getSanitizationCases()...)
	cases = append(cases, getEdgeCases()...)
	return cases
}

func getCustomUsernameCases2() []usernameTestCase {
	return []usernameTestCase{
		{
			name:        "custom username provided",
			username:    ptr.To("custom-user"),
			objectName:  "any-object-name",
			expected:    "custom-user",
			description: "Should return custom username when provided",
		},
		{
			name:        "custom username with special chars",
			username:    ptr.To("user@domain.com"),
			objectName:  "any-object",
			expected:    "user@domain.com",
			description: "Should return custom username as-is without sanitization",
		},
		{
			name:        "empty custom username falls back to object name",
			username:    ptr.To(""),
			objectName:  "test-object",
			expected:    "test-object",
			description: "Should use object name when custom username is empty",
		},
	}
}

func getValidNameCases() []usernameTestCase {
	return []usernameTestCase{
		{
			name:        "valid object name with alphanumeric",
			username:    nil,
			objectName:  "app-backup-123",
			expected:    "app-backup-123",
			description: "Should preserve valid alphanumeric characters and hyphens",
		},
		{
			name:        "valid object name with underscores",
			username:    nil,
			objectName:  "app_backup_job",
			expected:    "app_backup_job",
			description: "Should preserve underscores in object names",
		},
		{
			name:        "object name with mixed valid chars",
			username:    nil,
			objectName:  "App-Backup_123",
			expected:    "App-Backup_123",
			description: "Should preserve mixed case, hyphens, and underscores",
		},
		{
			name:        "object name with kubernetes-style name",
			username:    nil,
			objectName:  "my-app-deployment-7d4f8b9c6d",
			expected:    "my-app-deployment-7d4f8b9c6d",
			description: "Should handle typical Kubernetes object names",
		},
		{
			name:        "object name with numbers only",
			username:    nil,
			objectName:  "12345",
			expected:    "12345",
			description: "Should preserve numeric-only names",
		},
		{
			name:        "object name starting and ending with numbers",
			username:    nil,
			objectName:  "1app-backup9",
			expected:    "1app-backup9",
			description: "Should preserve names starting and ending with numbers",
		},
	}
}

func getSanitizationCases() []usernameTestCase {
	return []usernameTestCase{
		{
			name:        "object name with special characters",
			username:    nil,
			objectName:  "app@backup.service",
			expected:    "appbackupservice",
			description: "Should remove special characters like @, ., etc.",
		},
		{
			name:        "object name with leading/trailing hyphens",
			username:    nil,
			objectName:  "-app-backup-",
			expected:    "app-backup",
			description: "Should trim leading and trailing hyphens",
		},
		{
			name:        "object name with leading/trailing underscores",
			username:    nil,
			objectName:  "_app_backup_",
			expected:    "app_backup",
			description: "Should trim leading and trailing underscores",
		},
		{
			name:        "object name with mixed leading/trailing chars",
			username:    nil,
			objectName:  "-_app-backup_-",
			expected:    "app-backup",
			description: "Should trim all leading and trailing hyphens and underscores",
		},
		{
			name:        "object name with spaces",
			username:    nil,
			objectName:  "app backup service",
			expected:    "appbackupservice",
			description: "Should remove spaces",
		},
		{
			name:        "object name with unicode characters",
			username:    nil,
			objectName:  "app-backup-ñ-test",
			expected:    "app-backup--test",
			description: "Should remove unicode characters",
		},
		{
			name:        "object name with dots",
			username:    nil,
			objectName:  "app.backup.service",
			expected:    "appbackupservice",
			description: "Should remove dots which are not allowed in usernames",
		},
	}
}

func getEdgeCases() []usernameTestCase {
	return []usernameTestCase{
		{
			name:        "object name with only special characters",
			username:    nil,
			objectName:  "@#$%^&*()",
			expected:    defaultUsername,
			description: "Should return fallback when all chars are removed",
		},
		{
			name:        "object name with only hyphens and underscores",
			username:    nil,
			objectName:  "-_-_-",
			expected:    defaultUsername,
			description: "Should return fallback when only separators remain",
		},
		{
			name:        "empty object name",
			username:    nil,
			objectName:  "",
			expected:    defaultUsername,
			description: "Should return fallback for empty object name",
		},
		{
			name:        "very long object name",
			username:    nil,
			objectName:  strings.Repeat("a", 100) + "-backup",
			expected:    strings.Repeat("a", 100) + "-backup",
			description: "Should handle very long valid names",
		},
	}
}
