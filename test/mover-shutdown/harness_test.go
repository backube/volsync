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
// spawns a grandchild that also traps TERM, waits until the grandchild's trap
// is armed, and only then records that it started: a spec that signals the
// process group after seeing the started marker must not race a trap that is
// not installed yet. On TERM it records the signal, takes time to clean up,
// reaps the grandchild, and exits 143.
func blockingFakeBody(name string) string {
	return fmt.Sprintf(`name=%s
(
    trap 'echo gc-term > "${FAKE_MARKER_DIR}/${name}-gc-term"; exit 0' TERM
    echo gc-ready > "${FAKE_MARKER_DIR}/${name}-gc-ready"
    while true; do sleep 1; done
) &
grandchild=$!
trap 'echo term > "${FAKE_MARKER_DIR}/${name}-term"
      sleep 1.2
      echo cleaned > "${FAKE_MARKER_DIR}/${name}-cleaned"
      wait "$grandchild" 2>/dev/null
      exit 143' TERM
while [[ ! -e "${FAKE_MARKER_DIR}/${name}-gc-ready" ]]; do
    sleep 0.02
done
echo started > "${FAKE_MARKER_DIR}/${name}-started"
while true; do sleep 60; done
`, name)
}

func (s *sandbox) environment(extra map[string]string) []string {
	overrides := map[string]string{
		"PATH":            s.binDir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"HOME":            s.root,
		"DEBUG_MOVER":     "0",
		"FAKE_MARKER_DIR": s.markersDir,
		// shutdown.sh keeps helper status files in a mktemp directory. Point
		// mktemp at the sandbox so a force-killed namespace cannot leak
		// status directories into the host /tmp.
		"TMPDIR":               s.dir("tmp"),
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
	ep := s.startEntrypoint(scriptPath, args, extraEnv)
	ep.waitForMarker(startedMarker)

	termSentAt := time.Now()
	Expect(syscall.Kill(ep.initPid, syscall.SIGTERM)).To(
		Succeed(), "failed to signal namespace init\noutput:\n%s", ep.output.String())

	ep.waitToExit(fmt.Sprintf("mover did not exit within %s after SIGTERM", exitTimeout))

	return runResult{
		exitCode: ep.cmd.ProcessState.ExitCode(),
		elapsed:  time.Since(termSentAt),
		output:   ep.output.String(),
	}
}

// runEntrypointToCompletion starts the entrypoint like runEntrypoint but never
// signals it: the script must exit on its own within exitTimeout. It reports
// the exit code, the time from start to exit, and the combined output, so a
// spec can assert that an entrypoint whose helpers failed stops itself
// instead of waiting for a signal that never comes.
func (s *sandbox) runEntrypointToCompletion(
	scriptPath string, args []string, extraEnv map[string]string,
) runResult {
	ep := s.startEntrypoint(scriptPath, args, extraEnv)
	ep.waitToExit(fmt.Sprintf("mover did not exit on its own within %s", exitTimeout))
	return ep.result()
}

// runningEntrypoint is a mover entrypoint running as PID 1 of its own PID
// namespace, together with the state needed to observe it and tear it down.
type runningEntrypoint struct {
	sb        *sandbox
	cmd       *exec.Cmd
	output    *lockedBuffer
	initPid   int
	done      chan error
	startedAt time.Time
	finished  bool
}

// startEntrypoint starts scriptPath via unshare so it becomes PID 1 of a new
// PID namespace and resolves the namespace init PID. The returned entrypoint
// must be collected with waitToExit; a spec that fails before that is covered
// by the registered cleanup.
func (s *sandbox) startEntrypoint(
	scriptPath string, args []string, extraEnv map[string]string,
) *runningEntrypoint {
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

	ep := &runningEntrypoint{
		sb:        s,
		cmd:       cmd,
		output:    &output,
		done:      make(chan error, 1),
		startedAt: time.Now(),
	}
	go func() { ep.done <- cmd.Wait() }()

	// Best-effort cleanup so a failing spec never leaves a process behind.
	DeferCleanup(ep.forceKill)

	var err error
	ep.initPid, err = waitForNamespaceInit(cmd.Process.Pid)
	Expect(err).NotTo(HaveOccurred(), "failed to locate the namespace init\noutput:\n%s", output.String())
	return ep
}

// forceKill tears down the namespace of an entrypoint that was never
// collected. It is a no-op once waitToExit has completed.
func (ep *runningEntrypoint) forceKill() {
	if ep.finished {
		return
	}
	// Killing PID 1 tears down the whole namespace; killing unshare also
	// covers the window before the init PID was resolved.
	if ep.initPid != 0 {
		_ = syscall.Kill(ep.initPid, syscall.SIGKILL)
	}
	_ = ep.cmd.Process.Kill()
}

// waitForMarker blocks until the named marker file appears. It fails the spec
// when the entrypoint exits before writing the marker or when the marker does
// not appear within startMarkerTimeout.
func (ep *runningEntrypoint) waitForMarker(name string) {
	path := filepath.Join(ep.sb.markersDir, name)
	deadline := time.Now().Add(startMarkerTimeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case err := <-ep.done:
			Fail(fmt.Sprintf("mover exited before writing %q: %v\noutput:\n%s",
				name, err, ep.output.String()))
		default:
		}
		time.Sleep(20 * time.Millisecond)
	}
	Fail(fmt.Sprintf(
		"timed out after %s waiting for %q\noutput:\n%s", startMarkerTimeout, name, ep.output.String()))
}

// waitToExit waits for the entrypoint to exit within exitTimeout. On timeout
// it tears down the namespace and fails the spec with the captured output.
func (ep *runningEntrypoint) waitToExit(timeoutMessage string) {
	select {
	case err := <-ep.done:
		// Wait returned; a non-nil error is expected when the mover exits
		// non-zero. The status is in cmd.ProcessState.
		_ = err
	case <-time.After(exitTimeout):
		ep.forceKill()
		Fail(fmt.Sprintf("%s\noutput:\n%s", timeoutMessage, ep.output.String()))
	}
	ep.finished = true
}

// result collects the outcome of an entrypoint that exited. The elapsed time
// is measured from the start of the entrypoint.
func (ep *runningEntrypoint) result() runResult {
	return runResult{
		exitCode: ep.cmd.ProcessState.ExitCode(),
		elapsed:  time.Since(ep.startedAt),
		output:   ep.output.String(),
	}
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
