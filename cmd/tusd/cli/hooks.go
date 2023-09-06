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

func getHookHandler(config *handler.Config) hooks.HookHandler {
	if Flags.FileHooksDir != "" {
		stdout.Printf("Using '%s' for hooks", Flags.FileHooksDir)

		return &file.FileHook{
			Directory: Flags.FileHooksDir,
		}
	} else if Flags.HttpHooksEndpoint != "" {
		stdout.Printf("Using '%s' as the endpoint for hooks", Flags.HttpHooksEndpoint)

		return &http.HttpHook{
			Endpoint:       Flags.HttpHooksEndpoint,
			MaxRetries:     Flags.HttpHooksRetry,
			Backoff:        Flags.HttpHooksBackoff,
			ForwardHeaders: strings.Split(Flags.HttpHooksForwardHeaders, ","),
		}
	} else if Flags.GrpcHooksEndpoint != "" {
		stdout.Printf("Using '%s' as the endpoint for gRPC hooks", Flags.GrpcHooksEndpoint)

		return &grpc.GrpcHook{
			Endpoint:   Flags.GrpcHooksEndpoint,
			MaxRetries: Flags.GrpcHooksRetry,
			Backoff:    Flags.GrpcHooksBackoff,
		}
	} else if Flags.PluginHookPath != "" {
		stdout.Printf("Using '%s' to load plugin for hooks", Flags.PluginHookPath)

		return &plugin.PluginHook{
			Path: Flags.PluginHookPath,
		}
	} else {
		return nil
	}
}
