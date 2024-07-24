// Package hooks allows you to execute hooks based on events emitted from the tusd handler
// using the callbacks and notification channels. The actual hook systems are implemented
// in the subpackages and this package provides the glue betwen the tusd handler and the hook
// system. For example, to use the HTTP-based hook system:
//
//	import (
//		"github.com/tus/tusd/v2/pkg/handler"
//		"github.com/tus/tusd/v2/pkg/hooks"
//		"github.com/tus/tusd/v2/pkg/hooks/http"
//	)
//	config := handler.Config{}
//	hookHandler := http.HttpHook{
//		Endpoint: "https://example.com"
//	}
//	handler, err = hooks.NewHandlerWithHooks(&config, hookHandler, hooks.AvailableHooks)
//
// More details can be found in the documentation at github.com/tus/tusd/docs/hooks.md
package hooks

import (
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/tus/tusd/v2/pkg/handler"
	"golang.org/x/exp/slices"
	"log/slog"
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

// AvailableHooks is a slice of all hooks that are implemented by tusd.
var AvailableHooks []HookType = []HookType{HookPreCreate, HookPostCreate, HookPostReceive, HookPostTerminate, HookPostFinish, HookPreFinish}

func preCreateCallback(event handler.HookEvent, hookHandler HookHandler) (handler.HTTPResponse, handler.FileInfoChanges, error) {
	ok, hookRes, err := invokeHookSync(HookPreCreate, event, hookHandler)
	if !ok || err != nil {
		return handler.HTTPResponse{}, handler.FileInfoChanges{}, err
	}

	httpRes := hookRes.HTTPResponse

	// If the hook response includes the instruction to reject the upload, reuse the error code
	// and message from ErrUploadRejectedByServer, but also include custom HTTP response values.
	if hookRes.RejectUpload {
		err := handler.ErrUploadRejectedByServer
		err.HTTPResponse = err.HTTPResponse.MergeWith(httpRes)

		return handler.HTTPResponse{}, handler.FileInfoChanges{}, err
	}

	// Pass any changes regarding file info from the hook to the handler.
	changes := hookRes.ChangeFileInfo
	return httpRes, changes, nil
}

func preFinishCallback(event handler.HookEvent, hookHandler HookHandler) (handler.HTTPResponse, error) {
	ok, hookRes, err := invokeHookSync(HookPreFinish, event, hookHandler)
	if !ok || err != nil {
		return handler.HTTPResponse{}, err
	}

	httpRes := hookRes.HTTPResponse
	return httpRes, nil
}

func postReceiveCallback(event handler.HookEvent, hookHandler HookHandler) {
	ok, hookRes, _ := invokeHookSync(HookPostReceive, event, hookHandler)
	// invokeHookSync already logs the error, if any occurs. So by checking `ok`, we can ensure
	// that the hook finished successfully
	if !ok {
		return
	}

	if hookRes.StopUpload {
		slog.Info("HookStopUpload", "id", event.Upload.ID)

		event.Upload.StopUpload(hookRes.HTTPResponse)
	}
}

var MetricsHookErrorsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "tusd_hook_errors_total",
		Help: "Total number of execution errors per hook type.",
	},
	[]string{"hooktype"},
)

var MetricsHookInvocationsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "tusd_hook_invocations_total",
		Help: "Total number of invocations per hook type.",
	},
	[]string{"hooktype"},
)

func SetupHookMetrics() {
	MetricsHookErrorsTotal.WithLabelValues(string(HookPostFinish)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(HookPostTerminate)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(HookPostReceive)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(HookPostCreate)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(HookPreCreate)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(HookPreFinish)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(HookPostFinish)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(HookPostTerminate)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(HookPostReceive)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(HookPostCreate)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(HookPreCreate)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(HookPreFinish)).Add(0)
}

func invokeHookAsync(typ HookType, event handler.HookEvent, hookHandler HookHandler) {
	go func() {
		// Error handling is taken care by the function.
		_, _, _ = invokeHookSync(typ, event, hookHandler)
	}()
}

// invokeHookSync executes a hook of the given type with the given event data. If
// the hook was not executed properly (e.g. an error occurred or not handler is installed),
// `ok` will be false and `res` is not filled. `err` can contain the underlying error.
// If `ok` is true, `res` contains the response as retrieved from the hook.
// Therefore, a caller should always check `ok` and `err` before assuming that the
// hook completed successfully.
func invokeHookSync(typ HookType, event handler.HookEvent, hookHandler HookHandler) (ok bool, res HookResponse, err error) {
	MetricsHookInvocationsTotal.WithLabelValues(string(typ)).Add(1)

	id := event.Upload.ID

	slog.Debug("HookInvocationStart", "type", typ, "id", id)

	res, err = hookHandler.InvokeHook(HookRequest{
		Type:  typ,
		Event: event,
	})
	if err != nil {
		// If an error occurs during the hook execution, we log and track the error, but do not
		// return a hook response.
		slog.Error("HookInvocationError", "type", typ, "id", id, "error", err.Error())
		MetricsHookErrorsTotal.WithLabelValues(string(typ)).Add(1)
		return false, HookResponse{}, err
	}

	slog.Debug("HookInvocationFinish", "type", typ, "id", id)

	return true, res, nil
}

// NewHandlerWithHooks creates a tusd request handler, whose notifcation channels and callbacks are configured to
// emit the hooks on the provided hook handler. NewHandlerWithHooks will overwrite the `config.Notify*` and `config.*Callback`
// fields depending on the enabled hooks. These can be controlled via the `enabledHooks` slice. Non-enabled hooks will
// not be emitted.
//
// If you want to create an UnroutedHandler instead of the routed handler, you can first create a routed handler and then
// extract an unrouted one:
//
//	routedHandler := hooks.NewHandlerWithHooks(...)
//	unroutedHandler := routedHandler.UnroutedHandler
//
// Note: NewHandlerWithHooks sets up a goroutine to consume the notfication channels (CompleteUploads, TerminatedUploads,
// CreatedUploads, UploadProgress) on the created handler. These channels must not be consumed by the caller or otherwise
// events might not be passed to the hook handler.
func NewHandlerWithHooks(config *handler.Config, hookHandler HookHandler, enabledHooks []HookType) (*handler.Handler, error) {
	if err := hookHandler.Setup(); err != nil {
		return nil, fmt.Errorf("unable to setup hooks for handler: %s", err)
	}

	// Activate notifications for post-* hooks
	config.NotifyCompleteUploads = slices.Contains(enabledHooks, HookPostFinish)
	config.NotifyTerminatedUploads = slices.Contains(enabledHooks, HookPostTerminate)
	config.NotifyUploadProgress = slices.Contains(enabledHooks, HookPostReceive)
	config.NotifyCreatedUploads = slices.Contains(enabledHooks, HookPostCreate)

	// Install callbacks for pre-* hooks
	if slices.Contains(enabledHooks, HookPreCreate) {
		config.PreUploadCreateCallback = func(event handler.HookEvent) (handler.HTTPResponse, handler.FileInfoChanges, error) {
			return preCreateCallback(event, hookHandler)
		}
	}
	if slices.Contains(enabledHooks, HookPreFinish) {
		config.PreFinishResponseCallback = func(event handler.HookEvent) (handler.HTTPResponse, error) {
			return preFinishCallback(event, hookHandler)
		}
	}

	// Create handler
	handler, err := handler.NewHandler(*config)
	if err != nil {
		return nil, err
	}

	// Listen for notifications for post-* hooks
	go func() {
		for {
			select {
			case event := <-handler.CompleteUploads:
				invokeHookAsync(HookPostFinish, event, hookHandler)
			case event := <-handler.TerminatedUploads:
				invokeHookAsync(HookPostTerminate, event, hookHandler)
			case event := <-handler.CreatedUploads:
				invokeHookAsync(HookPostCreate, event, hookHandler)
			case event := <-handler.UploadProgress:
				go postReceiveCallback(event, hookHandler)
			}
		}
	}()

	return handler, nil
}
