package cli

import (
	"fmt"
	"net/http"
)

var greeting string

func PrepareGreeting() {
	// Do not show information about metric endpoint, if it is not exposed
	metricsInfo := ""
	if Flags.ExposeMetrics {
		metricsInfo = fmt.Sprintf("- %s - gather statistics to keep tusd running smoothly\n", Flags.MetricsPath)
	}

	greeting = fmt.Sprintf(
		`Welcome to tusd
===============

Congratulations on setting up tusd! Thanks for joining our cause, you have taken
the first step towards making the future of resumable uploading a reality! We
hope you are as excited about this as we are!

While you did an awesome job on getting tusd running, this is just the welcome
message, so let's talk about the places that really matter:

- %s - send your tus uploads to this endpoint
%s- https://github.com/fetlife/tusd/issues - report your bugs here

So quit lollygagging, send over your files and experience the future!

Version = %s
GitCommit = %s
BuildDate = %s
`, Flags.Basepath, metricsInfo, VersionName, GitCommit, BuildDate)
}

func DisplayGreeting(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(greeting))
}
