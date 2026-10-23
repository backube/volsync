package mover_shutdown

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	envPrivilegedMover = "PRIVILEGED_MOVER"
	envVolsyncDataDir  = "VOLSYNC_DATA_DIR"
	envVolsyncKeyDir   = "VOLSYNC_KEY_DIR"

	fakeRestic  = "restic"
	fakeRclone  = "rclone"
	fakeRsync   = "rsync"
	fakeStunnel = "stunnel"
	fakeTail    = "tail"
	fakeSSHD    = "sshd"
)

func resticFake() string {
	return `if [[ "${1:-}" == "cat" ]]; then
    exit 10
fi
if [[ "${1:-}" != "backup" ]]; then
    exit 0
fi
` + blockingFakeBody(fakeRestic)
}

func rcloneFake() string {
	return `if [[ "${1:-}" != "sync" ]]; then
    exit 0
fi
` + blockingFakeBody(fakeRclone)
}

func stunnelFake() string {
	return `if [[ "${1:-}" == "-version" ]]; then
    echo "fake stunnel version"
    exit 0
fi
` + blockingFakeBody(fakeStunnel)
}

// rsyncTransferFake blocks for every invocation except --version and records
// each blocking invocation so a spec can prove no retry pass ran.
func rsyncTransferFake() string {
	return `if [[ "${1:-}" == "--version" ]]; then
    echo "fake rsync version"
    exit 0
fi
echo invocation >> "${FAKE_MARKER_DIR}/rsync-invocations"
` + blockingFakeBody(fakeRsync)
}

func rsyncVersionFake() string {
	return `echo "fake rsync version"
exit 0
`
}

var _ = Describe("Mover shutdown signal forwarding", func() {
	BeforeEach(func() {
		requirePIDNamespaces()
	})

	It("forwards SIGTERM to the running restic backup and waits for cleanup", func() {
		sb := newSandbox()
		sb.writeFake(fakeRestic, resticFake())
		dataDir := sb.dir("data")
		Expect(os.WriteFile(filepath.Join(dataDir, "file.txt"), []byte("data"), 0o600)).To(Succeed())

		result := sb.runEntrypoint(
			filepath.Join(repositoryRoot, "mover-restic", "entry.sh"),
			[]string{"backup"},
			map[string]string{
				envPrivilegedMover:  "1",
				"RESTIC_CACHE_DIR":  filepath.Join(sb.root, "cache"),
				"RESTIC_PASSWORD":   "x", // #nosec G101 -- fake credential for the test mover.
				"RESTIC_REPOSITORY": filepath.Join(sb.root, "repo"),
				"DATA_DIR":          dataDir,
				"FORGET_OPTIONS":    "",
			},
			"restic-started",
		)

		expectForwardedAndCleanedUp(sb, result, 143, fakeRestic)
	})

	It("forwards SIGTERM to the running rclone sync and waits for cleanup", func() {
		sb := newSandbox()
		sb.writeFake(fakeRclone, rcloneFake())
		sb.writeFake("getfacl", "cat > /dev/null\nexit 0\n")
		mountPath := sb.dir("data")
		Expect(os.WriteFile(filepath.Join(mountPath, "file.txt"), []byte("data"), 0o600)).To(Succeed())

		result := sb.runEntrypoint(
			filepath.Join(repositoryRoot, "mover-rclone", "active.sh"),
			nil,
			map[string]string{
				envPrivilegedMover:      "1",
				"RCLONE_DEST_PATH":      "/dest",
				"DIRECTION":             "source",
				"MOUNT_PATH":            mountPath,
				"RCLONE_CONFIG_SECTION": "remote",
			},
			"rclone-started",
		)

		expectForwardedAndCleanedUp(sb, result, 143, fakeRclone)
	})

	It("forwards SIGTERM to the running rsync source transfer and waits for cleanup", func() {
		sb := newSandbox()
		sb.writeFake(fakeRsync, blockingFakeBody(fakeRsync))
		sb.writeFake("ssh", "exit 0\n")
		dataDir := sb.dir("data")
		Expect(os.WriteFile(filepath.Join(dataDir, "file.txt"), []byte("data"), 0o600)).To(Succeed())
		keyDir := sb.dir("keys")
		Expect(os.WriteFile(filepath.Join(keyDir, "destination.pub"), []byte("ssh-ed25519 AAAA"), 0o600)).
			To(Succeed())
		Expect(os.WriteFile(filepath.Join(keyDir, "source"), []byte("private"), 0o600)).To(Succeed())

		result := sb.runEntrypoint(
			filepath.Join(repositoryRoot, "mover-rsync", "source.sh"),
			nil,
			map[string]string{
				"DESTINATION_ADDRESS": "127.0.0.1",
				"DESTINATION_PORT":    "8022",
				envVolsyncDataDir:     dataDir,
				envVolsyncKeyDir:      keyDir,
				envPrivilegedMover:    "1",
			},
			"rsync-started",
		)

		expectForwardedAndCleanedUp(sb, result, 143, fakeRsync)
	})

	It("forwards SIGTERM to sshd on the rsync destination and reports the interruption", func() {
		sb := newSandbox()
		sshd := sb.writeFake("fake-sshd", blockingFakeBody(fakeSSHD))
		dataDir := sb.dir("data")
		keyDir := sb.dir("keys")
		Expect(os.WriteFile(filepath.Join(keyDir, "source.pub"), []byte("ssh-ed25519 AAAA"), 0o600)).
			To(Succeed())

		result := sb.runEntrypoint(
			filepath.Join(repositoryRoot, "mover-rsync", "destination.sh"),
			nil,
			map[string]string{
				envVolsyncDataDir:  dataDir,
				envVolsyncKeyDir:   keyDir,
				"VOLSYNC_SSHD_BIN": sshd,
				envPrivilegedMover: "1",
			},
			"sshd-started",
		)

		// The destination script hard-codes 255 (transfer did not complete).
		expectForwardedAndCleanedUp(sb, result, 255, fakeSSHD)
		Expect(result.output).To(ContainSubstring("sshd stopped by SIGTERM"))
		Expect(result.output).NotTo(ContainSubstring("Exit code: 0"))
	})

	It("forwards SIGTERM to the in-flight rsync transfer and stops stunnel on exit", func() {
		sb := newSandbox()
		sb.writeFake(fakeStunnel, stunnelFake())
		sb.writeFake(fakeRsync, rsyncTransferFake())
		dataDir := sb.dir("data")
		Expect(os.WriteFile(filepath.Join(dataDir, "file.txt"), []byte("data"), 0o600)).To(Succeed())
		keyDir := sb.dir("keys")
		Expect(os.WriteFile(filepath.Join(keyDir, "psk.txt"), []byte("psk"), 0o600)).To(Succeed())

		result := sb.runEntrypoint(
			filepath.Join(repositoryRoot, "mover-rsync-tls", "client.sh"),
			nil,
			map[string]string{
				"DESTINATION_ADDRESS": "127.0.0.1",
				"DESTINATION_PORT":    "8000",
				envVolsyncDataDir:     dataDir,
				envVolsyncKeyDir:      keyDir,
				envPrivilegedMover:    "1",
			},
			"rsync-started",
		)

		expectForwardedAndCleanedUp(sb, result, 143, fakeRsync, fakeStunnel)

		invocations, err := os.ReadFile(filepath.Join(sb.markersDir, "rsync-invocations"))
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.Fields(string(invocations))).To(HaveLen(1),
			"a retry pass started after shutdown was requested\noutput:\n%s", result.output)
		Expect(result.output).NotTo(ContainSubstring("Retrying"))
		Expect(result.output).To(ContainSubstring("stopping helper process group"))
	})

	It("stops stunnel and tail and skips symlink munging when the TLS server is signalled", func() {
		sb := newSandbox()
		sb.writeFake(fakeStunnel, stunnelFake())
		sb.writeFake(fakeTail, blockingFakeBody(fakeTail))
		sb.writeFake(fakeRsync, rsyncVersionFake())
		dataDir := sb.dir("data")
		keyDir := sb.dir("keys")
		Expect(os.WriteFile(filepath.Join(keyDir, "psk.txt"), []byte("psk"), 0o600)).To(Succeed())

		result := sb.runEntrypoint(
			filepath.Join(repositoryRoot, "mover-rsync-tls", "server.sh"),
			nil,
			map[string]string{
				envVolsyncDataDir:  dataDir,
				envVolsyncKeyDir:   keyDir,
				envPrivilegedMover: "1",
			},
			"stunnel-started",
		)

		expectForwardedAndCleanedUp(sb, result, 143, fakeStunnel, fakeTail)
		Expect(result.output).To(ContainSubstring("Shutdown requested, skipping symlink munging."))
	})
})
