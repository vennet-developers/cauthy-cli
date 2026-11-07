// Command cauthy connects an application to Cauthy: init, contracts pull,
// pact check, keys and conformance. See `cauthy help`.
package main

import (
	"os"

	"github.com/vennet-developers/cauthy-cli/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], cli.System()))
}
