package cli

import (
	"log"
	"os"

	"github.com/tus/tusd/pkg/handler"
)

var stdout = log.New(os.Stdout, "[tusd] ", log.LstdFlags|log.Lmicroseconds)
var stderr = log.New(os.Stderr, "[tusd] ", log.LstdFlags|log.Lmicroseconds)

func logEv(logOutput *log.Logger, eventName string, details ...string) {
	handler.LogEvent(logOutput, eventName, details...)
}
