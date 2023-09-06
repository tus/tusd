package handler_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	. "github.com/tus/tusd/v2/pkg/handler"
)

func TestPatch(t *testing.T) {
	SubTest(t, "UploadChunk", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 5,
				Size:   10,
			}, nil),
			upload.EXPECT().WriteChunk(gomock.Any(), int64(5), NewReaderMatcher("hello")).Return(int64(5), nil),
			upload.EXPECT().FinishUpload(gomock.Any()),
		)

		handler, _ := NewHandler(Config{
			StoreComposer:         composer,
			NotifyCompleteUploads: true,
		})

		c := make(chan HookEvent, 1)
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
		event := <-c
		info := event.Upload
		a.Equal("yes", info.ID)
		a.EqualValues(int64(10), info.Size)
		a.Equal(int64(10), info.Offset)

		req := event.HTTPRequest
		a.Equal("PATCH", req.Method)
		a.Equal("yes", req.URI)
		a.Equal("5", req.Header.Get("Upload-Offset"))
	})

	SubTest(t, "MethodOverriding", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 5,
				Size:   10,
			}, nil),
			upload.EXPECT().WriteChunk(gomock.Any(), int64(5), NewReaderMatcher("hello")).Return(int64(5), nil),
			upload.EXPECT().FinishUpload(gomock.Any()),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
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

	SubTest(t, "UploadChunkToFinished", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 20,
				Size:   20,
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
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

	SubTest(t, "UploadNotFoundFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		store.EXPECT().GetUpload(gomock.Any(), "no").Return(nil, ErrNotFound)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
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

	SubTest(t, "MissmatchingOffsetFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 5,
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
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

	SubTest(t, "ExceedingMaxSizeFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 5,
				Size:   10,
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
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

	SubTest(t, "InvalidContentTypeFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
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

	SubTest(t, "InvalidOffsetFail", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		handler, _ := NewHandler(Config{
			StoreComposer: composer,
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

	SubTest(t, "OverflowWithoutLength", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		// In this test we attempt to upload more than 15 bytes to an upload
		// which has only space for 15 bytes (offset of 5 and size of 20).
		// The request does not contain the Content-Length header and the handler
		// therefore does not know the chunk's size before. The wanted behavior
		// is that even if the uploader supplies more than 15 bytes, we only
		// pass 15 bytes to the data store and ignore the rest.

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 5,
				Size:   20,
			}, nil),
			upload.EXPECT().WriteChunk(gomock.Any(), int64(5), NewReaderMatcher("hellothisismore")).Return(int64(15), nil),
			upload.EXPECT().FinishUpload(gomock.Any()),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		// Wrap the string.Reader in a NopCloser to hide its type. else
		// http.NewRequest() will detect the we supply a strings.Reader as body
		// and use this information to set the Content-Length header which we
		// explicitly do not want (see comment above for reason).
		body := io.NopCloser(strings.NewReader("hellothisismorethan15bytes"))

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

	SubTest(t, "DeclareLengthOnFinalChunk", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:             "yes",
				Offset:         5,
				Size:           0,
				SizeIsDeferred: true,
			}, nil),
			store.EXPECT().AsLengthDeclarableUpload(upload).Return(upload),
			upload.EXPECT().DeclareLength(gomock.Any(), int64(20)),
			upload.EXPECT().WriteChunk(gomock.Any(), int64(5), NewReaderMatcher("hellothisismore")).Return(int64(15), nil),
			upload.EXPECT().FinishUpload(gomock.Any()),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			MaxSize:       20,
		})

		body := strings.NewReader("hellothisismore")

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "5",
				"Upload-Length": "20",
			},
			ReqBody: body,
			Code:    http.StatusNoContent,
			ResHeader: map[string]string{
				"Upload-Offset": "20",
			},
		}).Run(handler, t)
	})

	SubTest(t, "DeclareLengthAfterFinalChunk", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:             "yes",
				Offset:         20,
				Size:           0,
				SizeIsDeferred: true,
			}, nil),
			store.EXPECT().AsLengthDeclarableUpload(upload).Return(upload),
			upload.EXPECT().DeclareLength(gomock.Any(), int64(20)),
			upload.EXPECT().FinishUpload(gomock.Any()),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			MaxSize:       20,
		})

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "20",
				"Upload-Length": "20",
			},
			ReqBody:   nil,
			Code:      http.StatusNoContent,
			ResHeader: map[string]string{},
		}).Run(handler, t)
	})

	SubTest(t, "DeclareLengthOnNonFinalChunk", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload1 := NewMockFullUpload(ctrl)
		upload2 := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload1, nil),
			upload1.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:             "yes",
				Offset:         5,
				Size:           0,
				SizeIsDeferred: true,
			}, nil),
			store.EXPECT().AsLengthDeclarableUpload(upload1).Return(upload1),
			upload1.EXPECT().DeclareLength(gomock.Any(), int64(20)),
			upload1.EXPECT().WriteChunk(gomock.Any(), int64(5), NewReaderMatcher("hello")).Return(int64(5), nil),

			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload2, nil),
			upload2.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:             "yes",
				Offset:         10,
				Size:           20,
				SizeIsDeferred: false,
			}, nil),
			upload2.EXPECT().WriteChunk(gomock.Any(), int64(10), NewReaderMatcher("thisismore")).Return(int64(10), nil),
			upload2.EXPECT().FinishUpload(gomock.Any()),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
			MaxSize:       20,
		})

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "5",
				"Upload-Length": "20",
			},
			ReqBody: strings.NewReader("hello"),
			Code:    http.StatusNoContent,
			ResHeader: map[string]string{
				"Upload-Offset": "10",
			},
		}).Run(handler, t)

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "10",
			},
			ReqBody: strings.NewReader("thisismore"),
			Code:    http.StatusNoContent,
			ResHeader: map[string]string{
				"Upload-Offset": "20",
			},
		}).Run(handler, t)
	})

	SubTest(t, "Locker", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		locker := NewMockFullLocker(ctrl)
		lock := NewMockFullLock(ctrl)
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			locker.EXPECT().NewLock("yes").Return(lock, nil),
			lock.EXPECT().Lock(gomock.Any(), gomock.Any()).Return(nil),
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 0,
				Size:   20,
			}, nil),
			upload.EXPECT().WriteChunk(gomock.Any(), int64(0), NewReaderMatcher("hello")).Return(int64(5), nil),
			lock.EXPECT().Unlock().Return(nil),
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

	SubTest(t, "RequestUnlock", func(t *testing.T, store *MockFullDataStore, _ *StoreComposer) {
		// This test ensures that the handler will stop an onging write if another request wants the lock.
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		locker := NewMockFullLocker(ctrl)
		lock := NewMockFullLock(ctrl)
		upload := NewMockFullUpload(ctrl)

		var requestReleaseFn func()

		// Mock a typical data store. The lock's requestRelease function is exposed, so we can
		// later call it. The WriteChunk mock simulates a real store, which waits until the source is closed
		// instead of returning immediately.
		gomock.InOrder(
			locker.EXPECT().NewLock("yes").Return(lock, nil),
			lock.EXPECT().Lock(gomock.Any(), gomock.Any()).Do(func(_ context.Context, requestRelease func()) {
				requestReleaseFn = requestRelease
			}).Return(nil),
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 0,
				Size:   20,
			}, nil),
			upload.EXPECT().WriteChunk(gomock.Any(), int64(0), gomock.Any()).DoAndReturn(func(_ context.Context, _ int64, src io.Reader) (int64, error) {
				data, err := io.ReadAll(src)
				if string(data) != "hello" {
					panic("unexpected result of reading source")
				}
				return int64(len(data)), err
			}),
			lock.EXPECT().Unlock().Return(nil),
		)

		composer := NewStoreComposer()
		composer.UseCore(store)
		composer.UseLocker(locker)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		// Simulate an ongoing upload. The write must happen in a goroutine because
		// io.PipeWriter.Write will block until a read operation occurs.
		// After the write, we simulate that another request wants to acquire the lock.
		reader, writer := io.Pipe()
		go func() {
			writer.Write([]byte("hello"))
			requestReleaseFn()
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
			Code:    http.StatusBadRequest,
			ResBody: "ERR_UPLOAD_INTERRUPTED: upload has been interrupted by another request for this upload resource\n",
		}).Run(handler, t)
	})

	SubTest(t, "NotifyUploadProgress", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		// We simulate that the upload has already an offset of 10 bytes. Therefore, the progress notifications
		// must be the sum of the exisiting offset and the newly read bytes.
		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 10,
				Size:   100,
			}, nil),
			upload.EXPECT().WriteChunk(gomock.Any(), int64(10), NewReaderMatcher("first second third")).Return(int64(18), nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer:        composer,
			NotifyUploadProgress: true,
		})

		c := make(chan HookEvent)
		handler.UploadProgress = c

		reader, writer := io.Pipe()
		a := assert.New(t)

		go func() {
			writer.Write([]byte("first "))
			event := <-c

			info := event.Upload
			a.Equal("yes", info.ID)
			a.Equal(int64(100), info.Size)
			a.Equal(int64(16), info.Offset)

			writer.Write([]byte("second "))
			writer.Write([]byte("third"))

			event = <-c
			info = event.Upload
			a.Equal("yes", info.ID)
			a.Equal(int64(100), info.Size)
			a.Equal(int64(28), info.Offset)

			writer.Close()

			// No progress event is sent after the writer is closed
			// because an event for 18 bytes was already emitted.
		}()

		(&httpTest{
			Method: "PATCH",
			URL:    "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Content-Type":  "application/offset+octet-stream",
				"Upload-Offset": "10",
			},
			ReqBody: reader,
			Code:    http.StatusNoContent,
			ResHeader: map[string]string{
				"Upload-Offset": "28",
			},
		}).Run(handler, t)

		// Wait a short time after the request has been handled before closing the
		// channel because another goroutine may still write to the channel.
		<-time.After(10 * time.Millisecond)
		close(handler.UploadProgress)

		_, more := <-c
		a.False(more)
	})

	SubTest(t, "StopUpload", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 0,
				Size:   100,
			}, nil),
			upload.EXPECT().WriteChunk(gomock.Any(), int64(0), NewReaderMatcher("first ")).Return(int64(6), nil),
			store.EXPECT().AsTerminatableUpload(upload).Return(upload),
			upload.EXPECT().Terminate(gomock.Any()),
		)

		handler, _ := NewHandler(Config{
			StoreComposer:        composer,
			NotifyUploadProgress: true,
		})

		c := make(chan HookEvent)
		handler.UploadProgress = c

		reader, writer := io.Pipe()
		a := assert.New(t)

		go func() {
			writer.Write([]byte("first "))

			event := <-c
			info := event.Upload
			// Include a custom response
			info.StopUpload(HTTPResponse{
				StatusCode: http.StatusPaymentRequired,
				Body:       "upload is stopped because you didn't pay",
				Header: HTTPHeader{
					"X-Foo": "bar",
				},
			})

			// Wait a short time to ensure that the goroutine in the PATCH
			// handler has received and processed the stop event.
			<-time.After(10 * time.Millisecond)

			// Assert that the "request body" has been closed.
			_, err := writer.Write([]byte("second "))
			a.Equal(err, io.ErrClosedPipe)

			// Close the upload progress handler so that the main goroutine
			// can exit properly after waiting for this goroutine to finish.
			close(handler.UploadProgress)
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
			Code:    http.StatusPaymentRequired,
			ResHeader: map[string]string{
				"Upload-Offset": "",
				"X-Foo":         "bar",
			},
			ResBody: "upload is stopped because you didn't pay",
		}).Run(handler, t)

		_, more := <-c
		a.False(more)
	})

	SubTest(t, "BodyReadError", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		// This test ensure that error that occurr from reading the request body are not forwarded to the
		// storage backend but are still causing an
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 0,
				Size:   100,
			}, nil),
			// The reader for WriteChunk must not return an error.
			upload.EXPECT().WriteChunk(gomock.Any(), int64(0), NewReaderMatcher("first ")).Return(int64(6), nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		reader, writer := io.Pipe()
		a := assert.New(t)

		go func() {
			writer.Write([]byte("first "))
			err := writer.CloseWithError(errors.New("an error while reading the body"))
			a.NoError(err)
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
			Code:    http.StatusInternalServerError,
			ResHeader: map[string]string{
				"Upload-Offset": "",
			},
			ResBody: "ERR_INTERNAL_SERVER_ERROR: an error while reading the body\n",
		}).Run(handler, t)
	})

	SubTest(t, "InterruptRequestHandling", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				ID:     "yes",
				Offset: 0,
				Size:   100,
			}, nil),
			upload.EXPECT().WriteChunk(gomock.Any(), int64(0), NewReaderMatcher("first ")).Return(int64(6), nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		reader, writer := io.Pipe()
		a := assert.New(t)

		go func() {
			writer.Write([]byte("first "))

			handler.InterruptRequestHandling()

			// Wait a short time to ensure that the goroutine in the PATCH
			// handler has received and processed the stop event.
			<-time.After(10 * time.Millisecond)

			// Assert that the "request body" has been closed.
			_, err := writer.Write([]byte("second "))
			a.Equal(err, io.ErrClosedPipe)
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
			Code:    http.StatusInternalServerError,
			ResHeader: map[string]string{
				"Upload-Offset": "",
			},
			ResBody: "ERR_SERVER_SHUTDOWN: request has been interrupted because the server is shutting down\n",
		}).Run(handler, t)
	})

	SubTest(t, "ExperimentalProtocol", func(t *testing.T, _ *MockFullDataStore, _ *StoreComposer) {
		SubTest(t, "CompleteUploadWithKnownSize", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:             "yes",
					Offset:         5,
					Size:           10,
					SizeIsDeferred: false,
				}, nil),
				upload.EXPECT().WriteChunk(gomock.Any(), int64(5), NewReaderMatcher("hello")).Return(int64(5), nil),
				upload.EXPECT().FinishUpload(gomock.Any()),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:              composer,
				EnableExperimentalProtocol: true,
			})

			(&httpTest{
				Method: "PATCH",
				URL:    "yes",
				ReqHeader: map[string]string{
					"Upload-Draft-Interop-Version": "3",
					"Upload-Offset":                "5",
					"Upload-Incomplete":            "?0",
				},
				ReqBody: strings.NewReader("hello"),
				Code:    http.StatusNoContent,
				ResHeader: map[string]string{
					"Upload-Offset": "10",
				},
			}).Run(handler, t)
		})
		SubTest(t, "CompleteUploadWithUnknownSize", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:             "yes",
					Offset:         5,
					Size:           0,
					SizeIsDeferred: true,
				}, nil),
				upload.EXPECT().WriteChunk(gomock.Any(), int64(5), NewReaderMatcher("hello")).Return(int64(5), nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:             "yes",
					Offset:         10,
					Size:           0,
					SizeIsDeferred: true,
				}, nil),
				store.EXPECT().AsLengthDeclarableUpload(upload).Return(upload),
				upload.EXPECT().DeclareLength(gomock.Any(), int64(10)),
				upload.EXPECT().FinishUpload(gomock.Any()),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:              composer,
				EnableExperimentalProtocol: true,
			})

			(&httpTest{
				Method: "PATCH",
				URL:    "yes",
				ReqHeader: map[string]string{
					"Upload-Draft-Interop-Version": "3",
					"Upload-Offset":                "5",
					"Upload-Incomplete":            "?0",
				},
				ReqBody: strings.NewReader("hello"),
				Code:    http.StatusNoContent,
				ResHeader: map[string]string{
					"Upload-Offset": "10",
				},
			}).Run(handler, t)
		})
		SubTest(t, "ContinueUploadWithKnownSize", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:             "yes",
					Offset:         5,
					Size:           10,
					SizeIsDeferred: false,
				}, nil),
				upload.EXPECT().WriteChunk(gomock.Any(), int64(5), NewReaderMatcher("hel")).Return(int64(3), nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:              composer,
				EnableExperimentalProtocol: true,
			})

			(&httpTest{
				Method: "PATCH",
				URL:    "yes",
				ReqHeader: map[string]string{
					"Upload-Draft-Interop-Version": "3",
					"Upload-Offset":                "5",
					"Upload-Incomplete":            "?1",
				},
				ReqBody: strings.NewReader("hel"),
				Code:    http.StatusNoContent,
				ResHeader: map[string]string{
					"Upload-Offset": "8",
				},
			}).Run(handler, t)
		})
		SubTest(t, "ContinueUploadWithUnknownSize", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			upload := NewMockFullUpload(ctrl)

			gomock.InOrder(
				store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
				upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
					ID:             "yes",
					Offset:         5,
					Size:           0,
					SizeIsDeferred: true,
				}, nil),
				upload.EXPECT().WriteChunk(gomock.Any(), int64(5), NewReaderMatcher("hel")).Return(int64(3), nil),
			)

			handler, _ := NewHandler(Config{
				StoreComposer:              composer,
				EnableExperimentalProtocol: true,
			})

			(&httpTest{
				Method: "PATCH",
				URL:    "yes",
				ReqHeader: map[string]string{
					"Upload-Draft-Interop-Version": "3",
					"Upload-Offset":                "5",
					"Upload-Incomplete":            "?1",
				},
				ReqBody: strings.NewReader("hel"),
				Code:    http.StatusNoContent,
				ResHeader: map[string]string{
					"Upload-Offset": "8",
				},
			}).Run(handler, t)
		})
	})
}
