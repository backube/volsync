package repository

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/restic/restic/internal/backend"
	"github.com/restic/restic/internal/backend/mem"
	"github.com/restic/restic/internal/debug"
	"github.com/restic/restic/internal/restic"
	rtest "github.com/restic/restic/internal/test"
)

type backendWrapper func(r backend.Backend) (backend.Backend, error)

func openLockTestRepo(t *testing.T, wrapper backendWrapper) (*Repository, backend.Backend) {
	be := backend.Backend(mem.New())
	// initialize repo
	TestRepositoryWithBackend(t, be, 0, Options{})

	// reopen repository to allow injecting a backend wrapper
	if wrapper != nil {
		var err error
		be, err = wrapper(be)
		rtest.OK(t, err)
	}

	return TestOpenBackend(t, be), be
}

func checkedLockRepo(ctx context.Context, t *testing.T, repo *Repository, lockerInst *locker, retryLock time.Duration) (*Unlocker, context.Context) {
	lock, wrappedCtx, err := lockerInst.Lock(ctx, repo, false, retryLock, func(msg string) {}, func(format string, args ...interface{}) {})
	rtest.OK(t, err)
	rtest.OK(t, wrappedCtx.Err())
	if lock.info.lock.Stale() {
		t.Fatal("lock returned stale lock")
	}
	return lock, wrappedCtx
}

func TestLock(t *testing.T) {
	t.Parallel()
	repo, _ := openLockTestRepo(t, nil)

	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo, lockerInst, 0)
	lock.Unlock()
	if wrappedCtx.Err() == nil {
		t.Fatal("unlock did not cancel context")
	}
}

func TestLockCancel(t *testing.T) {
	t.Parallel()
	repo, _ := openLockTestRepo(t, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lock, wrappedCtx := checkedLockRepo(ctx, t, repo, lockerInst, 0)
	cancel()
	if wrappedCtx.Err() == nil {
		t.Fatal("canceled parent context did not cancel context")
	}

	// Unlock should not crash
	lock.Unlock()
}

func TestLockConflict(t *testing.T) {
	t.Parallel()
	repo, be := openLockTestRepo(t, nil)
	repo2 := TestOpenBackend(t, be)

	lock, _, err := Lock(context.Background(), repo, true, 0, func(msg string) {}, func(format string, args ...interface{}) {})
	rtest.OK(t, err)
	defer lock.Unlock()
	_, _, err = Lock(context.Background(), repo2, false, 0, func(msg string) {}, func(format string, args ...interface{}) {})
	if err == nil {
		t.Fatal("second lock should have failed")
	}
	rtest.Assert(t, restic.IsAlreadyLocked(err), "unexpected error %v", err)
}

type writeOnceBackend struct {
	backend.Backend
	written bool
}

func (b *writeOnceBackend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	if b.written {
		return fmt.Errorf("fail after first write")
	}
	b.written = true
	return b.Backend.Save(ctx, h, rd)
}

func TestLockFailedRefresh(t *testing.T) {
	t.Parallel()
	repo, _ := openLockTestRepo(t, func(r backend.Backend) (backend.Backend, error) {
		return &writeOnceBackend{Backend: r}, nil
	})

	// reduce locking intervals to be suitable for testing
	li := &locker{
		retrySleepStart:       lockerInst.retrySleepStart,
		retrySleepMax:         lockerInst.retrySleepMax,
		refreshInterval:       20 * time.Millisecond,
		refreshabilityTimeout: 100 * time.Millisecond,
	}
	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo, li, 0)

	select {
	case <-wrappedCtx.Done():
		// expected lock refresh failure
	case <-time.After(time.Second):
		t.Fatal("failed lock refresh did not cause context cancellation")
	}
	// Unlock should not crash
	lock.Unlock()
}

type loggingBackend struct {
	backend.Backend
	t *testing.T
}

func (b *loggingBackend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	b.t.Logf("save %v @ %v", h, time.Now())
	err := b.Backend.Save(ctx, h, rd)
	b.t.Logf("save finished %v @ %v", h, time.Now())
	return err
}

func TestLockSuccessfulRefresh(t *testing.T) {
	t.Parallel()
	repo, _ := openLockTestRepo(t, func(r backend.Backend) (backend.Backend, error) {
		return &loggingBackend{
			Backend: r,
			t:       t,
		}, nil
	})

	t.Logf("test for successful lock refresh %v", time.Now())
	// reduce locking intervals to be suitable for testing
	li := &locker{
		retrySleepStart:       lockerInst.retrySleepStart,
		retrySleepMax:         lockerInst.retrySleepMax,
		refreshInterval:       60 * time.Millisecond,
		refreshabilityTimeout: 500 * time.Millisecond,
	}
	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo, li, 0)

	select {
	case <-wrappedCtx.Done():
		// don't call t.Fatal to allow the lock to be properly cleaned up
		t.Error("lock refresh failed", time.Now())

		// Dump full stacktrace
		buf := make([]byte, 1024*1024)
		n := runtime.Stack(buf, true)
		buf = buf[:n]
		t.Log(string(buf))

	case <-time.After(2 * li.refreshabilityTimeout):
		// expected lock refresh to work
	}
	// Unlock should not crash
	lock.Unlock()
}

type slowBackend struct {
	backend.Backend
	m     sync.Mutex
	sleep time.Duration
}

func (b *slowBackend) Save(ctx context.Context, h backend.Handle, rd backend.RewindReader) error {
	b.m.Lock()
	sleep := b.sleep
	b.m.Unlock()
	time.Sleep(sleep)
	return b.Backend.Save(ctx, h, rd)
}

func TestLockSuccessfulStaleRefresh(t *testing.T) {
	t.Parallel()
	var sb *slowBackend
	repo, _ := openLockTestRepo(t, func(r backend.Backend) (backend.Backend, error) {
		sb = &slowBackend{Backend: r}
		return sb, nil
	})

	t.Logf("test for successful lock refresh %v", time.Now())
	// reduce locking intervals to be suitable for testing
	li := &locker{
		retrySleepStart:       lockerInst.retrySleepStart,
		retrySleepMax:         lockerInst.retrySleepMax,
		refreshInterval:       10 * time.Millisecond,
		refreshabilityTimeout: 50 * time.Millisecond,
	}

	lock, wrappedCtx := checkedLockRepo(context.Background(), t, repo, li, 0)
	// delay lock refreshing long enough that the lock would expire
	sb.m.Lock()
	sb.sleep = li.refreshabilityTimeout + li.refreshInterval
	sb.m.Unlock()

	select {
	case <-wrappedCtx.Done():
		// don't call t.Fatal to allow the lock to be properly cleaned up
		t.Error("lock refresh failed", time.Now())

	case <-time.After(li.refreshabilityTimeout):
	}
	// reset slow backend
	sb.m.Lock()
	sb.sleep = 0
	sb.m.Unlock()
	debug.Log("normal lock period has expired")

	select {
	case <-wrappedCtx.Done():
		// don't call t.Fatal to allow the lock to be properly cleaned up
		t.Error("lock refresh failed", time.Now())

	case <-time.After(3 * li.refreshabilityTimeout):
		// expected lock refresh to work
	}

	// Unlock should not crash
	lock.Unlock()
}

func TestLockWaitTimeout(t *testing.T) {
	t.Parallel()
	repo, _ := openLockTestRepo(t, nil)

	elock, _, err := Lock(context.TODO(), repo, true, 0, func(msg string) {}, func(format string, args ...interface{}) {})
	rtest.OK(t, err)
	defer elock.Unlock()

	retryLock := 200 * time.Millisecond

	start := time.Now()
	_, _, err = Lock(context.TODO(), repo, false, retryLock, func(msg string) {}, func(format string, args ...interface{}) {})
	duration := time.Since(start)

	rtest.Assert(t, err != nil,
		"create normal lock with exclusively locked repo didn't return an error")
	rtest.Assert(t, strings.Contains(err.Error(), "repository is already locked exclusively"),
		"create normal lock with exclusively locked repo didn't return the correct error")
	rtest.Assert(t, retryLock <= duration && duration < retryLock*3/2,
		"create normal lock with exclusively locked repo didn't wait for the specified timeout")
}

func TestLockWaitCancel(t *testing.T) {
	t.Parallel()
	repo, _ := openLockTestRepo(t, nil)

	elock, _, err := Lock(context.TODO(), repo, true, 0, func(msg string) {}, func(format string, args ...interface{}) {})
	rtest.OK(t, err)
	defer elock.Unlock()

	retryLock := 200 * time.Millisecond
	cancelAfter := 40 * time.Millisecond

	start := time.Now()
	ctx, cancel := context.WithCancel(context.TODO())
	time.AfterFunc(cancelAfter, cancel)

	_, _, err = Lock(ctx, repo, false, retryLock, func(msg string) {}, func(format string, args ...interface{}) {})
	duration := time.Since(start)

	rtest.Assert(t, err != nil,
		"create normal lock with exclusively locked repo didn't return an error")
	rtest.Assert(t, strings.Contains(err.Error(), "context canceled"),
		"create normal lock with exclusively locked repo didn't return the correct error")
	rtest.Assert(t, cancelAfter <= duration && duration < retryLock-10*time.Millisecond,
		"create normal lock with exclusively locked repo didn't return in time, duration %v", duration)
}

func TestLockWaitSuccess(t *testing.T) {
	t.Parallel()
	repo, _ := openLockTestRepo(t, nil)

	elock, _, err := Lock(context.TODO(), repo, true, 0, func(msg string) {}, func(format string, args ...interface{}) {})
	rtest.OK(t, err)

	retryLock := 200 * time.Millisecond
	unlockAfter := 40 * time.Millisecond

	time.AfterFunc(unlockAfter, func() {
		elock.Unlock()
	})

	lock, _, err := Lock(context.TODO(), repo, false, retryLock, func(msg string) {}, func(format string, args ...interface{}) {})
	rtest.OK(t, err)
	lock.Unlock()
}

func createFakeLock(repo *Repository, t time.Time, pid int) (restic.ID, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return restic.ID{}, err
	}

	newLock := &restic.Lock{Time: t, PID: pid, Hostname: hostname}
	return restic.SaveJSONUnpacked(context.TODO(), &internalRepository{repo}, restic.LockFile, &newLock)
}

func lockExists(repo restic.Lister, t testing.TB, lockID restic.ID) bool {
	var exists bool
	rtest.OK(t, repo.List(context.TODO(), restic.LockFile, func(id restic.ID, size int64) error {
		if id == lockID {
			exists = true
		}
		return nil
	}))

	return exists
}

func removeLock(repo *Repository, id restic.ID) error {
	return (&internalRepository{repo}).RemoveUnpacked(context.TODO(), restic.LockFile, id)
}

func TestLockWithStaleLock(t *testing.T) {
	repo := TestRepository(t)

	id1, err := createFakeLock(repo, time.Now().Add(-time.Hour), os.Getpid())
	rtest.OK(t, err)

	id2, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid())
	rtest.OK(t, err)

	id3, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid()+500000)
	rtest.OK(t, err)

	processed, err := RemoveStaleLocks(context.TODO(), repo)
	rtest.OK(t, err)

	rtest.Assert(t, lockExists(repo, t, id1) == false,
		"stale lock still exists after RemoveStaleLocks was called")
	rtest.Assert(t, lockExists(repo, t, id2) == true,
		"non-stale lock was removed by RemoveStaleLocks")
	rtest.Assert(t, lockExists(repo, t, id3) == false,
		"stale lock still exists after RemoveStaleLocks was called")
	rtest.Assert(t, processed == 2,
		"number of locks removed does not match: expected %d, got %d",
		2, processed)

	rtest.OK(t, removeLock(repo, id2))
}

func TestRemoveAllLocks(t *testing.T) {
	repo := TestRepository(t)

	id1, err := createFakeLock(repo, time.Now().Add(-time.Hour), os.Getpid())
	rtest.OK(t, err)

	id2, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid())
	rtest.OK(t, err)

	id3, err := createFakeLock(repo, time.Now().Add(-time.Minute), os.Getpid()+500000)
	rtest.OK(t, err)

	processed, err := RemoveAllLocks(context.TODO(), repo)
	rtest.OK(t, err)

	rtest.Assert(t, lockExists(repo, t, id1) == false,
		"lock still exists after RemoveAllLocks was called")
	rtest.Assert(t, lockExists(repo, t, id2) == false,
		"lock still exists after RemoveAllLocks was called")
	rtest.Assert(t, lockExists(repo, t, id3) == false,
		"lock still exists after RemoveAllLocks was called")
	rtest.Assert(t, processed == 3,
		"number of locks removed does not match: expected %d, got %d",
		3, processed)
}
