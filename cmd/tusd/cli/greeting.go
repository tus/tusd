package cli

import (
	"fmt"
	"net/http"
)

var greeting string

func PrepareGreeting() {
	greeting = fmt.Sprintf(
		`Welcome to tusd
===============

Congratulations for setting up tusd! You are now part of the chosen elite and
able to experience the feeling of resumable uploads! We hope you are as excited
as we are (a lot)!

However, there is something you should be aware of: While you got tusd
running (you did an awesome job!), this is the root directory of the server
and tus requests are only accepted at the %s route.

So don't waste time, head over there and experience the future!

Version = %s
GitCommit = %s
BuildDate = %s
`, Flags.Basepath, VersionName, GitCommit, BuildDate)
}

func DisplayGreeting(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(greeting))
}
