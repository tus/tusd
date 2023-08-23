package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
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
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		upload := NewMockFullUpload(ctrl)

		gomock.InOrder(
			store.EXPECT().GetUpload(contextValueMatcher{"hello", "world"}, "yes").Return(upload, nil),
			upload.EXPECT().GetInfo(contextValueMatcher{"hello", "world"}).Return(FileInfo{
				Offset: 10,
				Size:   40,
			}, nil),
		)

		handler, _ := NewHandler(Config{
			StoreComposer: composer,
		})

		(&httpTest{
			Context: context.WithValue(context.Background(), "hello", "world"),
			Method:  "HEAD",
			URL:     "yes",
			ReqHeader: map[string]string{
				"Tus-Resumable": "1.0.0",
			},
			Code: http.StatusOK,
			ResHeader: map[string]string{
				"Upload-Offset": "10",
				"Upload-Length": "40",
			},
		}).Run(handler, t)
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
