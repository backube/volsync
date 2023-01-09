/*
Copyright 2022 The VolSync authors.

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

package restic_test

import (
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	restic "github.com/backube/volsync/controllers/mover/restic"
	"github.com/backube/volsync/controllers/utils"
)

var _ = Describe("Restic Filter Logs Tests", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	Context("Restic source mover logs", func() {
		// Sample backup log for restic mover
		// nolint:lll
		resticSourceLogSuccessful := `Starting container
VolSync restic container version: unknown
backup
restic 0.14.0 (v0.3.0-952-g444628f-dirty) compiled with go1.19.3 on linux/amd64
Testing mandatory env variables
== Checking directory for content ===
== Initialize Dir =======
created restic repository f5bccd54c8 at s3:http://minio-api-minio.apps.app-aws-411ga-sno-net2-zp5jq.dev06.red-chesterfield.com/ttest-restic-new

Please note that knowledge of your password is required to access
the repository. Losing your password means that your data is
irrecoverably lost.
=== Starting backup ===
~/DEVFEDORA/volsync/TESTDATA ~/DEVFEDORA/volsync/RESTICTESTS
repository f5bccd54 opened (repository version 2) successfully, password is correct
created new cache in /home/testuser/DEVFEDORA/volsync/RESTICTESTS/CACHE
no parent snapshot found, will read all files

Files:          25 new,     0 changed,     0 unmodified
Dirs:            3 new,     0 changed,     0 unmodified
Added to the repository: 12.941 MiB (12.529 MiB stored)

processed 25 files, 36.658 MiB in 0:12
snapshot 0ff74383 saved
~/DEVFEDORA/volsync/RESTICTESTS
=== Starting forget ===
Restic completed in 18s
=== Done ===`

		/*
		   		anotherRestic := `Starting container
		   VolSync restic container version: v0.6.0+7888b78
		   backup
		   restic 0.14.0 compiled with go1.19.4 on linux/amd64
		   Testing mandatory env variables
		   == Checking directory for content ===
		   == Initialize Dir =======
		   ID        Time                 Host        Tags        Paths
		   ------------------------------------------------------------
		   1d75de4c  2023-01-09 21:45:42  volsync                 /data
		   ------------------------------------------------------------
		   1 snapshots
		   === Starting backup ===
		   /data /
		   using parent snapshot 1d75de4c
		   [0:00] 0 files 0 B, total 1 files 5 B, 0 errors

		   Files:           0 new,     1 changed,     0 unmodified
		   Dirs:            0 new,     1 changed,     0 unmodified
		   Added to the repository: 923 B (498 B stored)

		   processed 1 files, 5 B in 0:00
		   snapshot 114586b7 saved
		   /
		   === Starting forget ===
		   Applying Policy: keep 3 hourly, 2 daily, 1 monthly snapshots
		   keep 1 snapshots:
		   ID        Time                 Host        Tags        Reasons           Paths
		   ------------------------------------------------------------------------------
		   114586b7  2023-01-09 21:46:02  volsync                 hourly snapshot   /data
		                                                          daily snapshot
		                                                          monthly snapshot
		   ------------------------------------------------------------------------------
		   1 snapshots

		   remove 1 snapshots:
		   ID        Time                 Host        Tags        Paths
		   ------------------------------------------------------------
		   1d75de4c  2023-01-09 21:45:42  volsync                 /data
		   ------------------------------------------------------------
		   1 snapshots

		   [0:00] 100.00%  1 / 1 files deleted

		   Restic completed in 8s
		   === Done ===`
		*/

		// nolint:lll
		expectedFilteredResticSourceLogSuccessful := `repository f5bccd54 opened (repository version 2) successfully, password is correct
no parent snapshot found, will read all files
Added to the repository: 12.941 MiB (12.529 MiB stored)
processed 25 files, 36.658 MiB in 0:12
snapshot 0ff74383 saved
Restic completed in 18s`

		It("Should filter the logs from a successful replication source (restic backup)", func() {
			reader := strings.NewReader(resticSourceLogSuccessful)
			filteredLines, err := utils.FilterLogs(reader, restic.LogLineFilterSuccess)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Logs after filter", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredResticSourceLogSuccessful))
		})
	})

	Context("Restic dest mover logs", func() {
		// Sample restore log for restic mover
		// nolint:lll
		resticDestlogSuccessful := `Starting container
VolSync restic container version: unknown
restore
restic 0.14.0 (v0.3.0-952-g444628f-dirty) compiled with go1.19.3 on linux/amd64
Testing mandatory env variables
=== Starting restore ===
~/DEVFEDORA/volsync/RESTICTESTS/RESTOREDIR ~/DEVFEDORA/volsync/RESTICTESTS
Selected restic snapshot with id: 0ff74383
repository f5bccd54 opened (repository version 2) successfully, password is correct
restoring <Snapshot 0ff74383 of [/home/testuser/DEVFEDORA/volsync/TESTDATA] at 2022-12-15 11:10:01.858799017 -0500 EST by testuser@volsync> to .
~/DEVFEDORA/volsync/RESTICTESTS
Restic completed in 9s
=== Done ===`

		// nolint:lll
		expectedFilteredResticDestlogSuccessful := `repository f5bccd54 opened (repository version 2) successfully, password is correct
restoring <Snapshot 0ff74383 of [/home/testuser/DEVFEDORA/volsync/TESTDATA] at 2022-12-15 11:10:01.858799017 -0500 EST by testuser@volsync> to .
Restic completed in 9s`

		It("Should filter the logs from a successful replication dest (restic restore)", func() {
			reader := strings.NewReader(resticDestlogSuccessful)
			filteredLines, err := utils.FilterLogs(reader, restic.LogLineFilterSuccess)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Logs after filter", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredResticDestlogSuccessful))
		})
	})
})
