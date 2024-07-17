//go:build !disable_rclone

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

package rclone_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	rclone "github.com/backube/volsync/controllers/mover/rclone"
	"github.com/backube/volsync/controllers/utils"
)

var _ = Describe("Rclone Filter Logs Tests", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	Context("Rclone source mover logs", func() {
		// Sample source log for rclone mover
		// nolint:lll
		sourceLog := `VolSync rclone container version: v0.6.0+39a85b7-dirty
getfacl: Removing leading '/' from absolute path names
2023/01/09 20:00:47 DEBUG : Setting --config "/rclone-config/rclone.conf" from environment variable RCLONE_CONFIG="/rclone-config/rclone.conf"
2023/01/09 20:00:47 DEBUG : rclone: Version "v1.60.1" starting with parameters ["rclone" "sync" "--checksum" "--one-file-system" "--create-empty-src-dirs" "--stats" "20s" "--transfers" "10" "/data" "rclone-data-mover:rclone-test-0-zx42b" "--log-level" "DEBUG"]
2023/01/09 20:00:47 DEBUG : Creating backend with remote "/data"
2023/01/09 20:00:47 DEBUG : Using config file from "/rclone-config/rclone.conf"
2023/01/09 20:00:47 DEBUG : local: detected overridden config - adding "{vECL7}" suffix to name
2023/01/09 20:00:47 DEBUG : fs cache: renaming cache item "/data" to be canonical "local{vECL7}:/data"
2023/01/09 20:00:47 DEBUG : Creating backend with remote "rclone-data-mover:rclone-test-0-zx42b"
2023/01/09 20:00:52 DEBUG : outfile: Sizes differ (src 715828 vs dst 715296)
2023/01/09 20:00:52 DEBUG : datafile: md5 = 6137cde4893c59f76f005a8123d8e8e6 OK
2023/01/09 20:00:52 DEBUG : datafile: Size and md5 of src and dst objects identical
2023/01/09 20:00:52 DEBUG : datafile: Unchanged skipping
2023/01/09 20:00:52 DEBUG : S3 bucket rclone-test-0-zx42b: Waiting for checks to finish
2023/01/09 20:00:52 DEBUG : TESTDIR2/file2: md5 = 8be2afc0c3d8fea26d7e674ee6e56867 OK
2023/01/09 20:00:52 DEBUG : TESTDIR2/file2: Size and md5 of src and dst objects identical
2023/01/09 20:00:52 DEBUG : TESTDIR2/file2: Unchanged skipping
2023/01/09 20:00:52 DEBUG : TESTDIR2/file3: md5 = 9513a5e720188686c24dba9b99b44198 OK
2023/01/09 20:00:52 DEBUG : TESTDIR2/file3: Size and md5 of src and dst objects identical
2023/01/09 20:00:52 DEBUG : TESTDIR2/file3: Unchanged skipping
2023/01/09 20:00:52 DEBUG : TESTDIR1/file1: md5 = 67f30a9f1d05a2a348b8152382022eb6 OK
2023/01/09 20:00:52 DEBUG : TESTDIR1/file1: Size and md5 of src and dst objects identical
2023/01/09 20:00:52 DEBUG : TESTDIR1/file1: Unchanged skipping
2023/01/09 20:00:52 DEBUG : S3 bucket rclone-test-0-zx42b: Waiting for transfers to finish
2023/01/09 20:00:52 DEBUG : outfile: md5 = 4d362fb579a7b9a9af219c7897270580 OK
2023/01/09 20:00:52 INFO  : outfile: Copied (replaced existing)
2023/01/09 20:00:52 DEBUG : lost+found: Making directory
2023/01/09 20:00:52 DEBUG : S3 bucket rclone-test-0-zx42b: copied 1 directories
2023/01/09 20:00:52 DEBUG : Waiting for deletions to finish
2023/01/09 20:00:52 INFO  : permissions.facl: Deleted
2023/01/09 20:00:52 INFO  :
Transferred:         699.051 KiB / 699.051 KiB, 100%, 0 B/s, ETA -
Checks:                 6 / 6, 100%
Deleted:                1 (files), 0 (dirs)
Transferred:            1 / 1, 100%
Elapsed time:         5.0s

2023/01/09 20:00:52 DEBUG : 10 go routines active
2023/01/09 20:00:52 DEBUG : Setting --config "/rclone-config/rclone.conf" from environment variable RCLONE_CONFIG="/rclone-config/rclone.conf"
2023/01/09 20:00:52 DEBUG : rclone: Version "v1.60.1" starting with parameters ["rclone" "copy" "--checksum" "--one-file-system" "--create-empty-src-dirs" "--stats-one-line-date" "--stats" "20s" "--transfers" "10" "--include" "permissions.facl" "/tmp" "rclone-data-mover:rclone-test-0-zx42b" "--log-level" "DEBUG"]
2023/01/09 20:00:52 DEBUG : Creating backend with remote "/tmp"
2023/01/09 20:00:52 DEBUG : Using config file from "/rclone-config/rclone.conf"
2023/01/09 20:00:52 DEBUG : local: detected overridden config - adding "{vECL7}" suffix to name
2023/01/09 20:00:52 DEBUG : fs cache: renaming cache item "/tmp" to be canonical "local{vECL7}:/tmp"
2023/01/09 20:00:52 DEBUG : Creating backend with remote "rclone-data-mover:rclone-test-0-zx42b"
2023/01/09 20:00:52 DEBUG : datafile: Excluded
2023/01/09 20:00:52 DEBUG : outfile: Excluded
2023/01/09 20:00:52 DEBUG : S3 bucket rclone-test-0-zx42b: Waiting for checks to finish
2023/01/09 20:00:52 DEBUG : S3 bucket rclone-test-0-zx42b: Waiting for transfers to finish
2023/01/09 20:00:52 DEBUG : permissions.facl: md5 = f6b91300cd0b781be85b8cd45ee1d56c OK
2023/01/09 20:00:52 INFO  : permissions.facl: Copied (new)
2023/01/09 20:00:52 INFO  : 2023/01/09 20:00:52 -         869 B / 869 B, 100%, 0 B/s, ETA -
2023/01/09 20:00:52 DEBUG : 5 go routines active
Rclone completed in 5s`

		expectedFilteredLog := `Transferred:         699.051 KiB / 699.051 KiB, 100%, 0 B/s, ETA -
Checks:                 6 / 6, 100%
Deleted:                1 (files), 0 (dirs)
Transferred:            1 / 1, 100%
Elapsed time:         5.0s
Rclone completed in 5s`

		It("Should filter the logs", func() {
			reader := strings.NewReader(sourceLog)
			filteredLines, err := utils.FilterLogs(reader, rclone.LogLineFilterSuccess)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Logs after filter", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredLog))
		})
	})

	Context("Rclone dest mover logs", func() {
		// Sample dest log for rclone mover
		// nolint:lll
		destLog := `VolSync rclone container version: v0.6.0+39a85b7-dirty
2023/01/09 20:04:51 DEBUG : Setting --config "/rclone-config/rclone.conf" from environment variable RCLONE_CONFIG="/rclone-config/rclone.conf"
2023/01/09 20:04:51 DEBUG : rclone: Version "v1.60.1" starting with parameters ["rclone" "sync" "--checksum" "--one-file-system" "--create-empty-src-dirs" "--stats" "20s" "--transfers" "10" "--exclude" "permissions.facl" "rclone-data-mover:rclone-test-0-zx42b" "/data" "--log-level" "DEBUG"]
2023/01/09 20:04:51 DEBUG : Creating backend with remote "rclone-data-mover:rclone-test-0-zx42b"
2023/01/09 20:04:51 DEBUG : Using config file from "/rclone-config/rclone.conf"
2023/01/09 20:04:51 DEBUG : Creating backend with remote "/data"
2023/01/09 20:04:51 DEBUG : local: detected overridden config - adding "{vECL7}" suffix to name
2023/01/09 20:04:51 DEBUG : fs cache: renaming cache item "/data" to be canonical "local{vECL7}:/data"
2023/01/09 20:04:56 DEBUG : permissions.facl: Excluded
2023/01/09 20:04:56 DEBUG : outfile: Sizes differ (src 715828 vs dst 580)
2023/01/09 20:04:56 DEBUG : datafile: md5 = 6137cde4893c59f76f005a8123d8e8e6 OK
2023/01/09 20:04:56 DEBUG : datafile: Size and md5 of src and dst objects identical
2023/01/09 20:04:56 DEBUG : datafile: Unchanged skipping
2023/01/09 20:04:56 DEBUG : Local file system at /data: Waiting for checks to finish
2023/01/09 20:04:56 DEBUG : TESTDIR2/file2: md5 = 8be2afc0c3d8fea26d7e674ee6e56867 OK
2023/01/09 20:04:56 DEBUG : TESTDIR2/file2: Size and md5 of src and dst objects identical
2023/01/09 20:04:56 DEBUG : TESTDIR2/file2: Unchanged skipping
2023/01/09 20:04:56 DEBUG : TESTDIR2/file3: md5 = 9513a5e720188686c24dba9b99b44198 OK
2023/01/09 20:04:56 DEBUG : TESTDIR2/file3: Size and md5 of src and dst objects identical
2023/01/09 20:04:56 DEBUG : TESTDIR2/file3: Unchanged skipping
2023/01/09 20:04:56 DEBUG : outfile: md5 = 4d362fb579a7b9a9af219c7897270580 OK
2023/01/09 20:04:56 INFO  : outfile: Copied (replaced existing)
2023/01/09 20:04:56 DEBUG : TESTDIR1/file1: md5 = 67f30a9f1d05a2a348b8152382022eb6 OK
2023/01/09 20:04:56 DEBUG : TESTDIR1/file1: Size and md5 of src and dst objects identical
2023/01/09 20:04:56 DEBUG : TESTDIR1/file1: Unchanged skipping
2023/01/09 20:04:56 DEBUG : Local file system at /data: Waiting for transfers to finish
2023/01/09 20:04:56 DEBUG : Waiting for deletions to finish
2023/01/09 20:04:56 INFO  :
Transferred:         699.051 KiB / 699.051 KiB, 100%, 0 B/s, ETA -
Checks:                 5 / 5, 100%
Transferred:            1 / 1, 100%
Elapsed time:         5.0s

2023/01/09 20:04:56 DEBUG : 10 go routines active
2023/01/09 20:04:56 DEBUG : Setting --config "/rclone-config/rclone.conf" from environment variable RCLONE_CONFIG="/rclone-config/rclone.conf"
2023/01/09 20:04:56 DEBUG : rclone: Version "v1.60.1" starting with parameters ["rclone" "copy" "--checksum" "--one-file-system" "--create-empty-src-dirs" "--stats-one-line-date" "--stats" "20s" "--transfers" "10" "--include" "permissions.facl" "rclone-data-mover:rclone-test-0-zx42b" "/tmp" "--log-level" "DEBUG"]
2023/01/09 20:04:56 DEBUG : Creating backend with remote "rclone-data-mover:rclone-test-0-zx42b"
2023/01/09 20:04:56 DEBUG : Using config file from "/rclone-config/rclone.conf"
2023/01/09 20:04:56 DEBUG : Creating backend with remote "/tmp"
2023/01/09 20:04:56 DEBUG : local: detected overridden config - adding "{vECL7}" suffix to name
2023/01/09 20:04:56 DEBUG : fs cache: renaming cache item "/tmp" to be canonical "local{vECL7}:/tmp"
2023/01/09 20:04:56 DEBUG : datafile: Excluded
2023/01/09 20:04:56 DEBUG : outfile: Excluded
2023/01/09 20:04:56 DEBUG : TESTDIR2/file2: Excluded
2023/01/09 20:04:56 DEBUG : TESTDIR2/file3: Excluded
2023/01/09 20:04:56 DEBUG : TESTDIR1/file1: Excluded
2023/01/09 20:04:56 DEBUG : permissions.facl: md5 = f6b91300cd0b781be85b8cd45ee1d56c OK
2023/01/09 20:04:56 INFO  : permissions.facl: Copied (new)
2023/01/09 20:04:56 DEBUG : Local file system at /tmp: Waiting for checks to finish
2023/01/09 20:04:56 DEBUG : Local file system at /tmp: Waiting for transfers to finish
2023/01/09 20:04:56 DEBUG : TESTDIR1: Making directory
2023/01/09 20:04:56 DEBUG : TESTDIR2: Making directory
2023/01/09 20:04:56 DEBUG : Local file system at /tmp: copied 2 directories
2023/01/09 20:04:56 INFO  : 2023/01/09 20:04:56 -         869 B / 869 B, 100%, 0 B/s, ETA -
2023/01/09 20:04:56 DEBUG : 9 go routines active
  File: /tmp/permissions.facl
  Size: 869           Blocks: 8          IO Block: 4096   regular file
Device: 200081h/2097281d    Inode: 1885795     Links: 1
Access: (0644/-rw-r--r--)  Uid: (1000830000/1000830000)   Gid: (1000830000/ UNKNOWN)
Access: 2023-01-09 20:00:47.204123709 +0000
Modify: 2023-01-09 20:00:47.204123709 +0000
Change: 2023-01-09 20:04:56.808712867 +0000
 Birth: -
setfacl: data/lost+found: No such file or directory
setfacl: data/TESTDIR1: Cannot change owner/group: Operation not permitted
setfacl: data/TESTDIR1/file1: Cannot change owner/group: Operation not permitted
setfacl: data/TESTDIR2: Cannot change owner/group: Operation not permitted
setfacl: data/TESTDIR2/file3: Cannot change owner/group: Operation not permitted
setfacl: data/TESTDIR2/file2: Cannot change owner/group: Operation not permitted
setfacl: data/outfile: Cannot change owner/group: Operation not permitted
Rclone completed in 5s`

		expectedFilteredLog := `Transferred:         699.051 KiB / 699.051 KiB, 100%, 0 B/s, ETA -
Checks:                 5 / 5, 100%
Transferred:            1 / 1, 100%
Elapsed time:         5.0s
Rclone completed in 5s`

		It("Should filter the logs", func() {
			reader := strings.NewReader(destLog)
			filteredLines, err := utils.FilterLogs(reader, rclone.LogLineFilterSuccess)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Logs after filter", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredLog))
		})
	})
})
