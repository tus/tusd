package cli

import (
	"errors"
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

func preCreateCallback(event handler.HookEvent) (handler.HTTPResponse, error) {
	return invokeHookSync(hooks.HookPreCreate, event)
}

func preFinishCallback(event handler.HookEvent) (handler.HTTPResponse, error) {
	return invokeHookSync(hooks.HookPreFinish, event)
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
	// if Flags.FileHooksDir != "" {
	// 	stdout.Printf("Using '%s' for hooks", Flags.FileHooksDir)

	// 	hookHandler = &hooks.FileHook{
	// 		Directory: Flags.FileHooksDir,
	// 	}
	// } else
	if Flags.HttpHooksEndpoint != "" {
		stdout.Printf("Using '%s' as the endpoint for hooks", Flags.HttpHooksEndpoint)

		hookHandler = &hooks.HttpHook{
			Endpoint:       Flags.HttpHooksEndpoint,
			MaxRetries:     Flags.HttpHooksRetry,
			Backoff:        Flags.HttpHooksBackoff,
			ForwardHeaders: strings.Split(Flags.HttpHooksForwardHeaders, ","),
		}
		// } else if Flags.GrpcHooksEndpoint != "" {
		// 	stdout.Printf("Using '%s' as the endpoint for gRPC hooks", Flags.GrpcHooksEndpoint)

		// 	hookHandler = &hooks.GrpcHook{
		// 		Endpoint:   Flags.GrpcHooksEndpoint,
		// 		MaxRetries: Flags.GrpcHooksRetry,
		// 		Backoff:    Flags.GrpcHooksBackoff,
		// 	}
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
		_, _ = invokeHookSync(typ, event)
	}()
}

func invokeHookSync(typ hooks.HookType, event handler.HookEvent) (httpRes handler.HTTPResponse, err error) {
	if !hookTypeInSlice(typ, Flags.EnabledHooks) {
		return httpRes, nil
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
		return httpRes, nil
	}

	if Flags.VerboseOutput {
		logEv(stdout, "HookInvocationStart", "type", string(typ), "id", id)
	}

	hookRes, err := hookHandler.InvokeHook(hooks.HookRequest{
		Type:  typ,
		Event: event,
	})

	if err != nil {
		//err = fmt.Errorf("%s hook failed: %s", typ, err)
		logEv(stderr, "HookInvocationError", "type", string(typ), "id", id, "error", err.Error())
		MetricsHookErrorsTotal.WithLabelValues(string(typ)).Add(1)
	} else if Flags.VerboseOutput {
		logEv(stdout, "HookInvocationFinish", "type", string(typ), "id", id)
	}

	// IDEA: PreHooks work like this: error return value does carry HTTP response information for error
	// Instead the additional HTTP response return value

	httpRes = hookRes.HTTPResponse

	if hookRes.Error != "" {
		// TODO: Is this actually useful?
		return httpRes, errors.New(hookRes.Error)
	}

	// If the hook response includes the instruction to reject the upload, reuse the error code
	// and message from ErrUploadRejectedByServer, but also include custom HTTP response values
	if typ == hooks.HookPreCreate && hookRes.RejectUpload {
		err := handler.ErrUploadRejectedByServer
		err.HTTPResponse = err.HTTPResponse.MergeWith(httpRes)

		return httpRes, err
	}

	if typ == hooks.HookPostReceive && hookRes.StopUpload {
		logEv(stdout, "HookStopUpload", "id", id)

		// TODO: Control response for PATCH request
		event.Upload.StopUpload()
	}

	return httpRes, err
}
