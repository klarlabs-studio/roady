package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Two agents run two roady processes over the same .roady/. A version check
// alone leaves a window between reading the version on disk and writing the
// file, in which both can pass and both write: two claims on one task. The
// check and the write therefore happen while holding a lock file, created
// exclusively so only one process holds it. It works the same on every OS.

const (
	lockWait  = 10 * time.Second
	lockStale = 30 * time.Second // a holder that died leaves its lock behind
	lockPoll  = 5 * time.Millisecond
)

// withFileLock runs fn while holding path+".lock".
func withFileLock(path string, fn func() error) error {
	lock := path + ".lock"
	deadline := time.Now().Add(lockWait)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- path is resolved by the repository
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
			_ = f.Close()
			defer func() { _ = os.Remove(lock) }()
			return fn()
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("lock %s: %w", filepath.Base(path), err)
		}
		if info, statErr := os.Stat(lock); statErr == nil && time.Since(info.ModTime()) > lockStale {
			_ = os.Remove(lock)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s is locked by another roady process (remove %s if none is running)", filepath.Base(path), lock)
		}
		time.Sleep(lockPoll)
	}
}

// writeFileAtomic replaces path with data so a reader never sees a half
// written file.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}
