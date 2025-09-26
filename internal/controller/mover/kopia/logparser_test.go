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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

var _ = Describe("Kopia Log Parser", func() {
	Describe("ParseKopiaDiscoveryOutput", func() {
		Context("when no snapshots are found", func() {
			It("should parse error message with requested identity", func() {
				logs := `
=== Starting restore ===
Discovery mode enabled - will list available snapshots if restore fails
Selecting snapshot to restore
Using previous offset: 0 (0=latest, 1=previous, etc.)
No eligible snapshots found
No snapshots found for user1@host1:/data

=== Discovery Mode: Available Snapshots ===
Listing all available snapshot identities in the repository...
Found snapshots in repository:

{"id":"abc123","userName":"user2","hostName":"host2","path":"/data",` +
					`"startTime":"2024-01-01T10:00:00Z","endTime":"2024-01-01T10:05:00Z"}
{"id":"def456","userName":"user3","hostName":"host3","path":"/data",` +
					`"startTime":"2024-01-01T11:00:00Z","endTime":"2024-01-01T11:05:00Z"}

Available identities (username@hostname combinations):
user2@host2:/data - Last snapshot: 2024-01-01T10:05:00Z
user3@host3:/data - Last snapshot: 2024-01-01T11:05:00Z
=== End Discovery Mode ===
`
				requestedIdentity, availableIdentities, errorMsg := ParseKopiaDiscoveryOutput(logs)

				Expect(requestedIdentity).To(Equal("user1@host1"))
				Expect(availableIdentities).To(HaveLen(2))
				Expect(availableIdentities[0].Identity).To(Equal("user2@host2"))
				Expect(availableIdentities[0].SnapshotCount).To(Equal(int32(1)))
				Expect(availableIdentities[1].Identity).To(Equal("user3@host3"))
				Expect(errorMsg).To(ContainSubstring("No snapshots found for identity 'user1@host1'"))
				Expect(errorMsg).To(ContainSubstring("Available identities: user2@host2, user3@host3"))
			})
		})

		Context("when parsing traditional snapshot listing", func() {
			It("should extract identities from listing format", func() {
				logs := `
user1@host1:/data 2024-01-01 10:00:00
user1@host1:/data 2024-01-02 10:00:00
user2@host2:/backup 2024-01-01 11:00:00
`
				_, availableIdentities, _ := ParseKopiaDiscoveryOutput(logs)

				Expect(availableIdentities).To(HaveLen(2))
				Expect(availableIdentities[0].Identity).To(Equal("user1@host1"))
				Expect(availableIdentities[0].SnapshotCount).To(Equal(int32(2)))
				Expect(availableIdentities[1].Identity).To(Equal("user2@host2"))
				Expect(availableIdentities[1].SnapshotCount).To(Equal(int32(1)))
			})
		})

		Context("when parsing error messages", func() {
			It("should extract identity from error message", func() {
				logs := `unable to find snapshots for source "user1@host1:/data"`

				requestedIdentity, _, errorMsg := ParseKopiaDiscoveryOutput(logs)

				Expect(requestedIdentity).To(Equal("user1@host1"))
				Expect(errorMsg).To(Equal("No snapshots found for identity 'user1@host1'"))
			})
		})
	})

	Describe("LogFilter", func() {
		It("should include error messages", func() {
			line := "ERROR: Failed to connect to repository"
			result := LogFilter(line)
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(line))
		})

		It("should include discovery mode headers", func() {
			line := "=== Discovery Mode: Available Snapshots ==="
			result := LogFilter(line)
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(line))
		})

		It("should include snapshot listings", func() {
			line := "user1@host1:/data 2024-01-01 10:00:00"
			result := LogFilter(line)
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(line))
		})

		It("should filter out unimportant lines", func() {
			line := "Some random debug output"
			result := LogFilter(line)
			Expect(result).To(BeNil())
		})

		It("should include JSON snapshot data", func() {
			line := `{"id":"abc123","userName":"user1","hostName":"host1","path":"/data"}`
			result := LogFilter(line)
			Expect(result).NotTo(BeNil())
			Expect(*result).To(Equal(line))
		})
	})
})

// Test function is in builder_test.go to avoid duplicate RunSpecs

// Additional unit tests using standard testing package
func TestProcessJSONSnapshots(t *testing.T) {
	tests := []struct {
		name               string
		jsonLines          []string
		expectedIdentities map[string]int32
	}{
		{
			name: "multiple snapshots same identity",
			jsonLines: []string{
				`{"id":"snap1","userName":"user1","hostName":"host1","path":"/data","endTime":"2024-01-01T10:00:00Z"}`,
				`{"id":"snap2","userName":"user1","hostName":"host1","path":"/data","endTime":"2024-01-02T10:00:00Z"}`,
			},
			expectedIdentities: map[string]int32{
				"user1@host1": 2,
			},
		},
		{
			name: "different identities",
			jsonLines: []string{
				`{"id":"snap1","userName":"user1","hostName":"host1","path":"/data","endTime":"2024-01-01T10:00:00Z"}`,
				`{"id":"snap2","userName":"user2","hostName":"host2","path":"/data","endTime":"2024-01-02T10:00:00Z"}`,
			},
			expectedIdentities: map[string]int32{
				"user1@host1": 1,
				"user2@host2": 1,
			},
		},
		{
			name: "invalid JSON ignored",
			jsonLines: []string{
				`{"id":"snap1","userName":"user1","hostName":"host1","path":"/data","endTime":"2024-01-01T10:00:00Z"}`,
				`invalid json`,
				`{"id":"snap2","userName":"user1","hostName":"host1","path":"/data","endTime":"2024-01-02T10:00:00Z"}`,
			},
			expectedIdentities: map[string]int32{
				"user1@host1": 2,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identityMap := make(map[string]*volsyncv1alpha1.KopiaIdentityInfo)
			processJSONSnapshots(tt.jsonLines, identityMap)

			if len(identityMap) != len(tt.expectedIdentities) {
				t.Errorf("expected %d identities, got %d", len(tt.expectedIdentities), len(identityMap))
			}

			for identity, expectedCount := range tt.expectedIdentities {
				if info, exists := identityMap[identity]; !exists {
					t.Errorf("expected identity %s not found", identity)
				} else if info.SnapshotCount != expectedCount {
					t.Errorf("identity %s: expected %d snapshots, got %d", identity, expectedCount, info.SnapshotCount)
				}
			}
		})
	}
}

func TestLatestSnapshotTime(t *testing.T) {
	identityMap := make(map[string]*volsyncv1alpha1.KopiaIdentityInfo)

	// Process snapshots with different times
	jsonLines := []string{
		`{"id":"snap1","userName":"user1","hostName":"host1","path":"/data","endTime":"2024-01-01T10:00:00Z"}`,
		`{"id":"snap2","userName":"user1","hostName":"host1","path":"/data","endTime":"2024-01-03T10:00:00Z"}`,
		`{"id":"snap3","userName":"user1","hostName":"host1","path":"/data","endTime":"2024-01-02T10:00:00Z"}`,
	}

	processJSONSnapshots(jsonLines, identityMap)

	info := identityMap["user1@host1"]
	if info == nil {
		t.Fatal("expected identity not found")
	}

	if info.LatestSnapshot == nil {
		t.Fatal("expected latest snapshot time to be set")
	}

	expectedTime, _ := time.Parse(time.RFC3339, "2024-01-03T10:00:00Z")
	if !info.LatestSnapshot.Time.Equal(expectedTime) {
		t.Errorf("expected latest time %v, got %v", expectedTime, info.LatestSnapshot.Time)
	}
}
