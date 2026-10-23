package mover_shutdown

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	// cleanupDelay is the time a fake mover takes to "clean up" after TERM.
	// A wrapper that does not wait for its child exits faster than this.
	cleanupDelay = time.Second

	namespaceInitTimeout = 5 * time.Second
	startMarkerTimeout   = 10 * time.Second
	exitTimeout          = 60 * time.Second
)

// repositoryRoot is derived from this source file so the tests do not depend
// on the working directory.
var repositoryRoot = locateRepositoryRoot()

func locateRepositoryRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot locate repository root: runtime.Caller failed")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(file), "..", ".."))
	if err != nil {
		panic(fmt.Sprintf("cannot resolve repository root: %v", err))
	}
	return root
}

var (
	pidNamespaceCheckOnce sync.Once
	errPIDNamespaceCheck  error
)

// requirePIDNamespaces skips the spec when the host cannot create a PID
// namespace. A skip is deliberate: these tests only mean something when the
// entrypoint really runs as PID 1 in its own namespace.
func requirePIDNamespaces() {
	pidNamespaceCheckOnce.Do(func() {
		out, err := exec.Command(
			"unshare", "-r", "--pid", "--fork", "--mount-proc", "true",
		).CombinedOutput()
		if err != nil {
			errPIDNamespaceCheck = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
	})
	if errPIDNamespaceCheck != nil {
		Skip(fmt.Sprintf("PID namespaces are not available: %v", errPIDNamespaceCheck))
	}
}

// sandbox is a per-spec directory holding fake binaries and the markers they
// write.
type sandbox struct {
	root       string
	binDir     string
	markersDir string
}

func newSandbox() *sandbox {
	root := GinkgoT().TempDir()
	sb := &sandbox{
		root:       root,
		binDir:     filepath.Join(root, "bin"),
		markersDir: filepath.Join(root, "markers"),
	}
	Expect(os.MkdirAll(sb.binDir, 0o750)).To(Succeed())
	Expect(os.MkdirAll(sb.markersDir, 0o750)).To(Succeed())
	return sb
}

func (s *sandbox) dir(name string) string {
	path := filepath.Join(s.root, name)
	Expect(os.MkdirAll(path, 0o750)).To(Succeed())
	return path
}

func (s *sandbox) writeFake(name, body string) string {
	path := filepath.Join(s.binDir, name)
	// #nosec G306 -- fake binaries must be executable by the mover script.
	Expect(os.WriteFile(path, []byte("#!/bin/bash\n"+body), 0o755)).To(Succeed())
	return path
}

// blockingFakeBody is the body of a fake long-running mover process. It
// records that it started, spawns a grandchild that also traps TERM, then
// blocks. On TERM it records the signal, takes time to clean up, reaps the
// grandchild, and exits 143.
func blockingFakeBody(name string) string {
	return fmt.Sprintf(`name=%s
echo started > "${FAKE_MARKER_DIR}/${name}-started"
(
    trap 'echo gc-term > "${FAKE_MARKER_DIR}/${name}-gc-term"; exit 0' TERM
    while true; do sleep 1; done
) &
grandchild=$!
trap 'echo term > "${FAKE_MARKER_DIR}/${name}-term"
      sleep 1.2
      echo cleaned > "${FAKE_MARKER_DIR}/${name}-cleaned"
      wait "$grandchild" 2>/dev/null
      exit 143' TERM
while true; do sleep 60; done
`, name)
}

func (s *sandbox) environment(extra map[string]string) []string {
	overrides := map[string]string{
		"PATH":                 s.binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME":                 s.root,
		"DEBUG_MOVER":          "0",
		"FAKE_MARKER_DIR":      s.markersDir,
		"VOLSYNC_SHUTDOWN_LIB": filepath.Join(repositoryRoot, "mover-common", "shutdown.sh"),
	}
	for name, value := range extra {
		overrides[name] = value
	}

	env := make([]string, 0, len(os.Environ())+len(overrides))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if _, overridden := overrides[name]; !overridden {
			env = append(env, entry)
		}
	}
	for name, value := range overrides {
		env = append(env, name+"="+value)
	}
	return env
}

// commandFor builds the command unshare should execute. The rsync-tls mover
// scripts are not tracked with the executable bit (the image runs them through
// /bin/bash, see internal/controller/mover/rsynctls/mover.go), so mirror that
// invocation instead of failing to exec the repository file.
func commandFor(scriptPath string, args []string) []string {
	info, err := os.Stat(scriptPath)
	Expect(err).NotTo(HaveOccurred(), "cannot stat %s", scriptPath)
	if info.Mode()&0o111 == 0 {
		return append([]string{"/bin/bash", scriptPath}, args...)
	}
	return append([]string{scriptPath}, args...)
}

// lockedBuffer collects the mover's combined output. os/exec writes from a
// goroutine, while failure messages read the buffer before Wait returns.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type runResult struct {
	exitCode int
	elapsed  time.Duration
	output   string
}

// runEntrypoint starts scriptPath via unshare so it becomes PID 1 of a new PID
// namespace, waits for startedMarker, sends SIGTERM to PID 1 only, and waits
// for the namespace to exit.
func (s *sandbox) runEntrypoint(
	scriptPath string, args []string, extraEnv map[string]string, startedMarker string,
) runResult {
	unshareArgs := make([]string, 0, 4+len(args)+2)
	unshareArgs = append(unshareArgs, "-r", "--pid", "--fork", "--mount-proc")
	unshareArgs = append(unshareArgs, commandFor(scriptPath, args)...)
	// #nosec G204 -- arguments come from the test spec, not from user input.
	cmd := exec.Command("unshare", unshareArgs...)
	cmd.Dir = s.root
	cmd.Env = s.environment(extraEnv)

	var output lockedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	Expect(cmd.Start()).To(Succeed(), "failed to start %s", scriptPath)

	finished := false
	defer func() {
		if !finished {
			// Best-effort cleanup so a failing spec never leaves a process behind.
			_ = cmd.Process.Kill()
		}
	}()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	initPid, err := waitForNamespaceInit(cmd.Process.Pid)
	Expect(err).NotTo(HaveOccurred(), "failed to locate the namespace init\noutput:\n%s", output.String())

	defer func() {
		if !finished {
			// Killing PID 1 tears down the whole namespace.
			_ = syscall.Kill(initPid, syscall.SIGKILL)
		}
	}()

	s.waitForMarker(startedMarker, done, &output)

	termSentAt := time.Now()
	Expect(syscall.Kill(initPid, syscall.SIGTERM)).To(
		Succeed(), "failed to signal namespace init\noutput:\n%s", output.String())

	select {
	case <-done:
	case <-time.After(exitTimeout):
		Fail(fmt.Sprintf(
			"mover did not exit within %s after SIGTERM\noutput:\n%s", exitTimeout, output.String()))
	}
	elapsed := time.Since(termSentAt)
	finished = true

	return runResult{
		exitCode: cmd.ProcessState.ExitCode(),
		elapsed:  elapsed,
		output:   output.String(),
	}
}

func (s *sandbox) waitForMarker(name string, done <-chan error, output *lockedBuffer) {
	path := filepath.Join(s.markersDir, name)
	deadline := time.Now().Add(startMarkerTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-done:
			Fail(fmt.Sprintf("mover exited before writing %q\noutput:\n%s", name, output.String()))
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	Fail(fmt.Sprintf(
		"timed out after %s waiting for %q\noutput:\n%s", startMarkerTimeout, name, output.String()))
}

// waitForNamespaceInit resolves the PID namespace init (PID 1 inside the new
// namespace) from the unshare process's children list.
func waitForNamespaceInit(unsharePid int) (int, error) {
	childrenPath := fmt.Sprintf("/proc/%d/task/%d/children", unsharePid, unsharePid)
	deadline := time.Now().Add(namespaceInitTimeout)
	for time.Now().Before(deadline) {
		// #nosec G304 -- path is a constructed /proc entry for our own child.
		data, err := os.ReadFile(childrenPath)
		if err == nil {
			fields := strings.Fields(string(data))
			if len(fields) > 0 {
				pid, convErr := strconv.Atoi(fields[0])
				if convErr == nil {
					return pid, nil
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return 0, fmt.Errorf("no child process found in %s", childrenPath)
}

// expectForwardedAndCleanedUp asserts that every named fake mover received
// TERM, cleaned up, reported that its grandchild was signalled, and that the
// entrypoint waited for the cleanup before exiting with expectedExit.
func expectForwardedAndCleanedUp(
	sb *sandbox, result runResult, expectedExit int, names ...string,
) {
	Expect(result.exitCode).To(Equal(expectedExit), "mover output:\n%s", result.output)
	Expect(result.elapsed).To(BeNumerically(">=", cleanupDelay),
		"entrypoint exited before its child finished cleanup\noutput:\n%s", result.output)
	Expect(result.elapsed).To(BeNumerically("<", 30*time.Second),
		"entrypoint took too long to shut down\noutput:\n%s", result.output)
	Expect(result.output).To(ContainSubstring("forwarding SIGTERM"))

	for _, name := range names {
		for _, suffix := range []string{"-term", "-cleaned", "-gc-term"} {
			marker := filepath.Join(sb.markersDir, name+suffix)
			_, err := os.Stat(marker)
			Expect(err).NotTo(HaveOccurred(),
				"expected marker %q\nmover output:\n%s", marker, result.output)
		}
	}
}
