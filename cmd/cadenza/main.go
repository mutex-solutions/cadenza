// Command cadenza is what coding agents run to show their work and ask their
// human questions. See docs/cadenza.md, or run `cadenza help`.
package main

import (
	"os"

	"github.com/mutex-solutions/cadenza/internal/cli"
)

// Version is set at build time with -ldflags "-X main.Version=…".
var Version = "dev"

func main() {
	os.Exit(cli.NewApp(Version).Run(os.Args[1:]))
}
