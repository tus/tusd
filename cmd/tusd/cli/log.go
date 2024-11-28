package cli

import (
	"log"
	"os"

	"golang.org/x/exp/slog"
)

var stdout = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
var stderr = log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds)

func SetupStructuredLogger() {
	level := slog.LevelInfo
	if Flags.VerboseOutput {
		level = slog.LevelDebug
	}
	replaceAttrFunc := func(groups []string, a slog.Attr) slog.Attr {
		// Remove time attribute, because that is handled by the logger
		if a.Key == slog.TimeKey {
			return slog.Attr{}
		}
		// Rename `msg` to `event`
		if a.Key == slog.MessageKey {
			a.Key = "event"
		}
		return a
	}

	var handler slog.Handler
	switch Flags.LogFormat {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	default:
		handler = slog.NewTextHandler(logWriter{log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)}, &slog.HandlerOptions{
			Level:       level,
			ReplaceAttr: replaceAttrFunc,
		})
	}

	slog.SetDefault(slog.New(handler))

	if Flags.LogFormat == "json" {
		// stdout and stderr helpers need to be overwritten to use slog
		stdout = log.Default()
		stderr = log.Default()
	}
}

// logWriter is an io.Writer that forwards all input to the given log.Logger,
// which can add its timestamp and prefix.
type logWriter struct {
	logger *log.Logger
}

func (l logWriter) Write(msg []byte) (int, error) {
	l.logger.Print(string(msg))
	return len(msg), nil
}

func printStartupLogLine(msg string, args ...interface{}) {
	// Check if the flag allows startup logs
	if Flags.ShowStartupLogs {
		stdout.Printf("[STARTUP] "+msg, args...)
	}
}
