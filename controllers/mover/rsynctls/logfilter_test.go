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

package rsynctls_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	rsynctls "github.com/backube/volsync/controllers/mover/rsynctls"
	"github.com/backube/volsync/controllers/utils"
)

var _ = Describe("RsyncTLS Filter Logs Tests", func() {
	logger := zap.New(zap.UseDevMode(true), zap.WriteTo(GinkgoWriter))

	Context("RsyncTLS source mover logs", func() {
		// Sample source log for rsync mover
		sourceLog := `2023.01.10 14:08:54 LOG7[0]: Deallocating application specific data for session connect address
2023.01.10 14:08:54 LOG7[0]: TLS state (connect): SSLv3/TLS read server hello
2023.01.10 14:08:54 LOG7[0]: TLS state (connect): TLSv1.3 read encrypted extensions
2023.01.10 14:08:54 LOG7[0]: TLS state (connect): SSLv3/TLS read finished
2023.01.10 14:08:54 LOG7[0]: TLS state (connect): SSLv3/TLS write change cipher spec
2023.01.10 14:08:54 LOG7[0]: TLS state (connect): SSLv3/TLS write finished
2023.01.10 14:08:54 LOG7[0]:      1 client connect(s) requested
2023.01.10 14:08:54 LOG7[0]:      1 client connect(s) succeeded
2023.01.10 14:08:54 LOG7[0]:      0 client renegotiation(s) requested
2023.01.10 14:08:54 LOG7[0]:      1 session reuse(s)
2023.01.10 14:08:54 LOG6[0]: TLS connected: previous session reused
2023.01.10 14:08:54 LOG6[0]: TLSv1.3 ciphersuite: TLS_AES_128_GCM_SHA256 (128-bit encryption)
2023.01.10 14:08:54 LOG6[0]: Peer temporary key: X25519, 253 bits
2023.01.10 14:08:54 LOG7[0]: Compression: null, expansion: null
2023.01.10 14:08:54 LOG6[0]: Session id: 
2023.01.10 14:08:54 LOG7[0]: TLS state (connect): SSL negotiation finished successfully
2023.01.10 14:08:54 LOG7[0]: TLS state (connect): SSL negotiation finished successfully
2023.01.10 14:08:54 LOG7[0]: Initializing application specific data for session authenticated
2023.01.10 14:08:54 LOG7[0]: Deallocating application specific data for session connect address
2023.01.10 14:08:54 LOG7[0]: New session callback
2023.01.10 14:08:54 LOG6[0]: No peer certificate received
2023.01.10 14:08:54 LOG6[0]: Session id: E98D33F73726E715081340727C7356DCC4274534654E3888C4D94D05985DBF09
2023.01.10 14:08:54 LOG7[0]: TLS state (connect): SSLv3/TLS read server session ticket
<f+++++++++ datafile

Number of files: 1 (reg: 1)
Number of created files: 1 (reg: 1)
Number of deleted files: 0
Number of regular files transferred: 1
Total file size: 5 bytes
Total transferred file size: 5 bytes
Literal data: 5 bytes
Matched data: 0 bytes
File list size: 0
File list generation time: 0.001 seconds
File list transfer time: 0.000 seconds
Total bytes sent: 125
Total bytes received: 35

sent 125 bytes  received 35 bytes  106.67 bytes/sec
total size is 5  speedup is 0.03
2023.01.10 14:08:54 LOG6[0]: Read socket closed (readsocket)
2023.01.10 14:08:54 LOG7[0]: Sending close_notify alert
2023.01.10 14:08:54 LOG7[0]: TLS alert (write): warning: close notify
2023.01.10 14:08:54 LOG6[0]: SSL_shutdown successfully sent close_notify alert
2023.01.10 14:08:54 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.10 14:08:54 LOG7[main]: FD=4 events=0x2001 revents=0x0
2023.01.10 14:08:54 LOG7[main]: FD=8 events=0x2001 revents=0x1
2023.01.10 14:08:54 LOG7[main]: Service [rsync] accepted (FD=11) from 127.0.0.1:55398
2023.01.10 14:08:54 LOG7[1]: Service [rsync] started
2023.01.10 14:08:54 LOG7[1]: Setting local socket options (FD=11)
2023.01.10 14:08:54 LOG7[1]: Option TCP_NODELAY set on local socket
2023.01.10 14:08:54 LOG5[1]: Service [rsync] accepted connection from 127.0.0.1:55398
2023.01.10 14:08:54 LOG6[1]: s_connect: connecting 172.32.226.217:8000
2023.01.10 14:08:54 LOG7[1]: s_connect: s_poll_wait 172.32.226.217:8000: waiting 10 seconds
2023.01.10 14:08:54 LOG7[1]: FD=6 events=0x2001 revents=0x0
2023.01.10 14:08:54 LOG7[1]: FD=12 events=0x2005 revents=0x0
2023.01.10 14:08:54 LOG5[1]: s_connect: connected 172.32.226.217:8000
2023.01.10 14:08:54 LOG5[1]: Service [rsync] connected remote server from 10.136.0.93:50052
2023.01.10 14:08:54 LOG7[1]: Setting remote socket options (FD=12)
2023.01.10 14:08:54 LOG7[1]: Option TCP_NODELAY set on remote socket
2023.01.10 14:08:54 LOG7[1]: Remote descriptor (FD=12) initialized
2023.01.10 14:08:54 LOG6[1]: SNI: sending servername: 172.32.226.217
2023.01.10 14:08:54 LOG6[1]: Peer certificate not required
2023.01.10 14:08:54 LOG7[1]: TLS state (connect): before SSL initialization
2023.01.10 14:08:54 LOG6[1]: PSK client configured for identity "volsync"
2023.01.10 14:08:54 LOG7[1]: Initializing application specific data for session authenticated
2023.01.10 14:08:54 LOG7[1]: TLS state (connect): SSLv3/TLS write client hello
2023.01.10 14:08:54 LOG7[1]: TLS state (connect): SSLv3/TLS write client hello
2023.01.10 14:08:54 LOG7[1]: Deallocating application specific data for session connect address
2023.01.10 14:08:54 LOG7[1]: TLS state (connect): SSLv3/TLS read server hello
2023.01.10 14:08:54 LOG7[1]: TLS state (connect): TLSv1.3 read encrypted extensions
2023.01.10 14:08:54 LOG7[1]: TLS state (connect): SSLv3/TLS read finished
2023.01.10 14:08:54 LOG7[1]: TLS state (connect): SSLv3/TLS write change cipher spec
2023.01.10 14:08:54 LOG7[1]: TLS state (connect): SSLv3/TLS write finished
2023.01.10 14:08:54 LOG7[1]: Remove session callback
2023.01.10 14:08:54 LOG7[1]:      2 client connect(s) requested
2023.01.10 14:08:54 LOG7[1]:      2 client connect(s) succeeded
2023.01.10 14:08:54 LOG7[1]:      0 client renegotiation(s) requested
2023.01.10 14:08:54 LOG7[1]:      2 session reuse(s)
2023.01.10 14:08:54 LOG6[1]: TLS connected: previous session reused
2023.01.10 14:08:54 LOG6[1]: TLSv1.3 ciphersuite: TLS_AES_128_GCM_SHA256 (128-bit encryption)
2023.01.10 14:08:54 LOG6[1]: Peer temporary key: X25519, 253 bits
2023.01.10 14:08:54 LOG7[1]: Compression: null, expansion: null
2023.01.10 14:08:54 LOG6[1]: Session id: E98D33F73726E715081340727C7356DCC4274534654E3888C4D94D05985DBF09
2023.01.10 14:08:54 LOG7[1]: TLS state (connect): SSL negotiation finished successfully
2023.01.10 14:08:54 LOG7[1]: TLS state (connect): SSL negotiation finished successfully
2023.01.10 14:08:54 LOG7[1]: Initializing application specific data for session authenticated
2023.01.10 14:08:54 LOG7[1]: New session callback
2023.01.10 14:08:54 LOG6[1]: No peer certificate received
2023.01.10 14:08:54 LOG6[1]: Session id: 0599D5E335DE950893CA6D42AA44E689E177A65A6A449C78A777FDFB3BCCC101
2023.01.10 14:08:54 LOG7[1]: TLS state (connect): SSLv3/TLS read server session ticket
2023.01.10 14:08:54 LOG7[0]: TLS alert (read): warning: close notify
2023.01.10 14:08:54 LOG6[0]: TLS closed (SSL_read)
2023.01.10 14:08:54 LOG7[0]: Sent socket write shutdown
2023.01.10 14:08:54 LOG5[0]: Connection closed: 256 byte(s) sent to TLS, 110 byte(s) sent to socket
2023.01.10 14:08:54 LOG7[0]: Deallocating application specific data for session connect address
2023.01.10 14:08:54 LOG7[0]: Remote descriptor (FD=10) closed
2023.01.10 14:08:54 LOG7[0]: Local descriptor (FD=3) closed
2023.01.10 14:08:54 LOG7[0]: Service [rsync] finished (1 left)

Number of files: 2 (reg: 1, dir: 1)
Number of created files: 0
Number of deleted files: 0
Number of regular files transferred: 0
Total file size: 5 bytes
Total transferred file size: 0 bytes
Literal data: 0 bytes
Matched data: 0 bytes
File list size: 0
File list generation time: 0.001 seconds
File list transfer time: 0.000 seconds
Total bytes sent: 82
Total bytes received: 19

sent 82 bytes  received 19 bytes  202.00 bytes/sec
total size is 5  speedup is 0.05
2023.01.10 14:08:54 LOG6[1]: Read socket closed (readsocket)
2023.01.10 14:08:54 LOG7[1]: Sending close_notify alert
2023.01.10 14:08:54 LOG7[1]: TLS alert (write): warning: close notify
2023.01.10 14:08:54 LOG6[1]: SSL_shutdown successfully sent close_notify alert
rsync completed in 1s
Sending shutdown to remote...
2023.01.10 14:08:54 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.10 14:08:54 LOG7[main]: FD=4 events=0x2001 revents=0x0
2023.01.10 14:08:54 LOG7[main]: FD=8 events=0x2001 revents=0x1
2023.01.10 14:08:54 LOG7[main]: Service [rsync] accepted (FD=3) from 127.0.0.1:55406
2023.01.10 14:08:54 LOG7[2]: Service [rsync] started
2023.01.10 14:08:54 LOG7[2]: Setting local socket options (FD=3)
2023.01.10 14:08:54 LOG7[2]: Option TCP_NODELAY set on local socket
2023.01.10 14:08:54 LOG5[2]: Service [rsync] accepted connection from 127.0.0.1:55406
2023.01.10 14:08:54 LOG6[2]: s_connect: connecting 172.32.226.217:8000
2023.01.10 14:08:54 LOG7[2]: s_connect: s_poll_wait 172.32.226.217:8000: waiting 10 seconds
2023.01.10 14:08:54 LOG7[2]: FD=6 events=0x2001 revents=0x0
2023.01.10 14:08:54 LOG7[2]: FD=10 events=0x2005 revents=0x0
2023.01.10 14:08:54 LOG5[2]: s_connect: connected 172.32.226.217:8000
2023.01.10 14:08:54 LOG5[2]: Service [rsync] connected remote server from 10.136.0.93:50060
2023.01.10 14:08:54 LOG7[2]: Setting remote socket options (FD=10)
2023.01.10 14:08:54 LOG7[2]: Option TCP_NODELAY set on remote socket
2023.01.10 14:08:54 LOG7[2]: Remote descriptor (FD=10) initialized
2023.01.10 14:08:54 LOG6[2]: SNI: sending servername: 172.32.226.217
2023.01.10 14:08:54 LOG6[2]: Peer certificate not required
2023.01.10 14:08:54 LOG7[2]: TLS state (connect): before SSL initialization
2023.01.10 14:08:54 LOG6[2]: PSK client configured for identity "volsync"
2023.01.10 14:08:54 LOG7[2]: Initializing application specific data for session authenticated
2023.01.10 14:08:54 LOG7[2]: TLS state (connect): SSLv3/TLS write client hello
2023.01.10 14:08:54 LOG7[2]: TLS state (connect): SSLv3/TLS write client hello
2023.01.10 14:08:54 LOG7[2]: Deallocating application specific data for session connect address
2023.01.10 14:08:54 LOG7[2]: TLS state (connect): SSLv3/TLS read server hello
2023.01.10 14:08:54 LOG7[2]: TLS state (connect): TLSv1.3 read encrypted extensions
2023.01.10 14:08:54 LOG7[2]: TLS state (connect): SSLv3/TLS read finished
2023.01.10 14:08:54 LOG7[2]: TLS state (connect): SSLv3/TLS write change cipher spec
2023.01.10 14:08:54 LOG7[2]: TLS state (connect): SSLv3/TLS write finished
2023.01.10 14:08:54 LOG7[2]: Remove session callback
2023.01.10 14:08:54 LOG7[2]:      3 client connect(s) requested
2023.01.10 14:08:54 LOG7[2]:      3 client connect(s) succeeded
2023.01.10 14:08:54 LOG7[2]:      0 client renegotiation(s) requested
2023.01.10 14:08:54 LOG7[2]:      3 session reuse(s)
2023.01.10 14:08:54 LOG6[2]: TLS connected: previous session reused
2023.01.10 14:08:54 LOG6[2]: TLSv1.3 ciphersuite: TLS_AES_128_GCM_SHA256 (128-bit encryption)
2023.01.10 14:08:54 LOG6[2]: Peer temporary key: X25519, 253 bits
2023.01.10 14:08:54 LOG7[2]: Compression: null, expansion: null
2023.01.10 14:08:54 LOG6[2]: Session id: 0599D5E335DE950893CA6D42AA44E689E177A65A6A449C78A777FDFB3BCCC101
2023.01.10 14:08:54 LOG7[2]: TLS state (connect): SSL negotiation finished successfully
2023.01.10 14:08:54 LOG7[2]: TLS state (connect): SSL negotiation finished successfully
2023.01.10 14:08:54 LOG7[2]: Initializing application specific data for session authenticated
2023.01.10 14:08:54 LOG7[2]: New session callback
2023.01.10 14:08:54 LOG6[2]: No peer certificate received
2023.01.10 14:08:54 LOG6[2]: Session id: 97D67227FD49C836E133187E87BF75645933060D4E9A26F29CB4B1349F32B314
2023.01.10 14:08:54 LOG7[2]: TLS state (connect): SSLv3/TLS read server session ticket
2023.01.10 14:08:54 LOG7[1]: TLS alert (read): warning: close notify
2023.01.10 14:08:54 LOG6[1]: TLS closed (SSL_read)
2023.01.10 14:08:54 LOG7[1]: Sent socket write shutdown
2023.01.10 14:08:54 LOG5[1]: Connection closed: 221 byte(s) sent to TLS, 69 byte(s) sent to socket
2023.01.10 14:08:54 LOG7[1]: Deallocating application specific data for session connect address
2023.01.10 14:08:54 LOG7[1]: Remote descriptor (FD=12) closed
2023.01.10 14:08:54 LOG7[1]: Local descriptor (FD=11) closed
2023.01.10 14:08:54 LOG7[1]: Service [rsync] finished (1 left)
...done
2023.01.10 14:08:54 LOG6[2]: Read socket closed (readsocket)
2023.01.10 14:08:54 LOG7[2]: Sending close_notify alert
2023.01.10 14:08:54 LOG7[2]: TLS alert (write): warning: close notify
2023.01.10 14:08:54 LOG6[2]: SSL_shutdown successfully sent close_notify alert
2023.01.10 14:08:54 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.10 14:08:54 LOG7[main]: FD=4 events=0x2001 revents=0x1
2023.01.10 14:08:54 LOG7[main]: FD=8 events=0x2001 revents=0x0
2023.01.10 14:08:54 LOG7[main]: Dispatching a signal from the signal pipe
2023.01.10 14:08:54 LOG7[main]: Processing SIGNAL_TERMINATE
2023.01.10 14:08:54 LOG5[main]: Terminated
2023.01.10 14:08:54 LOG7[main]: Leak detection table utilization: 107/997, 10.73%
2023.01.10 14:08:54 LOG7[main]: Removed pid file /tmp/stunnel-client.pid
2023.01.10 14:08:54 LOG7[main]: Terminating a thread for [rsync]
2023.01.10 14:08:54 LOG7[main]: Terminating the cron thread
2023.01.10 14:08:54 LOG6[main]: Terminating 2 service thread(s)`

		// nolint:lll
		expectedFilteredLog := `sent 125 bytes  received 35 bytes  106.67 bytes/sec
total size is 5  speedup is 0.03
sent 82 bytes  received 19 bytes  202.00 bytes/sec
total size is 5  speedup is 0.05
rsync completed in 1s`

		It("Should filter the logs", func() {
			reader := strings.NewReader(sourceLog)
			filteredLines, err := utils.FilterLogs(reader, rsynctls.LogLineFilterSuccess)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Filtered lines are", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredLog))
		})
	})

	Context("RsyncTLS source mover failure logs", func() {
		// sample source mover failure log
		// nolint:lll,misspell
		sourceFailedLog := `2023.01.20 16:03:15 LOG3[3]: No remote host resolved
2023.01.20 16:03:15 LOG5[3]: Connection reset: 0 byte(s) sent to TLS, 0 byte(s) sent to socket
2023.01.20 16:03:15 LOG7[3]: Local descriptor (FD=3) closed
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
2023.01.20 16:03:15 LOG7[3]: Service [rsync] finished (0 left)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
Syncronization failed. Retrying in 4 seconds. Retry 2/5.
2023.01.20 16:03:19 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.20 16:03:19 LOG7[main]: FD=4 events=0x2001 revents=0x0
2023.01.20 16:03:19 LOG7[main]: FD=8 events=0x2001 revents=0x1
2023.01.20 16:03:19 LOG7[main]: Service [rsync] accepted (FD=3) from 127.0.0.1:45340
2023.01.20 16:03:19 LOG7[4]: Service [rsync] started
2023.01.20 16:03:19 LOG7[4]: Setting local socket options (FD=3)
2023.01.20 16:03:19 LOG7[4]: Option TCP_NODELAY set on local socket
2023.01.20 16:03:19 LOG5[4]: Service [rsync] accepted connection from 127.0.0.1:45340
2023.01.20 16:03:19 LOG3[4]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
2023.01.20 16:03:19 LOG3[4]: No remote host resolved
2023.01.20 16:03:19 LOG5[4]: Connection reset: 0 byte(s) sent to TLS, 0 byte(s) sent to socket
2023.01.20 16:03:19 LOG7[4]: Local descriptor (FD=3) closed
2023.01.20 16:03:19 LOG7[4]: Service [rsync] finished (0 left)
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
2023.01.20 16:03:19 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.20 16:03:19 LOG7[main]: FD=4 events=0x2001 revents=0x0
2023.01.20 16:03:19 LOG7[main]: FD=8 events=0x2001 revents=0x1
2023.01.20 16:03:19 LOG7[main]: Service [rsync] accepted (FD=3) from 127.0.0.1:45358
2023.01.20 16:03:19 LOG7[5]: Service [rsync] started
2023.01.20 16:03:19 LOG7[5]: Setting local socket options (FD=3)
2023.01.20 16:03:19 LOG7[5]: Option TCP_NODELAY set on local socket
2023.01.20 16:03:19 LOG5[5]: Service [rsync] accepted connection from 127.0.0.1:45358
2023.01.20 16:03:19 LOG3[5]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
2023.01.20 16:03:19 LOG3[5]: No remote host resolved
2023.01.20 16:03:19 LOG5[5]: Connection reset: 0 byte(s) sent to TLS, 0 byte(s) sent to socket
2023.01.20 16:03:19 LOG7[5]: Local descriptor (FD=3) closed
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
2023.01.20 16:03:19 LOG7[5]: Service [rsync] finished (0 left)
Syncronization failed. Retrying in 8 seconds. Retry 3/5.
2023.01.20 16:03:27 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.20 16:03:27 LOG7[main]: FD=4 events=0x2001 revents=0x0
2023.01.20 16:03:27 LOG7[main]: FD=8 events=0x2001 revents=0x1
2023.01.20 16:03:27 LOG7[main]: Service [rsync] accepted (FD=3) from 127.0.0.1:45366
2023.01.20 16:03:27 LOG7[6]: Service [rsync] started
2023.01.20 16:03:27 LOG7[6]: Setting local socket options (FD=3)
2023.01.20 16:03:27 LOG7[6]: Option TCP_NODELAY set on local socket
2023.01.20 16:03:27 LOG5[6]: Service [rsync] accepted connection from 127.0.0.1:45366
2023.01.20 16:03:27 LOG3[6]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
2023.01.20 16:03:27 LOG3[6]: No remote host resolved
2023.01.20 16:03:27 LOG5[6]: Connection reset: 0 byte(s) sent to TLS, 0 byte(s) sent to socket
2023.01.20 16:03:27 LOG7[6]: Local descriptor (FD=3) closed
2023.01.20 16:03:27 LOG7[6]: Service [rsync] finished (0 left)
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
2023.01.20 16:03:27 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.20 16:03:27 LOG7[main]: FD=4 events=0x2001 revents=0x0
2023.01.20 16:03:27 LOG7[main]: FD=8 events=0x2001 revents=0x1
2023.01.20 16:03:27 LOG7[main]: Service [rsync] accepted (FD=3) from 127.0.0.1:45380
2023.01.20 16:03:27 LOG7[7]: Service [rsync] started
2023.01.20 16:03:27 LOG7[7]: Setting local socket options (FD=3)
2023.01.20 16:03:27 LOG7[7]: Option TCP_NODELAY set on local socket
2023.01.20 16:03:27 LOG5[7]: Service [rsync] accepted connection from 127.0.0.1:45380
2023.01.20 16:03:27 LOG3[7]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
2023.01.20 16:03:27 LOG3[7]: No remote host resolved
2023.01.20 16:03:27 LOG5[7]: Connection reset: 0 byte(s) sent to TLS, 0 byte(s) sent to socket
2023.01.20 16:03:27 LOG7[7]: Local descriptor (FD=3) closed
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
2023.01.20 16:03:27 LOG7[7]: Service [rsync] finished (0 left)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
Syncronization failed. Retrying in 16 seconds. Retry 4/5.
2023.01.20 16:03:43 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.20 16:03:43 LOG7[main]: FD=4 events=0x2001 revents=0x0
2023.01.20 16:03:43 LOG7[main]: FD=8 events=0x2001 revents=0x1
2023.01.20 16:03:43 LOG7[main]: Service [rsync] accepted (FD=3) from 127.0.0.1:43534
2023.01.20 16:03:43 LOG7[8]: Service [rsync] started
2023.01.20 16:03:43 LOG7[8]: Setting local socket options (FD=3)
2023.01.20 16:03:43 LOG7[8]: Option TCP_NODELAY set on local socket
2023.01.20 16:03:43 LOG5[8]: Service [rsync] accepted connection from 127.0.0.1:43534
2023.01.20 16:03:43 LOG3[8]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
2023.01.20 16:03:43 LOG3[8]: No remote host resolved
2023.01.20 16:03:43 LOG5[8]: Connection reset: 0 byte(s) sent to TLS, 0 byte(s) sent to socket
2023.01.20 16:03:43 LOG7[8]: Local descriptor (FD=3) closed
2023.01.20 16:03:43 LOG7[8]: Service [rsync] finished (0 left)
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
2023.01.20 16:03:43 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.20 16:03:43 LOG7[main]: FD=4 events=0x2001 revents=0x0
2023.01.20 16:03:43 LOG7[main]: FD=8 events=0x2001 revents=0x1
2023.01.20 16:03:43 LOG7[main]: Service [rsync] accepted (FD=3) from 127.0.0.1:43544
2023.01.20 16:03:43 LOG7[9]: Service [rsync] started
2023.01.20 16:03:43 LOG7[9]: Setting local socket options (FD=3)
2023.01.20 16:03:43 LOG7[9]: Option TCP_NODELAY set on local socket
2023.01.20 16:03:43 LOG5[9]: Service [rsync] accepted connection from 127.0.0.1:43544
2023.01.20 16:03:43 LOG3[9]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
2023.01.20 16:03:43 LOG3[9]: No remote host resolved
2023.01.20 16:03:43 LOG5[9]: Connection reset: 0 byte(s) sent to TLS, 0 byte(s) sent to socket
2023.01.20 16:03:43 LOG7[9]: Local descriptor (FD=3) closed
2023.01.20 16:03:43 LOG7[9]: Service [rsync] finished (0 left)
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
Syncronization failed. Retrying in 32 seconds. Retry 5/5.
rsync completed in 62s
Synchronization failed. rsync returned: 1212
2023.01.20 16:04:15 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.20 16:04:15 LOG7[main]: FD=4 events=0x2001 revents=0x1
`

		// nolint:lll,misspell
		expectedFilteredLog := `rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
Syncronization failed. Retrying in 4 seconds. Retry 2/5.
2023.01.20 16:03:19 LOG3[4]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
2023.01.20 16:03:19 LOG3[5]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
Syncronization failed. Retrying in 8 seconds. Retry 3/5.
2023.01.20 16:03:27 LOG3[6]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
2023.01.20 16:03:27 LOG3[7]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
Syncronization failed. Retrying in 16 seconds. Retry 4/5.
2023.01.20 16:03:43 LOG3[8]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
2023.01.20 16:03:43 LOG3[9]: Error resolving "volsync-rsync-dst-busybox-pvc2-gp3-csi.busybox-sample.svc.clusterset.local": Neither nodename nor servname known (EAI_NONAME)
rsync: [sender] safe_read failed to read 1 bytes: Connection reset by peer (104)
rsync error: error in rsync protocol data stream (code 12) at io.c(276) [sender=3.2.3]
Syncronization failed. Retrying in 32 seconds. Retry 5/5.
rsync completed in 62s
Synchronization failed. rsync returned: 1212`

		It("Should filter the logs", func() {
			reader := strings.NewReader(sourceFailedLog)
			filteredLines, err := utils.FilterLogs(reader, rsynctls.LogLineFilterFailure)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Filtered lines are", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredLog))
		})
	})

	Context("RsyncTLS dest mover logs", func() {
		// Sample dest log for volsync
		// nolint:lll
		destLog := `rsync  version 3.2.3  protocol version 31
Copyright (C) 1996-2020 by Andrew Tridgell, Wayne Davison, and others.
Web site: https://rsync.samba.org/
Capabilities:
    64-bit files, 64-bit inums, 64-bit timestamps, 64-bit long ints,
    socketpairs, hardlinks, hardlink-specials, symlinks, IPv6, atimes,
    batchfiles, inplace, append, ACLs, xattrs, optional protect-args, iconv,
    symtimes, prealloc, stop-at, no crtimes
Optimizations:
    SIMD, asm, openssl-crypto
Checksum list:
    md5 md4 none
Compress list:
    zstd lz4 zlibx zlib none

rsync comes with ABSOLUTELY NO WARRANTY.  This is free software, and you
are welcome to redistribute it under certain conditions.  See the GNU
General Public Licence for details.
Initializing inetd mode configuration
stunnel 5.62 on x86_64-redhat-linux-gnu platform
Compiled/running with OpenSSL 3.0.1 14 Dec 2021
Threading:PTHREAD Sockets:POLL,IPv6 TLS:ENGINE,FIPS,OCSP,PSK,SNI
 
Global options:
fips                   = no
RNDbytes               = 1024
RNDfile                = /dev/urandom
RNDoverwrite           = yes
 
Service-level options:
ciphers                = FIPS (with "fips = yes")
ciphers                = PROFILE=SYSTEM (with "fips = no")
ciphersuites           = TLS_AES_256_GCM_SHA384:TLS_AES_128_GCM_SHA256:TLS_CHACHA20_POLY1305_SHA256 (with TLSv1.3)
curves                 = P-256:P-521:P-384 (with "fips = yes")
curves                 = X25519:P-256:X448:P-521:P-384 (with "fips = no")
debug                  = daemon.notice
logId                  = sequential
options                = NO_SSLv2
options                = NO_SSLv3
securityLevel          = 2
sessionCacheSize       = 1000
sessionCacheTimeout    = 300 seconds
stack                  = 65536 bytes
TIMEOUTbusy            = 300 seconds
TIMEOUTclose           = 60 seconds
TIMEOUTconnect         = 10 seconds
TIMEOUTidle            = 43200 seconds
verify                 = none
Starting stunnel...
2023.01.10 14:07:55 LOG6[ui]: Initializing inetd mode configuration
2023.01.10 14:07:55 LOG7[ui]: Clients allowed=512000
2023.01.10 14:07:55 LOG5[ui]: stunnel 5.62 on x86_64-redhat-linux-gnu platform
2023.01.10 14:07:55 LOG5[ui]: Compiled/running with OpenSSL 3.0.1 14 Dec 2021
2023.01.10 14:07:55 LOG5[ui]: Threading:PTHREAD Sockets:POLL,IPv6 TLS:ENGINE,FIPS,OCSP,PSK,SNI
2023.01.10 14:07:55 LOG7[ui]: errno: (*__errno_location ())
2023.01.10 14:07:55 LOG6[ui]: Initializing inetd mode configuration
2023.01.10 14:07:55 LOG5[ui]: Reading configuration from file /tmp/stunnel.conf
2023.01.10 14:07:55 LOG5[ui]: UTF-8 byte order mark not detected
2023.01.10 14:07:55 LOG5[ui]: FIPS mode disabled
2023.01.10 14:07:55 LOG6[ui]: Compression enabled: 0 methods
2023.01.10 14:07:55 LOG7[ui]: No PRNG seeding was required
2023.01.10 14:07:55 LOG6[ui]: Initializing service [rsync]
2023.01.10 14:07:55 LOG6[ui]: PSKsecrets line 1: 64-byte hexadecimal key configured for identity "volsync"
2023.01.10 14:07:55 LOG6[ui]: PSK identities: 1 retrieved
2023.01.10 14:07:55 LOG6[ui]: Using the default TLS version as specified in OpenSSL crypto policies. Not setting explicitly.
2023.01.10 14:07:55 LOG6[ui]: Using the default TLS version as specified in OpenSSL crypto policies. Not setting explicitly
2023.01.10 14:07:55 LOG6[ui]: OpenSSL security level is used: 2
2023.01.10 14:07:55 LOG7[ui]: Ciphers: PSK
2023.01.10 14:07:55 LOG7[ui]: TLSv1.3 ciphersuites: TLS_AES_256_GCM_SHA384:TLS_AES_128_GCM_SHA256:TLS_CHACHA20_POLY1305_SHA256
2023.01.10 14:07:55 LOG7[ui]: TLS options: 0x2100000 (+0x0, -0x0)
2023.01.10 14:07:55 LOG6[ui]: Session resumption enabled
2023.01.10 14:07:55 LOG7[ui]: No certificate or private key specified
2023.01.10 14:07:55 LOG6[ui]: DH initialization needed for DHE-PSK-AES256-GCM-SHA384
2023.01.10 14:07:55 LOG7[ui]: DH initialization
2023.01.10 14:07:55 LOG7[ui]: No certificate available to load DH parameters
2023.01.10 14:07:55 LOG6[ui]: Using dynamic DH parameters
2023.01.10 14:07:55 LOG7[ui]: ECDH initialization
2023.01.10 14:07:55 LOG7[ui]: ECDH initialized with curves X25519:P-256:X448:P-521:P-384
2023.01.10 14:07:55 LOG5[ui]: Configuration successful
2023.01.10 14:07:55 LOG7[ui]: Deallocating deployed section defaults
2023.01.10 14:07:55 LOG7[ui]: Binding service [rsync]
2023.01.10 14:07:55 LOG7[ui]: Listening file descriptor created (FD=8)
2023.01.10 14:07:55 LOG7[ui]: Setting accept socket options (FD=8)
2023.01.10 14:07:55 LOG7[ui]: Option SO_REUSEADDR set on accept socket
2023.01.10 14:07:55 LOG6[ui]: Service [rsync] (FD=8) bound to :::8000
2023.01.10 14:07:55 LOG7[main]: Created pid file /tmp/stunnel.pid
2023.01.10 14:07:55 LOG7[cron]: Cron thread initialized
2023.01.10 14:07:55 LOG6[cron]: Executing cron jobs
2023.01.10 14:07:55 LOG5[cron]: Updating DH parameters
Waiting for control file to be created (/tmp/control/complete)...
2023.01.10 14:08:54 LOG7[0]: Deallocating application specific data for session connect address
2023.01.10 14:08:54 LOG7[0]: Remote descriptor (FD=12) closed
2023.01.10 14:08:54 LOG7[0]: Local descriptor (FD=3) closed
2023.01.10 14:08:54 LOG7[0]: Service [rsync] finished (1 left)
2023/01/10 14:08:54 [87] rsync to data/ from UNDETERMINED (::ffff:10.136.0.93)
2023/01/10 14:08:54 [87] receiving file list
2023.01.10 14:08:54 LOG7[1]: TLS alert (read): warning: close notify
2023.01.10 14:08:54 LOG6[1]: TLS closed (SSL_read)
2023.01.10 14:08:54 LOG7[1]: Sent socket write shutdown
2023.01.10 14:08:54 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.10 14:08:54 LOG7[main]: FD=4 events=0x2001 revents=0x0
2023.01.10 14:08:54 LOG7[main]: FD=8 events=0x2001 revents=0x1
2023.01.10 14:08:54 LOG7[main]: Service [rsync] accepted (FD=3) from ::ffff:10.136.0.93:50060
2023.01.10 14:08:54 LOG7[2]: Service [rsync] started
2023.01.10 14:08:54 LOG7[2]: Setting local socket options (FD=3)
2023.01.10 14:08:54 LOG7[2]: Option TCP_NODELAY set on local socket
2023.01.10 14:08:54 LOG5[2]: Service [rsync] accepted connection from ::ffff:10.136.0.93:50060
2023.01.10 14:08:54 LOG6[2]: Peer certificate not required
2023.01.10 14:08:54 LOG7[2]: TLS state (accept): before SSL initialization
2023.01.10 14:08:54 LOG7[2]: TLS state (accept): before SSL initialization
2023.01.10 14:08:54 LOG6[2]: PSK identity not found (session resumption?)
2023.01.10 14:08:54 LOG7[2]: Initializing application specific data for session authenticated
2023.01.10 14:08:54 LOG7[2]: Decrypt session ticket callback
2023.01.10 14:08:54 LOG6[2]: Decrypted ticket for an authenticated session: yes
2023.01.10 14:08:54 LOG7[2]: SNI: no virtual services defined
2023.01.10 14:08:54 LOG7[2]: TLS state (accept): SSLv3/TLS read client hello
2023.01.10 14:08:54 LOG7[2]: TLS state (accept): SSLv3/TLS write server hello
2023.01.10 14:08:54 LOG7[2]: TLS state (accept): SSLv3/TLS write change cipher spec
2023.01.10 14:08:54 LOG7[2]: TLS state (accept): TLSv1.3 write encrypted extensions
2023.01.10 14:08:54 LOG7[2]: TLS state (accept): SSLv3/TLS write finished
2023.01.10 14:08:54 LOG7[2]: TLS state (accept): TLSv1.3 early data
2023.01.10 14:08:54 LOG7[2]: TLS state (accept): TLSv1.3 early data
2023.01.10 14:08:54 LOG7[2]: TLS state (accept): SSLv3/TLS read finished
2023.01.10 14:08:54 LOG7[2]:      3 server accept(s) requested
2023.01.10 14:08:54 LOG7[2]:      3 server accept(s) succeeded
2023.01.10 14:08:54 LOG7[2]:      0 server renegotiation(s) requested
2023.01.10 14:08:54 LOG7[2]:      3 session reuse(s)
2023.01.10 14:08:54 LOG7[2]:      0 internal session cache item(s)
2023.01.10 14:08:54 LOG7[2]:      0 internal session cache fill-up(s)
2023.01.10 14:08:54 LOG7[2]:      0 internal session cache miss(es)
2023.01.10 14:08:54 LOG7[2]:      0 external session cache hit(s)
2023.01.10 14:08:54 LOG7[2]:      0 expired session(s) retrieved
2023.01.10 14:08:54 LOG7[2]: Initializing application specific data for session authenticated
2023.01.10 14:08:54 LOG7[2]: Deallocating application specific data for session connect address
2023.01.10 14:08:54 LOG7[2]: Generate session ticket callback
2023.01.10 14:08:54 LOG7[2]: Initializing application specific data for session authenticated
2023.01.10 14:08:54 LOG7[2]: Deallocating application specific data for session connect address
2023.01.10 14:08:54 LOG7[2]: New session callback
2023.01.10 14:08:54 LOG6[2]: No peer certificate received
2023.01.10 14:08:54 LOG6[2]: Session id: F44019EEAE51880B18C88C2D3E41A4D8605CDDD7911D4A29C9CB7F8C0812E29F
2023.01.10 14:08:54 LOG7[2]: TLS state (accept): SSLv3/TLS write session ticket
2023.01.10 14:08:54 LOG6[2]: TLS accepted: previous session reused
2023.01.10 14:08:54 LOG6[2]: TLSv1.3 ciphersuite: TLS_AES_128_GCM_SHA256 (128-bit encryption)
2023.01.10 14:08:54 LOG6[2]: Peer temporary key: X25519, 253 bits
2023.01.10 14:08:54 LOG7[2]: Compression: null, expansion: null
2023.01.10 14:08:54 LOG6[2]: Session id: F44019EEAE51880B18C88C2D3E41A4D8605CDDD7911D4A29C9CB7F8C0812E29F
2023.01.10 14:08:54 LOG6[2]: Local mode child started (PID=91)
2023.01.10 14:08:54 LOG7[2]: Setting remote socket options (FD=13)
2023.01.10 14:08:54 LOG7[2]: Option TCP_NODELAY set on remote socket
2023.01.10 14:08:54 LOG7[2]: Remote descriptor (FD=13) initialized
2023/01/10 14:08:54 [87] sent 24 bytes  received 90 bytes  total size 5
2023.01.10 14:08:54 LOG6[1]: Read socket closed (readsocket)
2023.01.10 14:08:54 LOG7[1]: Sending close_notify alert
2023.01.10 14:08:54 LOG7[1]: TLS alert (write): warning: close notify
2023.01.10 14:08:54 LOG6[1]: SSL_shutdown successfully sent close_notify alert
2023.01.10 14:08:54 LOG5[1]: Connection closed: 69 byte(s) sent to TLS, 221 byte(s) sent to socket
2023.01.10 14:08:54 LOG7[1]: Deallocating application specific data for session connect address
2023.01.10 14:08:54 LOG7[1]: Remote descriptor (FD=14) closed
2023.01.10 14:08:54 LOG7[1]: Local descriptor (FD=10) closed
2023.01.10 14:08:54 LOG7[1]: Service [rsync] finished (1 left)
2023.01.10 14:08:54 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.10 14:08:54 LOG7[main]: FD=4 events=0x2001 revents=0x1
2023.01.10 14:08:54 LOG7[main]: FD=8 events=0x2001 revents=0x0
2023.01.10 14:08:54 LOG7[main]: Dispatching a signal from the signal pipe
2023.01.10 14:08:54 LOG7[main]: Processing SIGCHLD
2023.01.10 14:08:54 LOG7[main]: Retrieving pid statuses with waitpid()
2023.01.10 14:08:54 LOG6[main]: Child process 87 finished with code 0
2023/01/10 14:08:54 [91] rsync to control/complete from UNDETERMINED (::ffff:10.136.0.93)
2023/01/10 14:08:54 [91] receiving file list
2023/01/10 14:08:54 [91] recv UNDETERMINED [::ffff:10.136.0.93] control () client.sh 3087
2023.01.10 14:08:54 LOG7[2]: TLS alert (read): warning: close notify
2023.01.10 14:08:54 LOG6[2]: TLS closed (SSL_read)
2023.01.10 14:08:54 LOG7[2]: Sent socket write shutdown
2023/01/10 14:08:54 [91] sent 40 bytes  received 3181 bytes  total size 3087
2023.01.10 14:08:54 LOG6[2]: Read socket closed (readsocket)
2023.01.10 14:08:54 LOG7[2]: Sending close_notify alert
2023.01.10 14:08:54 LOG7[2]: TLS alert (write): warning: close notify
2023.01.10 14:08:54 LOG6[2]: SSL_shutdown successfully sent close_notify alert
2023.01.10 14:08:54 LOG5[2]: Connection closed: 85 byte(s) sent to TLS, 3249 byte(s) sent to socket
2023.01.10 14:08:54 LOG7[2]: Deallocating application specific data for session connect address
2023.01.10 14:08:54 LOG7[2]: Remote descriptor (FD=13) closed
2023.01.10 14:08:54 LOG7[2]: Local descriptor (FD=3) closed
2023.01.10 14:08:54 LOG7[2]: Service [rsync] finished (0 left)
2023.01.10 14:08:54 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.10 14:08:54 LOG7[main]: FD=4 events=0x2001 revents=0x1
2023.01.10 14:08:54 LOG7[main]: FD=8 events=0x2001 revents=0x0
2023.01.10 14:08:54 LOG7[main]: Dispatching a signal from the signal pipe
2023.01.10 14:08:54 LOG7[main]: Processing SIGCHLD
2023.01.10 14:08:54 LOG7[main]: Retrieving pid statuses with waitpid()
2023.01.10 14:08:54 LOG6[main]: Child process 91 finished with code 0
Shutting down...
2023.01.10 14:08:55 LOG7[main]: Found 1 ready file descriptor(s)
2023.01.10 14:08:55 LOG7[main]: FD=4 events=0x2001 revents=0x1
2023.01.10 14:08:55 LOG7[main]: FD=8 events=0x2001 revents=0x0
2023.01.10 14:08:55 LOG7[main]: Dispatching a signal from the signal pipe
2023.01.10 14:08:55 LOG7[main]: Processing SIGNAL_TERMINATE
2023.01.10 14:08:55 LOG5[main]: Terminated
2023.01.10 14:08:55 LOG7[main]: Leak detection table utilization: 128/997, 12.84%
2023.01.10 14:08:55 LOG7[main]: Removed pid file /tmp/stunnel.pid
2023.01.10 14:08:55 LOG7[main]: Terminating the cron thread
2023.01.10 14:08:55 LOG6[main]: Terminating 1 service thread(s)
2023.01.10 14:08:55 LOG6[main]: Service threads terminated
2023.01.10 14:08:55 LOG7[main]: Unbinding service [rsync]
2023.01.10 14:08:55 LOG7[main]: Service [rsync] closed (FD=8)
2023.01.10 14:08:55 LOG7[main]: Service [rsync] closed`

		expectedFilteredLog := `2023/01/10 14:08:54 [87] sent 24 bytes  received 90 bytes  total size 5
2023/01/10 14:08:54 [91] sent 40 bytes  received 3181 bytes  total size 3087`

		It("Should filter the logs", func() {
			reader := strings.NewReader(destLog)
			filteredLines, err := utils.FilterLogs(reader, rsynctls.LogLineFilterSuccess)
			Expect(err).NotTo(HaveOccurred())

			logger.Info("Filtered lines are", "filteredLines", filteredLines)
			Expect(filteredLines).To(Equal(expectedFilteredLog))
		})
	})
})
