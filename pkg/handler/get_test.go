package handler_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/tus/tusd/pkg/handler"
)

type closingStringReader struct {
	*strings.Reader
	closed bool
}

func (reader *closingStringReader) Close() error {
	reader.closed = true
	return nil
}

func TestGet(t *testing.T) {
	SubTest(t, "Download", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		reader := &closingStringReader{
			Reader: strings.NewReader("hello"),
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		locker := NewMockFullLocker(ctrl)
		lock := NewMockFullLock(ctrl)
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			locker.EXPECT().NewLock("yes").Return(lock, nil),
			lock.EXPECT().Lock().Return(nil),
			store.EXPECT().GetUpload(context.Background(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
				Offset: 5,
				Size:   20,
				MetaData: map[string]string{
					"filename": "file.jpg\"evil",
					"filetype": "image/jpeg",
				},
			}, nil),
			upload.EXPECT().GetReader(context.Background()).Return(reader, nil),
			lock.EXPECT().Unlock().Return(nil),
		)

		composer = NewStoreComposer()
		composer.UseCore(store)
		composer.UseLocker(locker)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "GET",
			URL:    "yes",
			ResHeader: map[string]string{
				"Content-Length":      "5",
				"Content-Type":        "image/jpeg",
				"Content-Disposition": `inline;filename="file.jpg\"evil"`,
			},
			Code:    http.StatusOK,
			ResBody: "hello",
		}).Run(handler, t)

		if !reader.closed {
			t.Error("expected reader to be closed")
		}
	})

	SubTest(t, "EmptyDownload", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(context.Background(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
				Offset: 0,
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "GET",
			URL:    "yes",
			ResHeader: map[string]string{
				"Content-Length":      "0",
				"Content-Disposition": `attachment`,
			},
			Code:    http.StatusNoContent,
			ResBody: "",
		}).Run(handler, t)
	})

	SubTest(t, "InvalidFileType", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(context.Background(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
				Offset: 0,
				MetaData: map[string]string{
					"filetype": "non-a-valid-mime-type",
				},
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "GET",
			URL:    "yes",
			ResHeader: map[string]string{
				"Content-Length":      "0",
				"Content-Type":        "application/octet-stream",
				"Content-Disposition": `attachment`,
			},
			Code:    http.StatusNoContent,
			ResBody: "",
		}).Run(handler, t)
	})

	SubTest(t, "NotWhitelistedFileType", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(context.Background(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(context.Background()).Return(FileInfo{
				Offset: 0,
				MetaData: map[string]string{
					"filetype": "application/vnd.openxmlformats-officedocument.wordprocessingml.document.v1",
					"filename": "invoice.docx",
				},
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "GET",
			URL:    "yes",
			ResHeader: map[string]string{
				"Content-Length":      "0",
				"Content-Type":        "application/vnd.openxmlformats-officedocument.wordprocessingml.document.v1",
				"Content-Disposition": `attachment;filename="invoice.docx"`,
			},
			Code:    http.StatusNoContent,
			ResBody: "",
		}).Run(handler, t)
	})
}
