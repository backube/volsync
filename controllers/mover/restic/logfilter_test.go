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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	restic "github.com/backube/volsync/controllers/mover/restic"
	"github.com/backube/volsync/controllers/utils"
)

var _ = Describe("Restic Filter Logs Tests", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	Context("Restic source mover logs", func() {
		It("Should filter the logs from a successful replication source (restic backup)", func() {
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

			// nolint:lll
			expectedFilteredResticSourceLogSuccessful := `repository f5bccd54 opened (repository version 2) successfully, password is correct
no parent snapshot found, will read all files
Added to the repository: 12.941 MiB (12.529 MiB stored)
processed 25 files, 36.658 MiB in 0:12
snapshot 0ff74383 saved
Restic completed in 18s`

			reader := strings.NewReader(resticSourceLogSuccessful)
			filteredLines, err := utils.FilterLogs(reader, restic.LogLineFilterSuccess)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Logs after filter", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredResticSourceLogSuccessful))
		})

		It("Should filter the logs from a replication source (restic backup) that performed an unlock", func() {
			// Sample backup log for restic mover
			resticSourceLog := `Starting container
VolSync restic container version: v0.8.0+a0a0bb8-dirty
unlock backup
restic 0.15.1 compiled with go1.19.3 on linux/amd64
Testing mandatory env variables
=== Starting unlock ===
913a91c60431342abb402d7707f50a370c52a911e01abdf4160e5d41a77e5151
successfully removed 1 locks
== Checking directory for content ===
== Initialize Dir =======
ID        Time                 Host         Tags        Paths
------------------------------------------------------------------------
4e825939  2023-04-07 20:17:00  volsync                  /mover-syncthing
29baa6a9  2023-04-07 21:27:34  volsync                  /mover-syncthing
f617a68d  2023-04-07 23:55:22  volsync                  /data
ea83f94a  2023-04-08 00:00:36  volsync                  /data
b126807d  2023-04-08 01:14:51  ttestrestic              /home
6cd03c8e  2023-04-08 01:19:27  volsync                  /data
eaf1a6ed  2023-04-08 02:53:08  volsync                  /data
------------------------------------------------------------------------
7 snapshots
=== Starting backup ===
/data /
using parent snapshot eaf1a6ed

Files:           0 new,     4 changed,     0 unmodified
Dirs:            0 new,     0 changed,     0 unmodified
Added to the repository: 1.653 KiB (562 B stored)

processed 4 files, 1.494 KiB in 0:00
snapshot 6b128c1e saved
/
=== Starting forget ===
Applying Policy: keep 3 hourly, 2 daily, 1 monthly snapshots
keep 2 snapshots:
ID        Time                 Host        Tags        Reasons           Paths
-----------------------------------------------------------------------------------------
4e825939  2023-04-07 20:17:00  volsync                 hourly snapshot   /mover-syncthing
29baa6a9  2023-04-07 21:27:34  volsync                 hourly snapshot   /mover-syncthing
                                                       daily snapshot
                                                       monthly snapshot
-----------------------------------------------------------------------------------------
2 snapshots

keep 4 snapshots:
ID        Time                 Host        Tags        Reasons           Paths
------------------------------------------------------------------------------
f617a68d  2023-04-07 23:55:22  volsync                 daily snapshot    /data
6cd03c8e  2023-04-08 01:19:27  volsync                 hourly snapshot   /data
eaf1a6ed  2023-04-08 02:53:08  volsync                 hourly snapshot   /data
6b128c1e  2023-04-08 04:23:11  volsync                 hourly snapshot   /data
                                                       daily snapshot
                                                       monthly snapshot
------------------------------------------------------------------------------
4 snapshots

remove 1 snapshots:
ID        Time                 Host        Tags        Paths
------------------------------------------------------------
ea83f94a  2023-04-08 00:00:36  volsync                 /data
------------------------------------------------------------
1 snapshots

[0:00] 100.00%  1 / 1 files deleted

Restic completed in 4s
=== Done ===
`

			expectedFilteredResticSourceLog := `successfully removed 1 locks
using parent snapshot eaf1a6ed
Added to the repository: 1.653 KiB (562 B stored)
processed 4 files, 1.494 KiB in 0:00
snapshot 6b128c1e saved
Restic completed in 4s`

			reader := strings.NewReader(resticSourceLog)
			filteredLines, err := utils.FilterLogs(reader, restic.LogLineFilterSuccess)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Logs after filter", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredResticSourceLog))
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
