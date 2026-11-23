// Command claudes is a wrapper around claude agents.
package main

import (
	"os"

	"github.com/uvw/claudes/internal/cli"
	"github.com/uvw/claudes/internal/procs"
)

func main() {
	procs.RunHelper()
	os.Exit(cli.Main(os.Args[1:]))
}
