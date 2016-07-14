package cli

import (
	"fmt"
	"net/http"
)

var greeting string

func PrepareGreeting() {
	// @todo: Introduce configurable metrics endpoint.
	// See: https://github.com/tus/tusd/issues/52
	greeting = fmt.Sprintf(
		`Welcome to tusd
===============

Congratulations on setting up tusd! Thanks for joining our cause, you have taken the first step towards making the future of resumable uploading a reality! We hope you are as excited about this as we are!

While you did an awesome job on getting tusd running, this is just the welcome message, so let's talk about the places that really matter:

- %s - send your tus uploads to this endpoint
- %s - gather statistics to keep tusd running smoothly
- https://github.com/tus/tusd/issues - report your bugs here

So quit lollygagging, send over your files and experience the future!

Version = %s
GitCommit = %s
BuildDate = %s
`, Flags.Basepath, "/metrics", VersionName, GitCommit, BuildDate)
}

func DisplayGreeting(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(greeting))
}
