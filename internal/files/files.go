// Package files writes files atomically and locks them the way the
// proper-lockfile package that Claude Code uses does: with a directory next to
// the file. The run lock is one such lock.
package files

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// WriteFile replaces path with data. Readers see the old file or the new one,
// never a part of either, and the data is on disk before the rename. The
// directory must exist, so a wrong directory fails instead of being created.
func WriteFile(path string, data []byte, perm fs.FileMode) error {
	return WriteFrom(path, bytes.NewReader(data), perm)
}

// WriteFileAll is WriteFile that first creates the directories above path.
func WriteFileAll(path string, data []byte, perm fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return WriteFile(path, data, perm)
}

// WriteFrom replaces path with what r holds, as WriteFile does.
func WriteFrom(path string, r io.Reader, perm fs.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := io.Copy(temp, r); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(perm); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return rename(temp.Name(), path)
}

// RenameNoReplace renames from, a file or a directory, to to, and fails with
// an error that matches fs.ErrExist when to exists, rather than replace it.
func RenameNoReplace(from, to string) error { return renameNoReplace(from, to) }

// Lock is a lock directory next to the file it guards.
type Lock struct {
	// Path is the lock directory, such as pins.json.lock.
	Path string
	// Stale is the age at which a lock is taken to be abandoned.
	Stale time.Duration
	// Attempts and Delay bound the wait for the lock.
	Attempts int
	Delay    time.Duration
	// Owner, when set, is written into the lock as its owner file, such as the
	// holder's PID. Gone reports whether the holder an owner file names has
	// ended. A stale lock whose holder has not ended stays, since a process
	// that is suspended stops refreshing its lock without giving it up. A
	// stale lock with no owner file is broken, as one is without Gone.
	Owner []byte
	Gone  func(owner []byte) bool
}

// ownerFile is the file in a lock directory that names its holder.
const ownerFile = "owner"

// ErrHeld means another process holds the lock.
var ErrHeld = errors.New("held by another process")

// Do runs fn while holding the lock.
func (l Lock) Do(fn func() error) error {
	release, err := l.Acquire()
	if err != nil {
		return err
	}
	defer release()
	return fn()
}

// Acquire takes the lock and keeps it fresh until release is called. Calling
// release more than once is safe.
func (l Lock) Acquire() (release func(), err error) {
	if err := os.MkdirAll(filepath.Dir(l.Path), 0o755); err != nil {
		return nil, err
	}
	delay := l.Delay
	attempts := max(l.Attempts, 1)
	var last error
	for tries := 0; tries < attempts; {
		err := os.Mkdir(l.Path, 0o755)
		if err == nil {
			if err := l.writeOwner(); err != nil {
				os.Remove(l.Path)
				return nil, err
			}
			return l.hold()
		}
		if !busy(err) {
			return nil, err
		}
		last = err
		// Breaking an abandoned lock is not a failed attempt, so even the
		// last attempt gets to take the lock it just freed.
		if l.breakStale() {
			continue
		}
		if tries++; tries < attempts {
			time.Sleep(delay)
			delay = min(delay*2, 500*time.Millisecond)
		}
	}
	// A lock directory that never appeared was never held, so the reason the
	// directory could not be made is the error, not ErrHeld.
	if !errors.Is(last, fs.ErrExist) {
		if _, err := os.Lstat(l.Path); err != nil {
			return nil, last
		}
	}
	return nil, fmt.Errorf("%s is %w: try again", l.Path, ErrHeld)
}

// writeOwner writes Owner into the lock this process just made, before hold
// notes the lock's time.
func (l Lock) writeOwner() error {
	if l.Owner == nil {
		return nil
	}
	return os.WriteFile(filepath.Join(l.Path, ownerFile), l.Owner, 0o644)
}

// hold keeps the lock this process just made until release is called.
func (l Lock) hold() (release func(), err error) {
	info, err := os.Stat(l.Path)
	if err != nil {
		// Without its time, release cannot tell this lock from a newer one,
		// so give it up rather than hold it until it goes stale.
		_ = removeLock(l.Path)
		return nil, err
	}
	h := &held{path: l.Path, made: info.ModTime()}
	stop := h.refresh(l.Stale / 2)
	var once sync.Once
	return func() {
		once.Do(func() {
			stop()
			h.release()
		})
	}, nil
}

// held is a lock this process made.
type held struct {
	path string
	// made is the lock's time as this process last set it.
	made time.Time
}

// refresh touches the lock every interval until stop is called, so a slow fn
// does not make it look abandoned, as proper-lockfile does.
func (h *held) refresh(every time.Duration) (stop func()) {
	if every <= 0 {
		return func() {}
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				h.touch()
			}
		}
	})
	return func() {
		close(done)
		wg.Wait()
	}
}

// touch moves the lock's time to now, unless another process broke the lock.
func (h *held) touch() {
	if info, err := os.Stat(h.path); err != nil || !info.ModTime().Equal(h.made) {
		return
	}
	now := time.Now()
	if os.Chtimes(h.path, now, now) != nil {
		return
	}
	if info, err := os.Stat(h.path); err == nil {
		h.made = info.ModTime()
	}
}

// breakStale removes an abandoned lock. Only the process that holds a second
// directory, the break lock, may remove it, and only after checking again that
// it is still stale. So a fresh lock is never removed, and two processes never
// both take the lock.
func (l Lock) breakStale() bool {
	if !l.isStale(l.Path) {
		return false
	}
	breaker := l.Path + ".break"
	if err := os.Mkdir(breaker, 0o755); err != nil {
		// A process that died while breaking leaves the break lock behind.
		if l.isStale(breaker) {
			os.Remove(breaker)
		}
		return false
	}
	defer os.Remove(breaker)
	return l.isStale(l.Path) && l.ownerGone() && removeLock(l.Path) == nil
}

// ownerGone reports whether the holder the lock's owner file names has ended.
// A lock with no owner file, or no Gone to ask, has no holder to wait for.
func (l Lock) ownerGone() bool {
	if l.Gone == nil {
		return true
	}
	owner, err := os.ReadFile(filepath.Join(l.Path, ownerFile))
	return err != nil || l.Gone(owner)
}

// removeLock removes a lock directory and its owner file. Anything else in it
// keeps it, so a directory that is not a lock is never removed.
func removeLock(path string) error {
	if err := os.Remove(filepath.Join(path, ownerFile)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.Remove(path)
}

func (l Lock) isStale(path string) bool {
	info, err := os.Stat(path)
	return err == nil && time.Since(info.ModTime()) > l.Stale
}

// release removes the lock if it is still the one this process made. Another
// process may have broken it as stale and made its own.
func (h *held) release() {
	if info, err := os.Stat(h.path); err == nil && info.ModTime().Equal(h.made) {
		_ = removeLock(h.path)
	}
}
