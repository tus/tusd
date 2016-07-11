package cli

import (
	"fmt"
)

var VersionName = "n/a"
var GitCommit = "n/a"
var BuildDate = "n/a"

func ShowVersion() {
	fmt.Printf("Version: %s\nCommit: %s\nDate: %s\n", VersionName, GitCommit, BuildDate)
}
