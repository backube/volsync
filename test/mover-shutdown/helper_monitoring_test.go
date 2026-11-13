package mover_shutdown

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// helperFailureTimeout bounds how long an entrypoint may take to stop itself
// after a helper fails. The helper startup window in shutdown.sh is about 0.2s
// and the server control-file wait loop polls once per second, so an
// entrypoint that stops itself exits well within this bound. Only a hang, the
// bug these specs guard against, exceeds it.
const helperFailureTimeout = 10 * time.Second

// tlsDestinationAddress is the address the TLS client connects to. The fake
// stunnel never opens a connection, so the value only needs to be syntactically
// valid. A constant because the package already uses the literal twice.
const tlsDestinationAddress = "127.0.0.1"

// serverControlFile is the file the TLS server waits for before it finishes.
// The specs run the entrypoints in a PID namespace only, so the entrypoint
// shares the host /tmp with the test process and this path is visible to both.
const serverControlFile = "/tmp/control/complete"

// exitingHelperFake answers the -version probe, records that it started, then
// exits with status after delay. A delay longer than the helper startup window
// in shutdown.sh (about 0.2s) turns the exit into a mid-run failure that only
// volsync_check_helpers can detect; a short delay looks like a startup failure
// caught by volsync_start_helper.
func exitingHelperFake(name string, delay time.Duration, status int) string {
	return fmt.Sprintf(`if [[ "${1:-}" == "-version" ]]; then
    echo "fake %s version"
    exit 0
fi
echo started > "${FAKE_MARKER_DIR}/%s-started"
sleep %g
exit %d
`, name, name, delay.Seconds(), status)
}

// rsyncFailingFake answers the --version probe and fails every transfer
// invocation immediately, so a spec can prove the retry loop never started.
func rsyncFailingFake() string {
	return `if [[ "${1:-}" == "--version" ]]; then
    echo "fake rsync version"
    exit 0
fi
exit 1
`
}

// rsyncCountingFailingFake answers the --version probe, records every transfer
// invocation in the rsync-invocations marker, and fails immediately.
func rsyncCountingFailingFake() string {
	return `if [[ "${1:-}" == "--version" ]]; then
    echo "fake rsync version"
    exit 0
fi
echo invocation >> "${FAKE_MARKER_DIR}/rsync-invocations"
exit 1
`
}

// tlsFixtureEnv creates the data and key directories the TLS mover scripts
// require and returns the environment entries pointing at them.
func tlsFixtureEnv(sb *sandbox) map[string]string {
	dataDir := sb.dir("data")
	keyDir := sb.dir("keys")
	Expect(os.WriteFile(filepath.Join(keyDir, "psk.txt"), []byte("psk"), 0o600)).To(Succeed())
	return map[string]string{
		envVolsyncDataDir:  dataDir,
		envVolsyncKeyDir:   keyDir,
		envPrivilegedMover: "1",
	}
}

// runTLSServerToCompletion runs the TLS server entrypoint with the standard
// server fakes (rsync version probe, blocking tail) and the given stunnel
// fake, without signalling it.
func runTLSServerToCompletion(stunnelBody string) runResult {
	sb := newSandbox()
	sb.writeFake(fakeStunnel, stunnelBody)
	sb.writeFake(fakeTail, blockingFakeBody(fakeTail))
	sb.writeFake(fakeRsync, rsyncVersionFake())
	return sb.runEntrypointToCompletion(
		filepath.Join(repositoryRoot, "mover-rsync-tls", "server.sh"),
		nil,
		tlsFixtureEnv(sb))
}

// expectHelperFailure asserts the entrypoint stopped itself quickly with a
// failure instead of waiting for a signal or hanging.
func expectHelperFailure(result runResult) {
	Expect(result.exitCode).NotTo(Equal(0), "mover output:\n%s", result.output)
	Expect(result.elapsed).To(BeNumerically("<", helperFailureTimeout),
		"the mover hung instead of stopping itself\nmover output:\n%s", result.output)
}

var _ = Describe("Mover helper monitoring", func() {
	BeforeEach(func() {
		requirePIDNamespaces()
	})

	It("fails the TLS server when stunnel cannot start", func() {
		result := runTLSServerToCompletion(exitingHelperFake(fakeStunnel, 0, 2))

		expectHelperFailure(result)
		Expect(result.output).To(ContainSubstring("helper failed to start: stunnel"),
			"the failure must name the helper that could not start\nmover output:\n%s", result.output)
	})

	It("fails the TLS server when a critical helper exits after startup", func() {
		// The delay must outlast both the helper startup window (about 0.2s)
		// and the first wait-loop cycle, which sleeps 1s before it checks
		// helpers, so only volsync_check_helpers can detect the exit.
		result := runTLSServerToCompletion(exitingHelperFake(fakeStunnel, 2*time.Second, 4))

		expectHelperFailure(result)
		Expect(result.output).To(ContainSubstring("helper exited unexpectedly"))
		Expect(result.output).To(ContainSubstring("ERROR: a required helper exited before the transfer completed"))
	})

	It("fails the TLS client when stunnel cannot start", func() {
		sb := newSandbox()
		sb.writeFake(fakeStunnel, exitingHelperFake(fakeStunnel, 0, 2))
		sb.writeFake(fakeRsync, rsyncFailingFake())
		env := tlsFixtureEnv(sb)
		env["DESTINATION_ADDRESS"] = tlsDestinationAddress
		env["DESTINATION_PORT"] = "8000"

		result := sb.runEntrypointToCompletion(
			filepath.Join(repositoryRoot, "mover-rsync-tls", "client.sh"),
			nil,
			env)

		expectHelperFailure(result)
		Expect(result.output).To(ContainSubstring("helper failed to start: stunnel"),
			"the failure must name the helper that could not start\nmover output:\n%s", result.output)
		Expect(result.output).NotTo(ContainSubstring("Retrying"),
			"the retry loop must not start when stunnel is gone\nmover output:\n%s", result.output)
	})

	// Serial: client.sh keeps its file list at a fixed /tmp path. The other
	// TLS client spec fills that file with an entry while it runs, which
	// would make this spec's empty source look non-empty and add a transfer
	// pass when Ginkgo runs the suite's specs in parallel (make test uses -p).
	It("stops retrying the TLS client when stunnel exits after startup", Serial, func() {
		sb := newSandbox()
		sb.writeFake(fakeStunnel, exitingHelperFake(fakeStunnel, 1500*time.Millisecond, 4))
		sb.writeFake(fakeRsync, rsyncCountingFailingFake())
		env := tlsFixtureEnv(sb)
		env["DESTINATION_ADDRESS"] = tlsDestinationAddress
		env["DESTINATION_PORT"] = "8000"

		result := sb.runEntrypointToCompletion(
			filepath.Join(repositoryRoot, "mover-rsync-tls", "client.sh"),
			nil,
			env)

		expectHelperFailure(result)
		Expect(result.output).To(ContainSubstring("helper exited unexpectedly"))
		invocations, err := os.ReadFile(filepath.Join(sb.markersDir, "rsync-invocations"))
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Fields(string(invocations))).To(HaveLen(1),
			"the client started a second transfer attempt after stunnel exited\nmover output:\n%s", result.output)
	})

	// Serial: this spec creates the server control file in the shared /tmp,
	// which would make concurrently running server specs finish early when
	// Ginkgo runs the suite's specs in parallel (make test uses -p).
	It("keeps the TLS server running when a noncritical helper exits", Serial, func() {
		sb := newSandbox()
		sb.writeFake(fakeTail, exitingHelperFake(fakeTail, 0, 2))
		sb.writeFake(fakeStunnel, stunnelFake())
		sb.writeFake(fakeRsync, rsyncVersionFake())

		// Remove a control file left behind by an earlier run so this spec
		// starts clean, and remove this spec's file again so no later server
		// spec sees it.
		Expect(os.RemoveAll(serverControlFile)).To(Succeed())
		DeferCleanup(func() {
			Expect(os.RemoveAll(serverControlFile)).To(Succeed())
		})

		ep := sb.startEntrypoint(
			filepath.Join(repositoryRoot, "mover-rsync-tls", "server.sh"),
			nil,
			tlsFixtureEnv(sb))
		ep.waitForMarker(fakeStunnel + "-started")

		// The server removes the control file before it starts stunnel, so
		// creating it after the stunnel-started marker cannot be undone by
		// the entrypoint. The server then sleeps about 5s before it exits.
		Expect(os.MkdirAll(filepath.Dir(serverControlFile), 0o750)).To(Succeed())
		Expect(os.WriteFile(serverControlFile, []byte("done"), 0o600)).To(Succeed())

		ep.waitToExit(fmt.Sprintf(
			"server did not exit within %s after the control file was created", exitTimeout))
		result := ep.result()

		Expect(result.exitCode).To(Equal(0),
			"the server should finish normally despite the tail failure\nmover output:\n%s", result.output)
		Expect(result.output).To(ContainSubstring("noncritical helper exited"))
		Expect(result.output).NotTo(ContainSubstring("ERROR: a required helper"))
	})
})
