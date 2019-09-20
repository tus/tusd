package filelocker

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/pkg/handler"
)

var _ handler.Locker = &FileLocker{}

func TestFileLocker(t *testing.T) {
	a := assert.New(t)

	dir, err := ioutil.TempDir("", "tusd-file-locker")
	a.NoError(err)

	locker := FileLocker{dir}

	lock1, err := locker.NewLock("one")
	a.NoError(err)

	a.NoError(lock1.Lock())
	a.Equal(handler.ErrFileLocked, lock1.Lock())

	lock2, err := locker.NewLock("one")
	a.NoError(err)
	a.Equal(handler.ErrFileLocked, lock2.Lock())

	a.NoError(lock1.Unlock())
}
