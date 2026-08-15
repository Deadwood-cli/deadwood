// Command deadwood safely identifies and deletes local Git branches that are
// dead on the remote, without ever risking unmerged work.
package main

import (
	"os"

	"github.com/Deadwood-cli/deadwood/internal/cli"
)

// Overwritten at link time by goreleaser's default ldflags.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	info := cli.BuildInfo{Version: version, Commit: commit, Date: date}
	if err := cli.Execute(info); err != nil {
		os.Exit(1)
	}
}
