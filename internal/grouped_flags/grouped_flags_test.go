package grouped_flags

import (
	"flag"
	"os"
	"time"
)

func ExampleNewFlagGroupSet() {
	os.Args = []string{"tusd", "-h"}

	fs := NewFlagGroupSet(flag.ContinueOnError)
	fs.SetOutput(os.Stdout)

	var host string
	var behindProxy bool
	var maxSize int64
	var timeout time.Duration

	fs.AddGroup("Listening options", func(f *flag.FlagSet) {
		f.StringVar(&host, "host", "0.0.0.0", "Host to bind HTTP server to")
		f.BoolVar(&behindProxy, "behind-proxy", false, "Respect X-Forwarded-* and similar headers which may be set by proxies")
	})

	fs.AddGroup("Upload protocol options", func(f *flag.FlagSet) {
		f.Int64Var(&maxSize, "max-size", 0, "Maximum size of a single upload in bytes")
	})

	fs.AddGroup("Timeout options", func(f *flag.FlagSet) {
		f.DurationVar(&timeout, "read-timeout", 60*time.Second, "Network read timeout. If the tusd does not receive data for this duration, it will consider the connection dead. A zero value means that network reads will not time out.")
	})

	fs.Parse()

	// Output:
	// Usage of tusd:
	//
	// Listening options:
	//   -behind-proxy
	//     	Respect X-Forwarded-* and similar headers which may be set by proxies
	//   -host string
	//     	Host to bind HTTP server to (default "0.0.0.0")
	//
	// Upload protocol options:
	//   -max-size int
	//     	Maximum size of a single upload in bytes
	//
	// Timeout options:
	//   -read-timeout duration
	//     	Network read timeout. If the tusd does not receive data for this duration, it will consider the connection dead. A zero value means that network reads will not time out. (default 1m0s)
	//
}
