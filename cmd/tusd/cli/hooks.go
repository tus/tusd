package cli

import (
	"strconv"
	"strings"

	"github.com/tus/tusd/cmd/tusd/cli/hooks"
	"github.com/tus/tusd/pkg/handler"
)

var hookHandler hooks.HookHandler = nil

func hookTypeInSlice(a hooks.HookType, list []hooks.HookType) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func preCreateCallback(event handler.HookEvent) (handler.HTTPResponse, handler.HookResponse, error) {
	return invokeHookSync(hooks.HookPreCreate, event)
}

func preFinishCallback(event handler.HookEvent) (handler.HTTPResponse, error) {
	res, _, err := invokeHookSync(hooks.HookPreFinish, event)
	return res, err
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

		hookHandler = &hooks.FileHook{
			Directory: Flags.FileHooksDir,
		}
	} else if Flags.HttpHooksEndpoint != "" {
		stdout.Printf("Using '%s' as the endpoint for hooks", Flags.HttpHooksEndpoint)

		hookHandler = &hooks.HttpHook{
			Endpoint:       Flags.HttpHooksEndpoint,
			MaxRetries:     Flags.HttpHooksRetry,
			Backoff:        Flags.HttpHooksBackoff,
			ForwardHeaders: strings.Split(Flags.HttpHooksForwardHeaders, ","),
		}
	} else if Flags.GrpcHooksEndpoint != "" {
		stdout.Printf("Using '%s' as the endpoint for gRPC hooks", Flags.GrpcHooksEndpoint)

		hookHandler = &hooks.GrpcHook{
			Endpoint:   Flags.GrpcHooksEndpoint,
			MaxRetries: Flags.GrpcHooksRetry,
			Backoff:    Flags.GrpcHooksBackoff,
		}
	} else if Flags.PluginHookPath != "" {
		stdout.Printf("Using '%s' to load plugin for hooks", Flags.PluginHookPath)

		hookHandler = &hooks.PluginHook{
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
			case event := <-handler.UploadProgress:
				invokeHookAsync(hooks.HookPostReceive, event)
			case event := <-handler.CreatedUploads:
				invokeHookAsync(hooks.HookPostCreate, event)
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

func invokeHookSync(typ hooks.HookType, event handler.HookEvent) (httpRes handler.HTTPResponse, hookRes handler.HookResponse, err error) {

	if !hookTypeInSlice(typ, Flags.EnabledHooks) {
		return httpRes, hookRes, nil
	}

	MetricsHookInvocationsTotal.WithLabelValues(string(typ)).Add(1)

	id := event.Upload.ID
	size := event.Upload.Size

	switch typ {
	case hooks.HookPostFinish:
		logEv(stdout, "UploadFinished", "id", id, "size", strconv.FormatInt(size, 10))
	case hooks.HookPostTerminate:
		logEv(stdout, "UploadTerminated", "id", id)
	}

	if hookHandler == nil {
		return httpRes, hookRes, nil
	}

	if Flags.VerboseOutput {
		logEv(stdout, "HookInvocationStart", "type", string(typ), "id", id)
	}

	hookResponse, err := hookHandler.InvokeHook(hooks.HookRequest{
		Type:  typ,
		Event: event,
	})

	if err != nil {
		logEv(stderr, "HookInvocationError", "type", string(typ), "id", id, "error", err.Error())
		MetricsHookErrorsTotal.WithLabelValues(string(typ)).Add(1)
		return httpRes, hookRes, err
	} else if Flags.VerboseOutput {
		logEv(stdout, "HookInvocationFinish", "type", string(typ), "id", id)
	}

	httpRes = hookResponse.HTTPResponse
	hookRes.UpdatedMetaData = handler.MetaData(hookResponse.UpdatedMetaData)
	hookRes.UpdatedID = hookResponse.UpdatedID

	// If the hook response includes the instruction to reject the upload, reuse the error code
	// and message from ErrUploadRejectedByServer, but also include custom HTTP response values
	if typ == hooks.HookPreCreate && hookResponse.RejectUpload {
		err := handler.ErrUploadRejectedByServer
		err.HTTPResponse = err.HTTPResponse.MergeWith(httpRes)

		return httpRes, hookRes, err
	}

	if typ == hooks.HookPostReceive && hookResponse.StopUpload {
		logEv(stdout, "HookStopUpload", "id", id)

		// TODO: Control response for PATCH request
		event.Upload.StopUpload()
	}

	return httpRes, hookRes, err
}
