package hooks

import (
	"errors"
	"net/http"
	"testing"
	"time"

	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/tus/tusd/v2/pkg/filestore"
	"github.com/tus/tusd/v2/pkg/handler"
)

//go:generate mockgen -source=hooks.go -destination=hooks_mock_test.go -package=hooks

func TestNewHandlerWithHooks(t *testing.T) {
	a := assert.New(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := filestore.New("some-path", 0774, 0664)
	config := handler.Config{
		StoreComposer: handler.NewStoreComposer(),
	}
	store.UseIn(config.StoreComposer)
	hookHandler := NewMockHookHandler(ctrl)

	event := handler.HookEvent{
		Upload: handler.FileInfo{
			ID: "id",
			MetaData: handler.MetaData{
				"hello": "world",
			},
		},
		HTTPRequest: handler.HTTPRequest{
			Method: "POST",
			URI:    "/files/",
			Header: http.Header{
				"X-Hello": []string{"there"},
			},
		},
	}

	response := handler.HTTPResponse{
		StatusCode: 200,
		Body:       "foobar",
		Header: handler.HTTPHeader{
			"X-Hello": "here",
		},
	}

	change := handler.FileInfoChanges{
		ID: "id2",
		MetaData: handler.MetaData{
			"hello": "world2",
		},
		Storage: map[string]string{
			"location": "foo",
		},
	}

	error := errors.New("oh no")

	gomock.InOrder(
		hookHandler.EXPECT().Setup(),
		hookHandler.EXPECT().InvokeHook(HookRequest{
			Type:  HookPreCreate,
			Event: event,
		}).Return(HookResponse{
			HTTPResponse:   response,
			ChangeFileInfo: change,
		}, nil),
		hookHandler.EXPECT().InvokeHook(HookRequest{
			Type:  HookPreCreate,
			Event: event,
		}).Return(HookResponse{
			HTTPResponse: response,
			RejectUpload: true,
		}, nil),
		hookHandler.EXPECT().InvokeHook(HookRequest{
			Type:  HookPreFinish,
			Event: event,
		}).Return(HookResponse{
			HTTPResponse:   response,
			ChangeFileInfo: change,
		}, nil),
		hookHandler.EXPECT().InvokeHook(HookRequest{
			Type:  HookPreFinish,
			Event: event,
		}).Return(HookResponse{}, error),
		hookHandler.EXPECT().InvokeHook(HookRequest{
			Type:  HookPreTerminate,
			Event: event,
		}).Return(HookResponse{
			HTTPResponse: response,
		}, nil),
		hookHandler.EXPECT().InvokeHook(HookRequest{
			Type:  HookPreTerminate,
			Event: event,
		}).Return(HookResponse{
			HTTPResponse:      response,
			RejectTermination: true,
		}, nil),
	)

	// The hooks are executed asynchronously, so we don't know their execution order.
	hookHandler.EXPECT().InvokeHook(HookRequest{
		Type:  HookPostCreate,
		Event: event,
	})

	hookHandler.EXPECT().InvokeHook(HookRequest{
		Type:  HookPostReceive,
		Event: event,
	})

	hookHandler.EXPECT().InvokeHook(HookRequest{
		Type:  HookPostFinish,
		Event: event,
	})

	hookHandler.EXPECT().InvokeHook(HookRequest{
		Type:  HookPostTerminate,
		Event: event,
	})

	uploadHandler, err := NewHandlerWithHooks(&config, hookHandler, []HookType{HookPreCreate, HookPostCreate, HookPostReceive, HookPostTerminate, HookPostFinish, HookPreFinish, HookPreTerminate})
	a.NoError(err)

	// Successful pre-create hook
	resp_got, change_got, err := config.PreUploadCreateCallback(event)
	a.NoError(err)
	a.Equal(response, resp_got)
	a.Equal(change, change_got)

	// Pre-create hook with rejection
	resp_got, change_got, err = config.PreUploadCreateCallback(event)
	a.Equal(handler.Error{
		ErrorCode: handler.ErrUploadRejectedByServer.ErrorCode,
		Message:   handler.ErrUploadRejectedByServer.Message,
		HTTPResponse: handler.HTTPResponse{
			StatusCode: 200,
			Body:       "foobar",
			Header: handler.HTTPHeader{
				"X-Hello":      "here",
				"Content-Type": "text/plain; charset=utf-8",
			},
		},
	}, err)
	a.Equal(handler.HTTPResponse{}, resp_got)
	a.Equal(handler.FileInfoChanges{}, change_got)

	// Succesful pre-finish hook
	resp_got, err = config.PreFinishResponseCallback(event)
	a.NoError(err)
	a.Equal(response, resp_got)

	// Pre-finish hook with error
	resp_got, err = config.PreFinishResponseCallback(event)
	a.Equal(error, err)
	a.Equal(handler.HTTPResponse{}, resp_got)

	// Successful pre-terminate hook
	resp_got, err = config.PreUploadTerminateCallback(event)
	a.NoError(err)
	a.Equal(response, resp_got)

	// Pre-terminate hook with rejection
	resp_got, err = config.PreUploadTerminateCallback(event)
	a.Equal(handler.Error{
		ErrorCode: handler.ErrUploadTerminationRejected.ErrorCode,
		Message:   handler.ErrUploadTerminationRejected.Message,
		HTTPResponse: handler.HTTPResponse{
			StatusCode: 200,
			Body:       "foobar",
			Header: handler.HTTPHeader{
				"X-Hello":      "here",
				"Content-Type": "text/plain; charset=utf-8",
			},
		},
	}, err)
	a.Equal(handler.HTTPResponse{}, resp_got)

	// Successful post-* hooks
	uploadHandler.CreatedUploads <- event
	uploadHandler.UploadProgress <- event
	uploadHandler.CompleteUploads <- event
	uploadHandler.TerminatedUploads <- event

	// Wait a short amount for all goroutines to settle
	<-time.After(100 * time.Millisecond)
}
