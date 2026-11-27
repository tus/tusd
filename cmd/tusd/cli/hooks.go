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
		printStartupLog("Using '%s' for hooks", Flags.FileHooksDir)

		return &file.FileHook{
			Directory: Flags.FileHooksDir,
			Timeout:   Flags.HooksTimeout,
		}
	} else if Flags.HttpHooksEndpoint != "" {
		printStartupLog("Using '%s' as the endpoint for hooks", Flags.HttpHooksEndpoint)

		return &http.HttpHook{
			Endpoint:           Flags.HttpHooksEndpoint,
			MaxRetries:         Flags.HttpHooksRetry,
			Backoff:            Flags.HttpHooksBackoff,
			ForwardHeaders:     strings.Split(Flags.HttpHooksForwardHeaders, ","),
			Timeout:            Flags.HooksTimeout,
			SizeLimit:          Flags.HttpHooksSizeLimit,
			InsecureSkipVerify: Flags.HttpHooksInsecureSkipVerify,
		}
	} else if Flags.GrpcHooksEndpoint != "" {
		printStartupLog("Using '%s' as the endpoint for gRPC hooks", Flags.GrpcHooksEndpoint)

		return &grpc.GrpcHook{
			Endpoint:                        Flags.GrpcHooksEndpoint,
			MaxRetries:                      Flags.GrpcHooksRetry,
			Backoff:                         Flags.GrpcHooksBackoff,
			Secure:                          Flags.GrpcHooksSecure,
			ServerTLSCertificateFilePath:    Flags.GrpcHooksServerTLSCertFile,
			ClientTLSCertificateFilePath:    Flags.GrpcHooksClientTLSCertFile,
			ClientTLSCertificateKeyFilePath: Flags.GrpcHooksClientTLSKeyFile,
			ForwardHeaders:                  strings.Split(Flags.GrpcHooksForwardHeaders, ","),
			Timeout:                         Flags.HooksTimeout,
		}
	} else if Flags.PluginHookPath != "" {
		printStartupLog("Using '%s' to load plugin for hooks", Flags.PluginHookPath)

		return &plugin.PluginHook{
			Path:          Flags.PluginHookPath,
			JSONLogFormat: Flags.LogFormat == "json",
			Timeout:       Flags.HooksTimeout,
		}
	} else {
		return nil
	}
}
