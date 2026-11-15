// Command claudes is a wrapper around claude agents.
package main

import (
	"os"

	"github.com/uvw/claudes/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:]))
}
