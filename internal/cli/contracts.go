package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/pull"
)

const contractsHelp = `cauthy contracts pull [--from <core-contracts folder or https address>] [--to <folder>]... [--allow-dirty]
cauthy contracts check [--to <folder>]...

pull vendors the Pact graph: graph.json and VERSION in every --to folder
(default: contracts), the same bytes in each, then checks they are
identical. Commit what it writes: the application never fetches its
contract while it runs. --from defaults to CONTRACTS_DIR, else
../core-contracts. A folder given as --from is trusted: pull runs its
scripts/graph.mjs with node (or reads its graph.json when it has none), so
give only a core-contracts checkout you trust. A checkout with uncommitted
changes is refused unless --allow-dirty: the graph an application vendors
is the one of a commit.

check says whether the --to folders hold identical copies (for CI).
`

func init() { register("contracts", runContracts) }

func runContracts(args []string, env Env) int {
	if len(args) == 0 || (args[0] != "pull" && args[0] != "check") {
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			fmt.Fprint(env.Out, contractsHelp)
			return ExitOK
		}
		fmt.Fprint(env.Err, contractsHelp)
		return ExitUsage
	}
	sub := args[0]
	fs := flag.NewFlagSet("contracts "+sub, flag.ContinueOnError)
	var to multi
	fs.Var(&to, "to", "")
	from := fs.String("from", "", "")
	allowDirty := fs.Bool("allow-dirty", false, "")
	if code, done := parse(fs, args[1:], env, contractsHelp); done {
		return code
	}
	if len(to) == 0 {
		to = multi{"contracts"}
	}
	if sub == "check" {
		if *from != "" || *allowDirty {
			return fail(env, usagef("contracts check takes only --to"))
		}
		if err := pull.Check(pullDeps(env), to); err != nil {
			return fail(env, err)
		}
		fmt.Fprintf(env.Out, "%s: identical\n", strings.Join(to, ", "))
		return ExitOK
	}
	line, err := pullInto(env, *from, to, *allowDirty)
	if err != nil {
		return fail(env, err)
	}
	fmt.Fprintln(env.Out, line)
	return ExitOK
}

func pullDeps(env Env) pull.Deps { return pull.Deps{Dir: env.Dir, HTTP: env.HTTP, Exec: env.Exec} }

// defaultFrom is where the contracts are when --from is not given.
func defaultFrom(env Env) string {
	if v := env.Getenv("CONTRACTS_DIR"); v != "" {
		return v
	}
	return "../core-contracts"
}

func pullInto(env Env, from string, to []string, allowDirty bool) (string, error) {
	if from == "" {
		from = defaultFrom(env)
	}
	src, err := pull.Fetch(pullDeps(env), from, allowDirty)
	if err != nil {
		return "", err
	}
	for _, w := range src.Warnings {
		fmt.Fprintf(env.Err, "cauthy: warning: %s\n", w)
	}
	return pull.Write(pullDeps(env), src, to)
}
