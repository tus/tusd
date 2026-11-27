package cli

import (
	"strings"
	"time"

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
			Timeout:   hooksTimeout(),
		}
	} else if Flags.HttpHooksEndpoint != "" {
		printStartupLog("Using '%s' as the endpoint for hooks", Flags.HttpHooksEndpoint)

		return &http.HttpHook{
			Endpoint:           Flags.HttpHooksEndpoint,
			MaxRetries:         Flags.HttpHooksRetry,
			Backoff:            Flags.HttpHooksBackoff,
			ForwardHeaders:     strings.Split(Flags.HttpHooksForwardHeaders, ","),
			Timeout:            httpHooksTimeout(),
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
			Timeout:                         hooksTimeout(),
		}
	} else if Flags.PluginHookPath != "" {
		printStartupLog("Using '%s' to load plugin for hooks", Flags.PluginHookPath)

		return &plugin.PluginHook{
			Path:          Flags.PluginHookPath,
			JSONLogFormat: Flags.LogFormat == "json",
			Timeout:       hooksTimeout(),
		}
	} else {
		return nil
	}
}

// defaultHttpHooksTimeout is the timeout for HTTP hooks if neither -hooks-timeout
// nor -hooks-http-timeout is set. Other hook handlers have no timeout by default
// to remain compatible with previous versions.
const defaultHttpHooksTimeout = 15 * time.Second

// hooksTimeout returns the timeout for gRPC, plugin and file hooks.
func hooksTimeout() time.Duration {
	if Flags.HooksTimeout != nil {
		return *Flags.HooksTimeout
	}
	return 0
}

// httpHooksTimeout returns the timeout for HTTP hooks. The deprecated
// -hooks-http-timeout flag takes precedence over -hooks-timeout.
func httpHooksTimeout() time.Duration {
	if Flags.HttpHooksTimeout != nil {
		return *Flags.HttpHooksTimeout
	}
	if Flags.HooksTimeout != nil {
		return *Flags.HooksTimeout
	}
	return defaultHttpHooksTimeout
}
