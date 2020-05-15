package cli

import (
	"fmt"
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

func hookCallback(typ hooks.HookType, info handler.HookEvent) error {
	if output, err := invokeHookSync(typ, info, true); err != nil {
		if hookErr, ok := err.(hooks.HookError); ok {
			return hooks.NewHookError(
				fmt.Errorf("%s hook failed: %s", typ, err),
				hookErr.StatusCode(),
				hookErr.Body(),
			)
		}
		return fmt.Errorf("%s hook failed: %s\n%s", typ, err, string(output))
	}

	return nil
}

func preCreateCallback(info handler.HookEvent) error {
	return hookCallback(hooks.HookPreCreate, info)
}

func preFinishCallback(info handler.HookEvent) error {
	return hookCallback(hooks.HookPreFinish, info)
}

func SetupHookMetrics() {
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPostFinish)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPostTerminate)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPostReceive)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPostCreate)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPreCreate)).Add(0)
	MetricsHookErrorsTotal.WithLabelValues(string(hooks.HookPreFinish)).Add(0)
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
			case info := <-handler.CompleteUploads:
				invokeHookAsync(hooks.HookPostFinish, info)
			case info := <-handler.TerminatedUploads:
				invokeHookAsync(hooks.HookPostTerminate, info)
			case info := <-handler.UploadProgress:
				invokeHookAsync(hooks.HookPostReceive, info)
			case info := <-handler.CreatedUploads:
				invokeHookAsync(hooks.HookPostCreate, info)
			}
		}
	}()
}

func invokeHookAsync(typ hooks.HookType, info handler.HookEvent) {
	go func() {
		// Error handling is taken care by the function.
		_, _ = invokeHookSync(typ, info, false)
	}()
}

func invokeHookSync(typ hooks.HookType, info handler.HookEvent, captureOutput bool) ([]byte, error) {
	if !hookTypeInSlice(typ, Flags.EnabledHooks) {
		return nil, nil
	}

	id := info.Upload.ID
	size := info.Upload.Size

	switch typ {
	case hooks.HookPostFinish:
		logEv(stdout, "UploadFinished", "id", id, "size", strconv.FormatInt(size, 10))
	case hooks.HookPostTerminate:
		logEv(stdout, "UploadTerminated", "id", id)
	}

	if hookHandler == nil {
		return nil, nil
	}

	name := string(typ)
	if Flags.VerboseOutput {
		logEv(stdout, "HookInvocationStart", "type", name, "id", id)
	}

	output, returnCode, err := hookHandler.InvokeHook(typ, info, captureOutput)

	if err != nil {
		logEv(stderr, "HookInvocationError", "type", string(typ), "id", id, "error", err.Error())
		MetricsHookErrorsTotal.WithLabelValues(string(typ)).Add(1)
	} else if Flags.VerboseOutput {
		logEv(stdout, "HookInvocationFinish", "type", string(typ), "id", id)
	}

	if typ == hooks.HookPostReceive && Flags.HooksStopUploadCode != 0 && Flags.HooksStopUploadCode == returnCode {
		logEv(stdout, "HookStopUpload", "id", id)

		info.Upload.StopUpload()
	}

	return output, err
}
