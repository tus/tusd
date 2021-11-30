package hooks

// TODO: Move hooks into a package in /pkg

import (
	"github.com/tus/tusd/pkg/handler"
)

type HookHandler interface {
	Setup() error
	InvokeHook(req HookRequest) (res HookResponse, err error)
}

type HookRequest struct {
	Type  HookType
	Event handler.HookEvent
}

type HookResponse struct {
	HTTPResponse handler.HTTPResponse

	RejectUpload bool
	StopUpload   bool
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
