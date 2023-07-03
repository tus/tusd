package cli

import (
	"strings"

	"github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/hooks"
	"github.com/tus/tusd/v2/pkg/hooks/file"
	"github.com/tus/tusd/v2/pkg/hooks/grpc"
	"github.com/tus/tusd/v2/pkg/hooks/http"
	"github.com/tus/tusd/v2/pkg/hooks/plugin"
)

// TODO: Move some parts into hooks package

var hookHandler hooks.HookHandler = nil

func hookTypeInSlice(a hooks.HookType, list []hooks.HookType) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func preCreateCallback(event handler.HookEvent) (handler.HTTPResponse, handler.FileInfoChanges, error) {
	ok, hookRes, err := invokeHookSync(hooks.HookPreCreate, event)
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

func preFinishCallback(event handler.HookEvent) (handler.HTTPResponse, error) {
	ok, hookRes, err := invokeHookSync(hooks.HookPreFinish, event)
	if !ok || err != nil {
		return handler.HTTPResponse{}, err
	}

	httpRes := hookRes.HTTPResponse
	return httpRes, nil
}

func postReceiveCallback(event handler.HookEvent) {
	ok, hookRes, _ := invokeHookSync(hooks.HookPostReceive, event)
	// invokeHookSync already logs the error, if any occurs. So by checking `ok`, we can ensure
	// that the hook finished successfully
	if !ok {
		return
	}

	if hookRes.StopUpload {
		logEv(stdout, "HookStopUpload", "id", event.Upload.ID)

		// TODO: Control response for PATCH request
		event.Upload.StopUpload()
	}
}

func SetupHookMetrics() {
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPostFinish)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPostTerminate)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPostReceive)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPostCreate)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPreCreate)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPreFinish)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(hooks.HookPostFinish)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(hooks.HookPostTerminate)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(hooks.HookPostReceive)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(hooks.HookPostCreate)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(hooks.HookPreCreate)).Add(0)
	MetricsHookInvocationsTotal.WithLabelValues(string(hooks.HookPreFinish)).Add(0)
}

func SetupPreHooks(config *handler.Config) error {
	if Flags.FileHooksDir != "" {
		stdout.Printf("Using '%s' for hooks", Flags.FileHooksDir)

		hookHandler = &file.FileHook{
			Directory: Flags.FileHooksDir,
		}
	} else if Flags.HttpHooksEndpoint != "" {
		stdout.Printf("Using '%s' as the endpoint for hooks", Flags.HttpHooksEndpoint)

		hookHandler = &http.HttpHook{
			Endpoint:       Flags.HttpHooksEndpoint,
			MaxRetries:     Flags.HttpHooksRetry,
			Backoff:        Flags.HttpHooksBackoff,
			ForwardHeaders: strings.Split(Flags.HttpHooksForwardHeaders, ","),
		}
	} else if Flags.GrpcHooksEndpoint != "" {
		stdout.Printf("Using '%s' as the endpoint for gRPC hooks", Flags.GrpcHooksEndpoint)

		hookHandler = &grpc.GrpcHook{
			Endpoint:   Flags.GrpcHooksEndpoint,
			MaxRetries: Flags.GrpcHooksRetry,
			Backoff:    Flags.GrpcHooksBackoff,
		}
	} else if Flags.PluginHookPath != "" {
		stdout.Printf("Using '%s' to load plugin for hooks", Flags.PluginHookPath)

		hookHandler = &plugin.PluginHook{
			Path: Flags.PluginHookPath,
		}
	} else {
		return nil
	}

	var enabledHooksString []string
	for _, h := range Flags.EnabledHooks {
		enabledHooksString = append(enabledHooksString, string(h))
	}

	stdout.Printf("Enabled hook events: %s", strings.Join(enabledHooksString, ", "))

	if err := hookHandler.Setup(); err != nil {
		return err
	}

	config.PreUploadCreateCallback = preCreateCallback
	config.PreFinishResponseCallback = preFinishCallback

	return nil
}

func SetupPostHooks(handler *handler.Handler) {
	go func() {
		for {
			select {
			case event := <-handler.CompleteUploads:
				invokeHookAsync(hooks.HookPostFinish, event)
			case event := <-handler.TerminatedUploads:
				invokeHookAsync(hooks.HookPostTerminate, event)
			case event := <-handler.CreatedUploads:
				invokeHookAsync(hooks.HookPostCreate, event)
			case event := <-handler.UploadProgress:
				go postReceiveCallback(event)
			}
		}
	}()
}

func invokeHookAsync(typ hooks.HookType, event handler.HookEvent) {
	go func() {
		// Error handling is taken care by the function.
		_, _, _ = invokeHookSync(typ, event)
	}()
}

// invokeHookSync executes a hook of the given type with the given event data. If
// the hook was not executed properly (e.g. an error occurred or not handler is installed),
// `ok` will be false and `res` is not filled. `err` can contain the underlying error.
// If `ok` is true, `res` contains the response as retrieved from the hook.
// Therefore, a caller should always check `ok` and `err` before assuming that the
// hook completed successfully.
func invokeHookSync(typ hooks.HookType, event handler.HookEvent) (ok bool, res hooks.HookResponse, err error) {
	// Stop, if no hook handler is installed or this hook event is not enabled
	if hookHandler == nil || !hookTypeInSlice(typ, Flags.EnabledHooks) {
		return false, hooks.HookResponse{}, nil
	}

	MetricsHookInvocationsTotal.WithLabelValues(string(typ)).Add(1)

	id := event.Upload.ID

	if Flags.VerboseOutput {
		logEv(stdout, "HookInvocationStart", "type", string(typ), "id", id)
	}

	res, err = hookHandler.InvokeHook(hooks.HookRequest{
		Type:  typ,
		Event: event,
	})
	if err != nil {
		// If an error occurs during the hook execution, we log and track the error, but do not
		// return a hook response.
		logEv(stderr, "HookInvocationError", "type", string(typ), "id", id, "error", err.Error())
		MetricsHookErrorsTotal.WithLabelValues(string(typ)).Add(1)
		return false, hooks.HookResponse{}, err
	}

	if Flags.VerboseOutput {
		logEv(stdout, "HookInvocationFinish", "type", string(typ), "id", id)
	}

	return true, res, nil
}
