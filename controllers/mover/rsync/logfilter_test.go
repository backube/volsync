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

package rsync_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	rsync "github.com/backube/volsync/controllers/mover/rsync"
	"github.com/backube/volsync/controllers/utils"
)

var _ = Describe("Rsync Filter Logs Tests", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	Context("Rsync source mover logs", func() {
		// Sample source log for rsync mover
		rsyncSourceLog := `VolSync rsync container version: v0.6.0+5d8dcef-dirty
Syncing data to 10.96.145.93:22 ...
.d..t...... ./
cd+++++++++ TESTDATA/
<f+++++++++ TESTDATA/vmlinuz-0-rescue-b1184baf4f57425dbd7f28a601c3bab7
cd+++++++++ TESTDATA/DIR1/
<f+++++++++ TESTDATA/DIR1/config-6.0.7-301.fc37.x86_64
cd+++++++++ TESTDATA/DIR2/
<f+++++++++ TESTDATA/DIR2/os-release
cd+++++++++ TESTDATA/DIR3/
<f+++++++++ TESTDATA/DIR3/README
<f+++++++++ TESTDATA/DIR3/aaa.txt
<f+++++++++ TESTDATA/DIR3/basic.conf
<f+++++++++ TESTDATA/DIR3/chrony.conf
<f+++++++++ TESTDATA/DIR3/cyrus-sasl.conf
<f+++++++++ TESTDATA/DIR3/dbus.conf
<f+++++++++ TESTDATA/DIR3/dnsmasq.conf
<f+++++++++ TESTDATA/DIR3/flatpak.conf
<f+++++++++ TESTDATA/DIR3/gamemode.conf
<f+++++++++ TESTDATA/DIR3/gdm.conf
<f+++++++++ TESTDATA/DIR3/gnome-initial-setup.conf
<f+++++++++ TESTDATA/DIR3/kubernetes.conf
<f+++++++++ TESTDATA/DIR3/plocate.conf
<f+++++++++ TESTDATA/DIR3/samba.conf
<f+++++++++ TESTDATA/DIR3/systemd-coredump.conf
<f+++++++++ TESTDATA/DIR3/systemd-journal.conf
<f+++++++++ TESTDATA/DIR3/systemd-network.conf
<f+++++++++ TESTDATA/DIR3/systemd-oom.conf
<f+++++++++ TESTDATA/DIR3/systemd-resolve.conf
<f+++++++++ TESTDATA/DIR3/systemd-timesync.conf
<f+++++++++ TESTDATA/DIR3/tcpdump.conf
<f+++++++++ TESTDATA/DIR3/tpm2-tss.conf

Number of files: 31 (reg: 26, dir: 5)
Number of created files: 29 (reg: 25, dir: 4)
Number of deleted files: 0
Number of regular files transferred: 25
Total file size: 38.44M bytes
Total transferred file size: 38.44M bytes
Literal data: 38.44M bytes
Matched data: 0 bytes
File list size: 0
File list generation time: 0.001 seconds
File list transfer time: 0.000 seconds
Total bytes sent: 37.57M
Total bytes received: 529

sent 37.57M bytes  received 529 bytes  25.05M bytes/sec
total size is 38.44M  speedup is 1.02
Rsync completed in 1s
Synchronization completed successfully. Notifying destination...
Initiating shutdown. Exit code: 0`

		// nolint:lll
		expectedFilteredLog := `sent 37.57M bytes  received 529 bytes  25.05M bytes/sec
total size is 38.44M  speedup is 1.02
Rsync completed in 1s`

		It("Should filter the logs", func() {
			reader := strings.NewReader(rsyncSourceLog)
			filteredLines, err := utils.FilterLogs(reader, rsync.LogLineFilterSuccess)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Filtered lines are", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredLog))
		})
	})

	Context("Rsync dest mover logs", func() {
		// Sample dest log for volsync
		// nolint:lll
		rsyncDestLog := `VolSync rsync container version: v0.6.0+5d8dcef-dirty
Waiting for connection...
Exiting... Exit code: 0`

		expectedFilteredLog := "" // Currently not filtering for any of the dest log contents

		It("Should filter the logs", func() {
			reader := strings.NewReader(rsyncDestLog)
			filteredLines, err := utils.FilterLogs(reader, rsync.LogLineFilterSuccess)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Filtered lines are", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredLog))
		})
	})
})
