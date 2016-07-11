package cli

import (
	"log"
	"os"
)

var stdout = log.New(os.Stdout, "[tusd] ", 0)
var stderr = log.New(os.Stderr, "[tusd] ", 0)
