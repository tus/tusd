package hooks

// TODO: Move hooks into a package in /pkg

import (
	"github.com/tus/tusd/v2/pkg/handler"
)

// HookHandler is the main inferface to be implemented by all hook backends.
type HookHandler interface {
	// Setup is invoked once the hook backend is initalized.
	Setup() error
	// InvokeHook is invoked for every hook that is executed. req contains the
	// corresponding information about the hook type, the involved upload, and
	// causing HTTP request.
	// The return value res allows to stop or reject an upload, as well as modifying
	// the HTTP response. See the documentation for HookResponse for more details.
	// If err is not nil, the value of res will be ignored. err should only be
	// non-nil if the hook failed to complete successfully.
	InvokeHook(req HookRequest) (res HookResponse, err error)
}

// HookRequest contains the information about the hook type, the involved upload,
// and causing HTTP request.
type HookRequest struct {
	// Type is the name of the hook.
	Type HookType
	// Event contains the involved upload and causing HTTP request.
	Event handler.HookEvent
}

// HookResponse is the response after a hook is executed.
type HookResponse struct {
	// HTTPResponse's fields can be filled to modify the HTTP response.
	// This is only possible for pre-create, pre-finish and post-receive hooks.
	// For other hooks this value is ignored.
	// If multiple hooks modify the HTTP response, a later hook may overwrite the
	// modified values from a previous hook (e.g. if multiple post-receive hooks
	// are executed).
	// Example usages: Send an error to the client if RejectUpload/StopUpload are
	// set in the pre-create/post-receive hook. Send more information to the client
	// in the pre-finish hook.
	HTTPResponse handler.HTTPResponse

	// RejectUpload will cause the upload to be rejected and not be created during
	// POST request. This value is only respected for pre-create hooks. For other hooks,
	// it is ignored. Use the HTTPResponse field to send details about the rejection
	// to the client.
	RejectUpload bool

	// ChangeFileInfo can be set to change selected properties of an upload before
	// it has been created. See the handler.FileInfoChanges type for more details.
	// Changes are applied on a per-property basis, meaning that specifying just
	// one property leaves all others unchanged.
	// This value is only respected for pre-create hooks.
	ChangeFileInfo handler.FileInfoChanges

	// StopUpload will cause the upload to be stopped during a PATCH request.
	// This value is only respected for post-receive hooks. For other hooks,
	// it is ignored. Use the HTTPResponse field to send details about the stop
	// to the client.
	StopUpload bool
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
