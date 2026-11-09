package cli

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/identity"
	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
)

const keyHelp = `cauthy key inspect [--key-env <name>] [--check]

inspect says what the application's CAUTHY_KEY names: the Cauthy it talks
to, its client, its key id, and whether its checksum is intact. It reads
the key from the environment, else from .env or .env.local in this folder,
and never prints its secret.

  --key-env <name>   read this variable instead of CAUTHY_KEY
  --check            also ask Cauthy whether it knows the key

Exits 1 when the key is damaged or refused, or Cauthy does not know it.
`

func init() { register("key", runKey) }

func runKey(args []string, env Env) int {
	if len(args) == 0 {
		fmt.Fprint(env.Err, keyHelp)
		return ExitUsage
	}
	switch args[0] {
	case "--help", "-h":
		fmt.Fprint(env.Out, keyHelp)
		return ExitOK
	case "inspect":
		return runKeyInspect(args[1:], env)
	}
	fmt.Fprintf(env.Err, "cauthy: unknown key command %q\n\n%s", args[0], keyHelp)
	return ExitUsage
}

func runKeyInspect(args []string, env Env) int {
	fs := flag.NewFlagSet("key inspect", flag.ContinueOnError)
	name := fs.String("key-env", "CAUTHY_KEY", "")
	check := fs.Bool("check", false, "")
	if code, done := parse(fs, args, env, keyHelp); done {
		return code
	}
	key, from, err := readKey(&env, env.Dir, *name)
	var refused *keyError
	if errors.As(err, &refused) {
		fmt.Fprintf(env.Out, "%s from %s\n", *name, source(from))
		reason, msg := refused.err.Reason, strings.TrimPrefix(refused.err.Msg, "CAUTHY_KEY: ")
		if reason == cauthykey.ReasonChecksum {
			fmt.Fprintf(env.Out, "  Checksum   damaged (%s): copy it again from the dashboard\n", reason)
		} else {
			fmt.Fprintf(env.Out, "  Refused    %s (%s)\n", msg, reason)
		}
		return ExitFail
	}
	if err != nil {
		return fail(env, err)
	}
	fmt.Fprintf(env.Out, "%s from %s\n", *name, source(from))
	fmt.Fprintf(env.Out, "  Cauthy     %s\n", key.Issuer)
	fmt.Fprintf(env.Out, "  Client     %s\n", key.ClientID)
	fmt.Fprintf(env.Out, "  Key id     %s\n", key.Derive().KID)
	fmt.Fprintf(env.Out, "  Checksum   ok\n")
	if !*check {
		return ExitOK
	}
	id, err := identity.Ask(env.context(), env.HTTP, key)
	if err != nil {
		fmt.Fprintf(env.Out, "  Cauthy knows it   no: %s\n", err)
		return ExitFail
	}
	fmt.Fprintf(env.Out, "  Cauthy knows it   yes (workspace %s, system %s)\n", id.Tenant.Slug, id.System)
	return ExitOK
}

// source names where a key came from, for a sentence.
func source(from string) string {
	if from == "environment" {
		return "the environment"
	}
	return from
}
