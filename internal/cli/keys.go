package cli

import (
	"flag"
	"fmt"

	"github.com/vennet-developers/cauthy-cli/internal/keys"
)

const keysHelp = `cauthy keys new-root [--out <file>] [--force]
cauthy keys rotate
cauthy keys reseal

new-root is deprecated: CAUTHY_KEY replaces PACT_ROOT_KEY, and Cauthy's
dashboard makes it on the application's card (Integrate → Applications).
new-root still makes a PACT_ROOT_KEY: 32 random bytes in base64url. Without --out
it prints the key to standard output (and a warning to standard error);
with --out it writes the file readable by its owner only (0600), prints
nothing secret, and never writes over a file unless --force.

rotate and reseal print how to rotate from the running application: the
CLI never holds the root key or the application's database.
`

func init() { register("keys", runKeys) }

const newRootDeprecated = "cauthy: keys new-root is deprecated: CAUTHY_KEY replaces PACT_ROOT_KEY. Make the key on the application's card in Cauthy's dashboard (Integrate → Applications).\n"

func runKeys(args []string, env Env) int {
	if len(args) == 0 {
		fmt.Fprint(env.Err, keysHelp)
		return ExitUsage
	}
	switch args[0] {
	case "--help", "-h":
		fmt.Fprint(env.Out, keysHelp)
		return ExitOK
	case "rotate", "reseal":
		fs := flag.NewFlagSet("keys "+args[0], flag.ContinueOnError)
		if code, done := parse(fs, args[1:], env, keysHelp); done {
			return code
		}
		if args[0] == "rotate" {
			fmt.Fprint(env.Out, keys.RotateHelp)
		} else {
			fmt.Fprint(env.Out, keys.ResealHelp)
		}
		return ExitOK
	case "new-root":
		fs := flag.NewFlagSet("keys new-root", flag.ContinueOnError)
		out := fs.String("out", "", "")
		force := fs.Bool("force", false, "")
		if code, done := parse(fs, args[1:], env, keysHelp); done {
			return code
		}
		fmt.Fprint(env.Err, newRootDeprecated)
		key, err := keys.NewRoot(env.Rand)
		if err != nil {
			return fail(env, err)
		}
		if *out == "" {
			fmt.Fprint(env.Err, keys.RootWarning)
			fmt.Fprintln(env.Out, key)
			return ExitOK
		}
		if err := keys.WriteRoot(env.abs(*out), key, *force); err != nil {
			return fail(env, err)
		}
		fmt.Fprintf(env.Out, "Wrote a new PACT_ROOT_KEY to %s (mode 0600). Put it in the secret manager, then delete the file.\n", *out)
		return ExitOK
	}
	fmt.Fprintf(env.Err, "cauthy: unknown keys command %q\n\n%s", args[0], keysHelp)
	return ExitUsage
}
