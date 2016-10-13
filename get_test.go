package tusd_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/tus/tusd"
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
	SubTest(t, "Download", func(t *testing.T, store *MockFullDataStore) {
		reader := &closingStringReader{
			Reader: strings.NewReader("hello"),
		}

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		locker := NewMockLocker(ctrl)

		gomock.InOrder(
			locker.EXPECT().LockUpload("yes"),
			store.EXPECT().GetInfo("yes").Return(FileInfo{
				Offset: 5,
				Size:   20,
				MetaData: map[string]string{
					"filename": "file.jpg\"evil",
				},
			}, nil),
			store.EXPECT().GetReader("yes").Return(reader, nil),
			locker.EXPECT().UnlockUpload("yes"),
		)

		composer := NewStoreComposer()
		composer.UseCore(store)
		composer.UseGetReader(store)
		composer.UseLocker(locker)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Method: "GET",
			URL:    "yes",
			ResHeader: map[string]string{
				"Content-Length":      "5",
				"Content-Disposition": `inline;filename="file.jpg\"evil"`,
			},
			Code:    http.StatusOK,
			ResBody: "hello",
		}).Run(handler, t)

		if !reader.closed {
			t.Error("expected reader to be closed")
		}
	})

	SubTest(t, "EmptyDownload", func(t *testing.T, store *MockFullDataStore) {
		store.EXPECT().GetInfo("yes").Return(FileInfo{
			Offset: 0,
			MetaData: map[string]string{
				"filename": "file.jpg\"evil",
			},
		}, nil)

		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method: "GET",
			URL:    "yes",
			ResHeader: map[string]string{
				"Content-Length":      "0",
				"Content-Disposition": `inline;filename="file.jpg\"evil"`,
			},
			Code:    http.StatusNoContent,
			ResBody: "",
		}).Run(handler, t)
	})
}
