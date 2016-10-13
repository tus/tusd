package tusd_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/tus/tusd"
)

func TestMethodOverride(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	store := NewMockFullDataStore(mockCtrl)

	store.EXPECT().GetInfo("yes").Return(FileInfo{
		Offset: 5,
		Size:   20,
	}, nil)

	store.EXPECT().WriteChunk("yes", int64(5), NewReaderMatcher("hello")).Return(int64(5), nil)

	handler, _ := NewHandler(Config{
		DataStore: store,
	})

	(&httpTest{
		Name:   "Successful request",
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
}
