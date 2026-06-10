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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Username Validation", func() {
	Describe("generateUsername", func() {
		Context("custom username cases", func() {
			It("should return custom username when provided", func() {
				customUser := "custom-user"
				result := generateUsername(&customUser, "any-object-name", "")
				Expect(result).To(Equal("custom-user"))
			})

			It("should return custom username as-is without sanitization", func() {
				customUser := "user@domain.com"
				result := generateUsername(&customUser, "any-object", "")
				Expect(result).To(Equal("user@domain.com"))
			})

			It("should use object name when custom username is empty", func() {
				emptyUser := ""
				result := generateUsername(&emptyUser, "test-object", "")
				Expect(result).To(Equal("test-object"))
			})
		})

		Context("valid name cases", func() {
			It("should preserve valid alphanumeric characters and hyphens", func() {
				result := generateUsername(nil, "app-backup-123", "")
				Expect(result).To(Equal("app-backup-123"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})

			It("should preserve underscores in object names", func() {
				result := generateUsername(nil, "app_backup_job", "")
				Expect(result).To(Equal("app_backup_job"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})

			It("should preserve mixed case, hyphens, and underscores", func() {
				result := generateUsername(nil, "App-Backup_123", "")
				Expect(result).To(Equal("App-Backup_123"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})

			It("should handle typical Kubernetes object names", func() {
				result := generateUsername(nil, "my-app-deployment-7d4f8b9c6d", "")
				Expect(result).To(Equal("my-app-deployment-7d4f8b9c6d"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})

			It("should preserve numeric-only names", func() {
				result := generateUsername(nil, "12345", "")
				Expect(result).To(Equal("12345"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})

			It("should preserve names starting and ending with numbers", func() {
				result := generateUsername(nil, "1app-backup9", "")
				Expect(result).To(Equal("1app-backup9"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})
		})

		Context("sanitization cases", func() {
			It("should remove special characters like @, ., etc.", func() {
				result := generateUsername(nil, "app@backup.service", "")
				Expect(result).To(Equal("appbackupservice"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})

			It("should trim leading and trailing hyphens", func() {
				result := generateUsername(nil, "-app-backup-", "")
				Expect(result).To(Equal("app-backup"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})

			It("should trim leading and trailing underscores", func() {
				result := generateUsername(nil, "_app_backup_", "")
				Expect(result).To(Equal("app_backup"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})

			It("should trim all leading and trailing hyphens and underscores", func() {
				result := generateUsername(nil, "-_app-backup_-", "")
				Expect(result).To(Equal("app-backup"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})

			It("should remove spaces", func() {
				result := generateUsername(nil, "app backup service", "")
				Expect(result).To(Equal("appbackupservice"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})

			It("should remove unicode characters", func() {
				result := generateUsername(nil, "app-backup-ñ-test", "")
				Expect(result).To(Equal("app-backup--test"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})

			It("should remove dots which are not allowed in usernames", func() {
				result := generateUsername(nil, "app.backup.service", "")
				Expect(result).To(Equal("appbackupservice"))
				validateUsernameCharactersGinkgo(nil, result)
				validateUsernameTrimmingGinkgo(nil, result)
			})
		})

		Context("edge cases", func() {
			It("should return fallback when all chars are removed", func() {
				result := generateUsername(nil, "@#$%^&*()", "")
				Expect(result).To(Equal(defaultUsername))
			})

			It("should return fallback when only separators remain", func() {
				result := generateUsername(nil, "-_-_-", "")
				Expect(result).To(Equal(defaultUsername))
			})

			It("should return fallback for empty object name", func() {
				result := generateUsername(nil, "", "")
				Expect(result).To(Equal(defaultUsername))
			})

			It("should truncate very long names to maxUsernameLength", func() {
				veryLongName := strings.Repeat("a", 100) + "-backup"
				result := generateUsername(nil, veryLongName, "")
				Expect(result).To(Equal(strings.Repeat("a", maxUsernameLength)))
			})
		})

		Context("additional edge cases", func() {
			It("should truncate very long names to max length", func() {
				veryLongName := strings.Repeat("a-", 1000) + "backup"
				result := generateUsername(nil, veryLongName, "")
				Expect(len(result)).To(BeNumerically("<=", maxUsernameLength))
			})

			It("should handle only invalid characters followed by valid ones", func() {
				result := generateUsername(nil, "!!!abc123", "")
				Expect(result).To(Equal("abc123"))
			})

			It("should handle valid chars followed by invalid ones", func() {
				result := generateUsername(nil, "abc123!!!", "")
				Expect(result).To(Equal("abc123"))
			})

			It("should handle alternating valid/invalid characters", func() {
				result := generateUsername(nil, "a!b@c#d$e%f^g", "")
				Expect(result).To(Equal("abcdefg"))
			})
		})

		Context("behavior preservation for backward compatibility", func() {
			It("should preserve app-backup", func() {
				result := generateUsername(nil, "app-backup", "")
				Expect(result).To(Equal("app-backup"))
			})

			It("should preserve database_backup", func() {
				result := generateUsername(nil, "database_backup", "")
				Expect(result).To(Equal("database_backup"))
			})

			It("should preserve service123", func() {
				result := generateUsername(nil, "service123", "")
				Expect(result).To(Equal("service123"))
			})

			It("should preserve MyApp-Backup_123", func() {
				result := generateUsername(nil, "MyApp-Backup_123", "")
				Expect(result).To(Equal("MyApp-Backup_123"))
			})

			It("should preserve simple", func() {
				result := generateUsername(nil, "simple", "")
				Expect(result).To(Equal("simple"))
			})

			It("should preserve backup-service-v1", func() {
				result := generateUsername(nil, "backup-service-v1", "")
				Expect(result).To(Equal("backup-service-v1"))
			})
		})
	})
})

// validateUsernameCharactersGinkgo validates that generated usernames contain only valid characters
func validateUsernameCharactersGinkgo(username *string, result string) {
	if username == nil && result != defaultUsername {
		for _, r := range result {
			validChar := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
			Expect(validChar).To(BeTrue(), "generateUsername() returned invalid character %c in result %q", r, result)
		}
	}
}

// validateUsernameTrimmingGinkgo validates that generated usernames don't have leading/trailing separators
func validateUsernameTrimmingGinkgo(username *string, result string) {
	if username == nil && result != defaultUsername {
		hasBadPrefix := strings.HasPrefix(result, "-") || strings.HasPrefix(result, "_")
		hasBadSuffix := strings.HasSuffix(result, "-") || strings.HasSuffix(result, "_")
		Expect(hasBadPrefix || hasBadSuffix).To(BeFalse(),
			"generateUsername() returned result %q with leading/trailing separators", result)
	}
}
