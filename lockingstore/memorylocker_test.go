package lockingstore

import (
	"testing"

	"github.com/tus/tusd"
)

func TestMemoryLocker(t *testing.T) {
	var locker Locker
	locker = New()

	if err := locker.LockUpload("one"); err != nil {
		t.Errorf("unexpected error when locking file: %s", err)
	}

	if err := locker.LockUpload("one"); err != tusd.ErrFileLocked {
		t.Errorf("expected error when locking locked file: %s", err)
	}

	if err := locker.UnlockUpload("one"); err != nil {
		t.Errorf("unexpected error when unlocking file: %s", err)
	}

	if err := locker.UnlockUpload("one"); err != nil {
		t.Errorf("unexpected error when unlocking file again: %s", err)
	}
}
