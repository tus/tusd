package hooks

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
	// Error indicates whether a fault occurred while processing the hook request.
	// If Error is an empty string, no fault is assumed.
	Error string

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
