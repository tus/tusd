package filestore

import (
	"io/ioutil"
	"testing"

	"github.com/tus/tusd"
	"github.com/tus/tusd/lockingstore"
)

func TestFileLocker(t *testing.T) {
	dir, err := ioutil.TempDir("", "tusd-file-locker")
	if err != nil {
		t.Fatal(err)
	}

	var locker lockingstore.Locker
	locker = FileLocker{dir}

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
