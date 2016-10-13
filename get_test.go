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

var reader = &closingStringReader{
	Reader: strings.NewReader("hello"),
}

func TestGet(t *testing.T) {
	SubTest(t, "Download", func(t *testing.T, store *MockFullDataStore) {
		gomock.InOrder(
			store.EXPECT().GetInfo("yes").Return(FileInfo{
				Offset: 5,
				Size:   20,
				MetaData: map[string]string{
					"filename": "file.jpg\"evil",
				},
			}, nil),
			store.EXPECT().GetReader("yes").Return(reader, nil),
		)

		handler, _ := NewHandler(Config{
			DataStore: store,
		})

		(&httpTest{
			Method:  "GET",
			URL:     "yes",
			Code:    http.StatusOK,
			ResBody: "hello",
			ResHeader: map[string]string{
				"Content-Length":      "5",
				"Content-Disposition": `inline;filename="file.jpg\"evil"`,
			},
		}).Run(handler, t)

		if !reader.closed {
			t.Error("expected reader to be closed")
		}
	})
}
