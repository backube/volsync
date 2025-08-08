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
	"regexp"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
)

// SnapshotInfo represents a snapshot in Kopia's JSON output
type SnapshotInfo struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	Username    string    `json:"userName"`
	Hostname    string    `json:"hostName"`
	Path        string    `json:"path"`
	StartTime   time.Time `json:"startTime"`
	EndTime     time.Time `json:"endTime"`
	Description string    `json:"description,omitempty"`
}

// ParseKopiaDiscoveryOutput parses Kopia job output to extract discovery information
// when snapshots cannot be found for the requested identity
//
//nolint:funlen,lll
func ParseKopiaDiscoveryOutput(logs string) (requestedIdentity string, availableIdentities []volsyncv1alpha1.KopiaIdentityInfo, errorMsg string) {
	lines := strings.Split(logs, "\n")
	identityMap := make(map[string]*volsyncv1alpha1.KopiaIdentityInfo)

	// Regular expressions for parsing
	noSnapshotsRegex := regexp.MustCompile(`No snapshots found for ([^@]+)@([^:]+):(.+)`)
	snapshotListingRegex := regexp.MustCompile(`^([^@]+)@([^:]+):(.+)\s+(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2})`)
	identityErrorRegex := regexp.MustCompile(`unable to find snapshots for source "([^"]+)"`)
	discoveryHeaderRegex := regexp.MustCompile(`=== Discovery Mode: Available Snapshots ===`)
	jsonSnapshotRegex := regexp.MustCompile(`^\{.*"id".*"userName".*"hostName".*\}`)

	var inDiscoveryMode bool
	var jsonSnapshots []string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for discovery mode header
		if discoveryHeaderRegex.MatchString(line) {
			inDiscoveryMode = true
			continue
		}

		// Extract requested identity from error messages
		if matches := noSnapshotsRegex.FindStringSubmatch(line); len(matches) > 2 {
			requestedIdentity = fmt.Sprintf("%s@%s", matches[1], matches[2])
			errorMsg = line
			continue
		}

		if matches := identityErrorRegex.FindStringSubmatch(line); len(matches) > 1 {
			// Extract username@hostname from the source path
			parts := strings.Split(matches[1], ":")
			if len(parts) > 0 {
				requestedIdentity = parts[0]
			}
			errorMsg = fmt.Sprintf("No snapshots found for identity '%s'", requestedIdentity)
			continue
		}

		// In discovery mode, collect JSON snapshot data
		if inDiscoveryMode && jsonSnapshotRegex.MatchString(line) {
			jsonSnapshots = append(jsonSnapshots, line)
			continue
		}

		// Parse traditional snapshot listing format
		if matches := snapshotListingRegex.FindStringSubmatch(line); len(matches) > 4 {
			username := matches[1]
			hostname := matches[2]
			identity := fmt.Sprintf("%s@%s", username, hostname)

			if _, exists := identityMap[identity]; !exists {
				identityMap[identity] = &volsyncv1alpha1.KopiaIdentityInfo{
					Identity:      identity,
					SnapshotCount: 0,
				}
			}

			identityMap[identity].SnapshotCount++

			// Parse timestamp for latest snapshot
			if timestamp, err := time.Parse("2006-01-02 15:04:05", matches[4]); err == nil {
				if identityMap[identity].LatestSnapshot == nil ||
					timestamp.After(identityMap[identity].LatestSnapshot.Time) {
					latestTime := metav1.NewTime(timestamp)
					identityMap[identity].LatestSnapshot = &latestTime
				}
			}
		}
	}

	// Process JSON snapshots if found
	if len(jsonSnapshots) > 0 {
		processJSONSnapshots(jsonSnapshots, identityMap)
	}

	// Convert map to slice and sort by identity
	for _, info := range identityMap {
		availableIdentities = append(availableIdentities, *info)
	}

	// Sort by identity for consistent output
	sort.Slice(availableIdentities, func(i, j int) bool {
		return availableIdentities[i].Identity < availableIdentities[j].Identity
	})

	// Generate better error message if we have discovery information
	if errorMsg == "" && requestedIdentity != "" && len(availableIdentities) > 0 {
		var identityList []string
		for _, id := range availableIdentities {
			identityList = append(identityList, id.Identity)
		}
		errorMsg = fmt.Sprintf("No snapshots found for identity '%s'. Available identities: %s",
			requestedIdentity, strings.Join(identityList, ", "))
	}

	return requestedIdentity, availableIdentities, errorMsg
}

// processJSONSnapshots processes JSON-formatted snapshot information
func processJSONSnapshots(jsonLines []string, identityMap map[string]*volsyncv1alpha1.KopiaIdentityInfo) {
	for _, line := range jsonLines {
		var snapshot SnapshotInfo
		if err := json.Unmarshal([]byte(line), &snapshot); err != nil {
			continue
		}

		identity := fmt.Sprintf("%s@%s", snapshot.Username, snapshot.Hostname)

		if _, exists := identityMap[identity]; !exists {
			identityMap[identity] = &volsyncv1alpha1.KopiaIdentityInfo{
				Identity:      identity,
				SnapshotCount: 0,
			}
		}

		identityMap[identity].SnapshotCount++

		// Update latest snapshot time
		if identityMap[identity].LatestSnapshot == nil ||
			snapshot.EndTime.After(identityMap[identity].LatestSnapshot.Time) {
			latestTime := metav1.NewTime(snapshot.EndTime)
			identityMap[identity].LatestSnapshot = &latestTime
		}
	}
}

// LogFilter returns a filter function for Kopia mover logs
// It extracts meaningful error messages and discovery information
func LogFilter(line string) *string {
	// Always include error messages
	if strings.Contains(strings.ToLower(line), "error") ||
		strings.Contains(strings.ToLower(line), "failed") ||
		strings.Contains(strings.ToLower(line), "unable to") {
		return &line
	}

	// Include discovery mode information
	if strings.Contains(line, "=== Discovery Mode") ||
		strings.Contains(line, "Available Snapshots") ||
		strings.Contains(line, "No snapshots found") {
		return &line
	}

	// Include snapshot listing in discovery mode
	if regexp.MustCompile(`^[^@]+@[^:]+:.+\s+\d{4}-\d{2}-\d{2}`).MatchString(line) {
		return &line
	}

	// Include JSON snapshot data
	if strings.Contains(line, `"userName"`) && strings.Contains(line, `"hostName"`) {
		return &line
	}

	// Include important status messages
	if strings.Contains(line, "Snapshot created successfully") ||
		strings.Contains(line, "Snapshot restore completed") ||
		strings.Contains(line, "Repository connected") ||
		strings.Contains(line, "No eligible snapshots") {
		return &line
	}

	return nil
}
