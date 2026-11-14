// Command cauthy connects an application to Cauthy: init, key inspect and
// conformance. See `cauthy help`.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/vennet-developers/cauthy-cli/internal/cli"
)

func main() {
	// Ctrl-C ends a long run (cauthy conformance) without starting another request.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	env := cli.System()
	env.Ctx = ctx
	code := cli.Run(os.Args[1:], env)
	stop()
	os.Exit(code)
}
