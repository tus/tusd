package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	. "github.com/tus/tusd/v2/pkg/handler"
)

// contextValueMatcher is a gomock.Matcher that tests if a given object
// is a context.Context and has a key with an expected value.
type contextValueMatcher struct {
	key           any
	expectedValue any
}

func (m contextValueMatcher) Matches(a interface{}) bool {
	ctx, ok := a.(context.Context)
	if !ok {
		return false
	}

	value := ctx.Value(m.key)
	return gomock.Eq(m.expectedValue).Matches(value)
}

func (m contextValueMatcher) String() string {
	return fmt.Sprintf("is a context.Context and has value %v (%T) under key %v", m.expectedValue, m.expectedValue, m.key)
}

// contextCancelMatcher is a gomock.Matcher that tests if a given object
// is a context.Context, which will be cancelled within a specific delay.
type contextCancelMatcher struct {
	minDelay time.Duration
	maxDelay time.Duration
}

func (m contextCancelMatcher) Matches(a interface{}) bool {
	ctx, ok := a.(context.Context)
	if !ok {
		return false
	}

	start := time.Now()
	select {
	case <-ctx.Done():
		delay := time.Since(start)
		fmt.Println(delay, m.minDelay)
		return delay > m.minDelay
	case <-time.After(m.maxDelay):
		fmt.Println("noo")
		return false
	}
}

func (m contextCancelMatcher) String() string {
	return "is a context.Context that should be cancelled"
}

func TestContext(t *testing.T) {
	SubTest(t, "Value", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		// This test ensures that values from the request's context are accessible in the store and hook events.

		// Define a custom type for the key, as recommended for context values
		type keyType string
		testKey := keyType("hello")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().NewUpload(contextValueMatcher{testKey, "world"}, FileInfo{
				Size:     300,
				MetaData: MetaData{},
			}).Return(upload, nil),
			upload.EXPECT().GetInfo(contextValueMatcher{testKey, "world"}).Return(FileInfo{
				ID:   "foo",
				Size: 300,
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer:        composer,
			BasePath:             "https://buy.art/files/",
			NotifyCreatedUploads: true,
		})

		c := make(chan HookEvent, 1)
		handler.CreatedUploads = c

		(&httpTest{
			Context: context.WithValue(context.Background(), testKey, "world"),
			Method:  "POST",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Length": "300",
			},
			Code: http.StatusCreated,
			ResHeader: map[string]string{
				"Location": "https://buy.art/files/foo",
			},
		}).Run(handler, t)

		// Check that the value is in the hook's context.
		event := <-c
		a := assert.New(t)
		a.Equal("world", event.Context.Value(testKey))
	})

	SubTest(t, "Cancel", func(t *testing.T, store *MockFullDataStore, composer *StoreComposer) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(gomock.Any(), "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(gomock.Any()).Return(FileInfo{
				Offset: 10,
				Size:   40,
			}, nil),
			upload.EXPECT().WriteChunk(contextCancelMatcher{150 * time.Millisecond, 250 * time.Millisecond}, int64(10), gomock.Any()),
		)

		handler, _ := NewHandler(Config{
			StoreComposer:                     composer,
			GracefulRequestCompletionDuration: 100 * time.Millisecond,
		})

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-time.After(100 * time.Millisecond)
			cancel()
		}()

		(&httpTest{
			Context: ctx,
			Method:  "PATCH",
			URL:     "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
				"Upload-Offset": "10",
				"Content-Type":  "application/offset+octet-stream",
			},
			ReqBody: strings.NewReader("hello"),
			Code:    http.StatusNoContent,
			ResHeader: map[string]string{
				"Upload-Offset": "10",
			},
		}).Run(handler, t)
	})
}
