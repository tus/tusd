package tusd_test

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	. "github.com/tus/tusd"
)

func TestPatch(t *testing.T) {
	SubTest(t, "UploadChunk", func(t *testing.T, store *MockFullDataStore) {
		gomock.InOrder(
			store.EXPECT().GetInfo("yes").Return(FileInfo{
				ID:     "yes",
				Offset: 5,
				Size:   10,
			}, nil),
			store.EXPECT().WriteChunk("yes", int64(5), NewReaderMatcher("hello")).Return(int64(5), nil),
			store.EXPECT().FinishUpload("yes"),
		)

		handler, _ := NewHandler(Config{
			DataStore:             store,
			NotifyCompleteUploads: true,
		})

		c := make(chan FileInfo, 1)
		handler.CompleteUploads = c

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "5",
			},
			ReqBody: strings.NewReader("hello"),
			Code:    http.StatusNoContent,
			ResHeader: map[string]string{
				"Upload-Offset": "10",
			},
		}).Run(handler, t)

		a := assert.New(t)
		info := <-c
		a.Equal("yes", info.ID)
		a.EqualValues(int64(10), info.Size)
		a.Equal(int64(10), info.Offset)
	})

	SubTest(t, "MethodOverriding", func(t *testing.T, store *MockFullDataStore) {
		gomock.InOrder(
			store.EXPECT().GetInfo("yes").Return(FileInfo{
				ID:     "yes",
				Offset: 5,
				Size:   20,
			}, nil),
			store.EXPECT().WriteChunk("yes", int64(5), NewReaderMatcher("hello")).Return(int64(5), nil),
		)

		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "POST",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable":          "1.0.0",
				"Upload-Offset":          "5",
				"Content-Type":           "application/offset+octet-stream",
				"X-HTTP-Method-Override": "PATCH",
			},
			ReqBody: strings.NewReader("hello"),
			Code:    http.StatusNoContent,
			ResHeader: map[string]string{
				"Upload-Offset": "10",
			},
		}).Run(handler, t)
	})

	SubTest(t, "UploadChunkToFinished", func(t *testing.T, store *MockFullDataStore) {
		store.EXPECT().GetInfo("yes").Return(FileInfo{
			Offset: 20,
			Size:   20,
		}, nil)

		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "20",
			},
			ReqBody: strings.NewReader(""),
			Code:    http.StatusNoContent,
			ResHeader: map[string]string{
				"Upload-Offset": "20",
			},
		}).Run(handler, t)
	})

	SubTest(t, "UploadNotFoundFail", func(t *testing.T, store *MockFullDataStore) {
		store.EXPECT().GetInfo("no").Return(FileInfo{}, os.ErrNotExist)

		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "PATCH",
			URL:    "no",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "5",
			},
			Code: http.StatusNotFound,
		}).Run(handler, t)
	})

	SubTest(t, "MissmatchingOffsetFail", func(t *testing.T, store *MockFullDataStore) {
		store.EXPECT().GetInfo("yes").Return(FileInfo{
			Offset: 5,
		}, nil)

		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "4",
			},
			Code: http.StatusConflict,
		}).Run(handler, t)
	})

	SubTest(t, "ExceedingMaxSizeFail", func(t *testing.T, store *MockFullDataStore) {
		store.EXPECT().GetInfo("yes").Return(FileInfo{
			Offset: 5,
			Size:   10,
		}, nil)

		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "5",
			},
			ReqBody: strings.NewReader("hellothisismorethan15bytes"),
			Code:    http.StatusRequestEntityTooLarge,
		}).Run(handler, t)
	})

	SubTest(t, "InvalidContentTypeFail", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/fail",
				"Upload-Offset": "5",
			},
			ReqBody: strings.NewReader("hellothisismorethan15bytes"),
			Code:    http.StatusBadRequest,
		}).Run(handler, t)
	})

	SubTest(t, "InvalidOffsetFail", func(t *testing.T, store *MockFullDataStore) {
		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "-5",
			},
			ReqBody: strings.NewReader("hellothisismorethan15bytes"),
			Code:    http.StatusBadRequest,
		}).Run(handler, t)
	})

	SubTest(t, "OverflowWithoutLength", func(t *testing.T, store *MockFullDataStore) {
		// In this test we attempt to upload more than 15 bytes to an upload
		// which has only space for 15 bytes (offset of 5 and size of 20).
		// The request does not contain the Content-Length header and the handler
		// therefore does not know the chunk's size before. The wanted behavior
		// is that even if the uploader supplies more than 15 bytes, we only
		// pass 15 bytes to the data store and ignore the rest.

		gomock.InOrder(
			store.EXPECT().GetInfo("yes").Return(FileInfo{
				Offset: 5,
				Size:   20,
			}, nil),
			store.EXPECT().WriteChunk("yes", int64(5), NewReaderMatcher("hellothisismore")).Return(int64(15), nil),
			store.EXPECT().FinishUpload("yes"),
		)

		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		// Wrap the string.Reader in a NopCloser to hide its type. else
		// http.NewRequest() will detect the we supply a strings.Reader as body
		// and use this information to set the Content-Length header which we
		// explicitly do not want (see comment above for reason).
		body := ioutil.NopCloser(strings.NewReader("hellothisismorethan15bytes"))

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "5",
			},
			ReqBody: body,
			Code:    http.StatusNoContent,
			ResHeader: map[string]string{
				"Upload-Offset": "20",
			},
		}).Run(handler, t)
	})

	SubTest(t, "Locker", func(t *testing.T, store *MockFullDataStore) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		locker := NewMockLocker(ctrl)

		gomock.InOrder(
			locker.EXPECT().LockUpload("yes").Return(nil),
			store.EXPECT().GetInfo("yes").Return(FileInfo{
				Offset: 0,
				Size:   20,
			}, nil),
			store.EXPECT().WriteChunk("yes", int64(0), NewReaderMatcher("hello")).Return(int64(5), nil),
			locker.EXPECT().UnlockUpload("yes").Return(nil),
		)

		composer := NewStoreComposer()
		composer.UseCore(store)
		composer.UseLocker(locker)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "0",
			},
			ReqBody: strings.NewReader("hello"),
			Code:    http.StatusNoContent,
		}).Run(handler, t)
	})

	SubTest(t, "NotifyUploadProgress", func(t *testing.T, store *MockFullDataStore) {
		gomock.InOrder(
			store.EXPECT().GetInfo("yes").Return(FileInfo{
				ID:     "yes",
				Offset: 0,
				Size:   100,
			}, nil),
			store.EXPECT().WriteChunk("yes", int64(0), NewReaderMatcher("first second third")).Return(int64(18), nil),
		)

		handler, _ := NewHandler(Config{
			DataStore:            store,
			NotifyUploadProgress: true,
		})

		c := make(chan FileInfo)
		handler.UploadProgress = c

		reader, writer := io.Pipe()
		a := assert.New(t)

		go func() {
			writer.Write([]byte("first "))

			info := <-c
			a.Equal("yes", info.ID)
			a.Equal(int64(100), info.Size)
			a.Equal(int64(6), info.Offset)

			writer.Write([]byte("second "))
			writer.Write([]byte("third"))

			info = <-c
			a.Equal("yes", info.ID)
			a.Equal(int64(100), info.Size)
			a.Equal(int64(18), info.Offset)

			writer.Close()

			info = <-c
			a.Equal("yes", info.ID)
			a.Equal(int64(100), info.Size)
			a.Equal(int64(18), info.Offset)
		}()

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "0",
			},
			ReqBody: reader,
			Code:    http.StatusNoContent,
			ResHeader: map[string]string{
				"Upload-Offset": "18",
			},
		}).Run(handler, t)

		// Wait a short time after the request has been handled before closing the
		// channel because another goroutine may still write to the channel.
		<-time.After(10 * time.Millisecond)
		close(handler.UploadProgress)

		_, more := <-c
		a.False(more)
	})
}
