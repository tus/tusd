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

	slogger := slog.New(
		slog.NewTextHandler(logWriter{stdout}, &slog.HandlerOptions{
			Level: level,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				// Remove time attribute, because that is handled by the logger
				if a.Key == slog.TimeKey {
					return slog.Attr{}
				}
				// Rename `msg` to `event`
				if a.Key == slog.MessageKey {
					a.Key = "event"
				}
				return a
			},
		}))
	slog.SetDefault(slogger)
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
