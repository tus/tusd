package hooks

import (
	"github.com/tus/tusd/pkg/handler"
)

type HookHandler interface {
	Setup() error
	InvokeHook(typ HookType, info handler.HookEvent, captureOutput bool) ([]byte, int, error)
}

type HookType string

const (
	HookPostFinish    HookType = "post-finish"
	HookPostTerminate HookType = "post-terminate"
	HookPostReceive   HookType = "post-receive"
	HookPostCreate    HookType = "post-create"
	HookPreCreate     HookType = "pre-create"
	HookPreFinish     HookType = "pre-finish"
)

var AvailableHooks []HookType = []HookType{HookPreCreate, HookPostCreate, HookPostReceive, HookPostTerminate, HookPostFinish, HookPreFinish}

type hookDataStore struct {
	handler.DataStore
}

type HookError struct {
	error
	statusCode int
	body       []byte
}

func NewHookError(err error, statusCode int, body []byte) HookError {
	return HookError{err, statusCode, body}
}

func (herr HookError) StatusCode() int {
	return herr.statusCode
}

func (herr HookError) Body() []byte {
	return herr.body
}

func (herr HookError) Error() string {
	return herr.error.Error()
}
