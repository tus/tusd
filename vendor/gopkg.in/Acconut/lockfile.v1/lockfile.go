// Handle pid file based locking.
package lockfile

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

type Lockfile string

var (
	ErrBusy        = errors.New("Locked by other process") // If you get this, retry after a short sleep might help
	ErrNeedAbsPath = errors.New("Lockfiles must be given as absolute path names")
	ErrInvalidPid  = errors.New("Lockfile contains invalid pid for system")
	ErrDeadOwner   = errors.New("Lockfile contains pid of process not existent on this system anymore")
)

// Describe a new filename located at path. It is expected to be an absolute path
func New(path string) (Lockfile, error) {
	if !filepath.IsAbs(path) {
		return Lockfile(""), ErrNeedAbsPath
	}
	return Lockfile(path), nil
}

// Who owns the lockfile?
func (l Lockfile) GetOwner() (*os.Process, error) {
	name := string(l)

	// Ok, see, if we have a stale lockfile here
	content, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, err
	}

	var pid int
	_, err = fmt.Sscanln(string(content), &pid)
	if err != nil {
		return nil, ErrInvalidPid
	}

	// try hard for pids. If no pid, the lockfile is junk anyway and we delete it.
	if pid > 0 {
		running, err := isRunning(pid)
		if err != nil {
			return nil, err
		}

		if running {
			proc, err := os.FindProcess(pid)
			if err != nil {
				return nil, err
			}
			return proc, nil
		} else {
			return nil, ErrDeadOwner
		}

	} else {
		return nil, ErrInvalidPid
	}
	panic("Not reached")
}

// Try to get Lockfile lock. Returns nil, if successful and and error describing the reason, it didn't work out.
// Please note, that existing lockfiles containing pids of dead processes and lockfiles containing no pid at all
// are deleted.
func (l Lockfile) TryLock() error {
	name := string(l)

	// This has been checked by New already. If we trigger here,
	// the caller didn't use New and re-implemented it's functionality badly.
	// So panic, that he might find this easily during testing.
	if !filepath.IsAbs(string(name)) {
		panic(ErrNeedAbsPath)
	}

	tmplock, err := ioutil.TempFile(filepath.Dir(name), "")
	if err != nil {
		return err
	} else {
		defer func(){
			tmplock.Close()
			os.Remove(tmplock.Name())
		}()
	}

	_, err = tmplock.WriteString(fmt.Sprintf("%d\n", os.Getpid()))
	if err != nil {
		return err
	}

	// return value intentionally ignored, as ignoring it is part of the algorithm
	_ = os.Link(tmplock.Name(), name)

	fiTmp, err := os.Lstat(tmplock.Name())
	if err != nil {
		return err
	}
	fiLock, err := os.Lstat(name)
	if err != nil {
		return err
	}

	// Success
	if os.SameFile(fiTmp, fiLock) {
		return nil
	}

	_, err = l.GetOwner()
	switch err {
	default:
		// Other errors -> defensively fail and let caller handle this
		return err
	case nil:
		return ErrBusy
	case ErrDeadOwner, ErrInvalidPid:
		// cases we can fix below
	}

	// clean stale/invalid lockfile
	err = os.Remove(name)
	if err != nil {
		return err
	}

	// now that we cleaned up the stale lockfile, let's recurse
	return l.TryLock()
}

// Release a lock again. Returns any error that happend during release of lock.
func (l Lockfile) Unlock() error {
	return os.Remove(string(l))
}
