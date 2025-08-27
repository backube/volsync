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
	"encoding/json"
	"fmt"
	"sort"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// KopiaSnapshot represents the structure of a Kopia snapshot as returned by 'kopia snapshot list --json'
type KopiaSnapshot struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	StartTime   time.Time `json:"startTime"`
	EndTime     time.Time `json:"endTime"`
	RootEntry   struct {
		Size int64 `json:"size"`
	} `json:"rootEntry"`
}

// simulateKopiaSnapshotList simulates the output of 'kopia snapshot list --json'
// It returns snapshots in the same order that Kopia does (chronological/oldest first)
func simulateKopiaSnapshotList() []KopiaSnapshot {
	now := time.Now()
	snapshots := []KopiaSnapshot{
		{
			ID:          "snapshot-1-oldest",
			Description: "First snapshot (oldest)",
			StartTime:   now.Add(-72 * time.Hour), // 3 days ago
			EndTime:     now.Add(-72 * time.Hour).Add(5 * time.Minute),
		},
		{
			ID:          "snapshot-2-week-old",
			Description: "Second snapshot (week old)",
			StartTime:   now.Add(-168 * time.Hour), // 7 days ago
			EndTime:     now.Add(-168 * time.Hour).Add(5 * time.Minute),
		},
		{
			ID:          "snapshot-3-yesterday",
			Description: "Third snapshot (yesterday)",
			StartTime:   now.Add(-24 * time.Hour), // 1 day ago
			EndTime:     now.Add(-24 * time.Hour).Add(5 * time.Minute),
		},
		{
			ID:          "snapshot-4-hour-ago",
			Description: "Fourth snapshot (1 hour ago)",
			StartTime:   now.Add(-1 * time.Hour), // 1 hour ago
			EndTime:     now.Add(-1 * time.Hour).Add(5 * time.Minute),
		},
		{
			ID:          "snapshot-5-latest",
			Description: "Fifth snapshot (latest/newest)",
			StartTime:   now.Add(-10 * time.Minute), // 10 minutes ago
			EndTime:     now.Add(-5 * time.Minute),
		},
	}

	// Sort by StartTime to simulate Kopia's chronological ordering (oldest first)
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].StartTime.Before(snapshots[j].StartTime)
	})

	return snapshots
}

// selectSnapshotWithoutReverse simulates the CURRENT (buggy) behavior in entry.sh
func selectSnapshotWithoutReverse(snapshots []KopiaSnapshot, previousOffset int) (string, error) {
	if len(snapshots) == 0 {
		return "", fmt.Errorf("no snapshots found")
	}
	if previousOffset >= len(snapshots) {
		return "", fmt.Errorf("offset %d exceeds snapshot count %d", previousOffset, len(snapshots))
	}
	// Current implementation: directly uses index without reversing
	// This incorrectly assumes [0] is newest, but it's actually oldest
	return snapshots[previousOffset].ID, nil
}

// selectSnapshotWithReverse simulates the FIXED behavior with reverse
func selectSnapshotWithReverse(snapshots []KopiaSnapshot, previousOffset int) (string, error) {
	if len(snapshots) == 0 {
		return "", fmt.Errorf("no snapshots found")
	}
	if previousOffset >= len(snapshots) {
		return "", fmt.Errorf("offset %d exceeds snapshot count %d", previousOffset, len(snapshots))
	}
	// Fixed implementation: reverse the array first to get newest first
	reversed := make([]KopiaSnapshot, len(snapshots))
	for i, snapshot := range snapshots {
		reversed[len(snapshots)-1-i] = snapshot
	}
	return reversed[previousOffset].ID, nil
}

var _ = Describe("Kopia Snapshot Selection", func() {
	Context("When selecting snapshots for restore", func() {
		var snapshots []KopiaSnapshot

		BeforeEach(func() {
			snapshots = simulateKopiaSnapshotList()
		})

		It("should demonstrate that Kopia returns snapshots in chronological order (oldest first)", func() {
			// Verify our test data matches Kopia's behavior
			Expect(snapshots).To(HaveLen(5))
			
			// First element should be the oldest
			Expect(snapshots[0].ID).To(ContainSubstring("week-old"))
			Expect(snapshots[0].Description).To(ContainSubstring("week old"))
			
			// Last element should be the newest
			Expect(snapshots[len(snapshots)-1].ID).To(ContainSubstring("latest"))
			Expect(snapshots[len(snapshots)-1].Description).To(ContainSubstring("latest"))
			
			// Verify chronological ordering
			for i := 1; i < len(snapshots); i++ {
				Expect(snapshots[i].StartTime.After(snapshots[i-1].StartTime)).To(BeTrue(),
					fmt.Sprintf("Snapshot %d should be newer than snapshot %d", i, i-1))
			}
		})

		It("should show the bug: current implementation selects oldest instead of newest", func() {
			// Test with previousOffset=0 (should select latest, but gets oldest)
			selected, err := selectSnapshotWithoutReverse(snapshots, 0)
			Expect(err).NotTo(HaveOccurred())
			
			// BUG: This selects the OLDEST snapshot instead of the newest!
			Expect(selected).To(ContainSubstring("week-old"), "Current buggy behavior: selects oldest when expecting newest")
			Expect(selected).NotTo(ContainSubstring("latest"), "Should have selected latest, but didn't")
		})

		It("should show the bug affects all offsets", func() {
			// previousOffset=1 should get the second-newest, but gets second-oldest
			selected, err := selectSnapshotWithoutReverse(snapshots, 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(ContainSubstring("oldest"), "Gets second-oldest instead of second-newest")
			
			// previousOffset=2 should get third-newest, but gets third-oldest
			selected, err = selectSnapshotWithoutReverse(snapshots, 2)
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(ContainSubstring("yesterday"), "Gets middle snapshot by accident")
		})

		It("should demonstrate the fix: reversing the array selects correctly", func() {
			// Test with previousOffset=0 (should correctly select latest)
			selected, err := selectSnapshotWithReverse(snapshots, 0)
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(ContainSubstring("latest"), "Fixed behavior: correctly selects newest")
			
			// Test with previousOffset=1 (should select second-newest)
			selected, err = selectSnapshotWithReverse(snapshots, 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(ContainSubstring("hour-ago"), "Correctly selects second-newest")
			
			// Test with previousOffset=2 (should select third-newest)
			selected, err = selectSnapshotWithReverse(snapshots, 2)
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(ContainSubstring("yesterday"), "Correctly selects third-newest")
		})

		It("should handle edge cases correctly", func() {
			// Empty snapshot list
			emptySnapshots := []KopiaSnapshot{}
			_, err := selectSnapshotWithReverse(emptySnapshots, 0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no snapshots"))
			
			// Offset exceeds available snapshots
			_, err = selectSnapshotWithReverse(snapshots, 10)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exceeds snapshot count"))
		})

		It("should demonstrate the real-world impact", func() {
			// Simulate what users are experiencing
			fmt.Fprintf(GinkgoWriter, "\n=== Real-World Impact ===\n")
			fmt.Fprintf(GinkgoWriter, "User expects to restore: %s\n", snapshots[len(snapshots)-1].ID)
			
			buggySelection, _ := selectSnapshotWithoutReverse(snapshots, 0)
			fmt.Fprintf(GinkgoWriter, "Current bug restores: %s\n", buggySelection)
			
			fixedSelection, _ := selectSnapshotWithReverse(snapshots, 0)
			fmt.Fprintf(GinkgoWriter, "After fix restores: %s\n", fixedSelection)
			
			// Calculate how old the wrong snapshot is
			for _, s := range snapshots {
				if s.ID == buggySelection {
					age := time.Since(s.StartTime)
					fmt.Fprintf(GinkgoWriter, "Bug causes restore of data that is %.1f days old!\n", age.Hours()/24)
					break
				}
			}
		})

		It("should verify JSON parsing behavior matches shell script", func() {
			// Test that our Go structs correctly parse Kopia JSON output
			jsonOutput, err := json.Marshal(snapshots)
			Expect(err).NotTo(HaveOccurred())
			
			var parsedSnapshots []KopiaSnapshot
			err = json.Unmarshal(jsonOutput, &parsedSnapshots)
			Expect(err).NotTo(HaveOccurred())
			
			Expect(len(parsedSnapshots)).To(Equal(len(snapshots)))
			// Compare IDs instead of full structs to avoid time comparison issues
			for i := range parsedSnapshots {
				Expect(parsedSnapshots[i].ID).To(Equal(snapshots[i].ID))
			}
			
			// Simulate jq processing: '.[0].id' on original array
			fmt.Fprintf(GinkgoWriter, "\nSimulating shell script behavior:\n")
			fmt.Fprintf(GinkgoWriter, "jq '.[0].id' (current): %s\n", parsedSnapshots[0].ID)
			fmt.Fprintf(GinkgoWriter, "jq 'reverse | .[0].id' (fixed): %s\n", parsedSnapshots[len(parsedSnapshots)-1].ID)
		})
	})
})