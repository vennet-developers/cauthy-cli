package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/vennet-developers/cauthy-cli/internal/graph"
	"github.com/vennet-developers/cauthy-cli/internal/pact"
	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
)

const pactHelp = `cauthy pact check --system <id> [--graph <file>] [--link <id>] [--offline]
                  [--live <app url>] [--base-path /api/auth] [--level basic|strict]

Says what the application's contract is, as its SDK will read it, and
whether the environment would make it refuse to start: CAUTHY_KEY parses
and names the contract's Cauthy, and no variable it replaces is set beside
it (PACT_ROOT_KEY is deprecated). Never prints a variable's value.

With CAUTHY_TOKEN (a personal access token with oauth_clients:manage) it asks
Cauthy how the system stands (GET /v1/pact/status), and the keys registered
for it against the ones it publishes. Without it that check is skipped and
the run exits 3, unless --offline says not to ask.

--live probes the deployed application: its jwks_url publishes the public
half of CAUTHY_KEY (its Ed25519 key, by its key id; RS256 keys only without
CAUTHY_KEY) and no private member, <base-path>/session answers the protocol
(with Cauthy-Protocol: 1 at level strict), and <base-path>/login sends
people to Cauthy with the contract's client and PKCE.

  --graph   default contracts/graph.json (cauthy contracts pull writes it)
  --link    the link, when the system has several with Cauthy

Exit codes: 0 ok; 1 a check failed or the application would refuse to
start; 2 usage, or the graph cannot be read; 3 a check was skipped.

Deprecated: contracts are retired. An application runs on CAUTHY_KEY
alone; protect sign-in with the "Protect sign-in" switches on the
application's card in Cauthy's dashboard.
`

func init() { register("pact", runPact) }

func runPact(args []string, env Env) int {
	if len(args) == 0 || args[0] != "check" {
		if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
			fmt.Fprint(env.Out, pactHelp)
			return ExitOK
		}
		fmt.Fprint(env.Err, pactHelp)
		return ExitUsage
	}
	fs := flag.NewFlagSet("pact check", flag.ContinueOnError)
	system := fs.String("system", "", "")
	graphFile := fs.String("graph", "contracts/graph.json", "")
	linkID := fs.String("link", "", "")
	offline := fs.Bool("offline", false, "")
	live := fs.String("live", "", "")
	base := fs.String("base-path", "/api/auth", "")
	level := fs.String("level", "basic", "")
	if code, done := parse(fs, args[1:], env, pactHelp); done {
		return code
	}
	contractsRetired(env, "pact check")
	if *system == "" {
		return fail(env, usagef("give --system, the application's id in the graph"))
	}
	if *level != "basic" && *level != "strict" {
		return fail(env, usagef("--level is basic or strict"))
	}
	if *offline && *live != "" {
		return fail(env, usagef("--offline and --live ask opposite things: give one"))
	}
	raw, err := os.ReadFile(env.abs(*graphFile))
	if err != nil {
		return fail(env, usagef("cannot read %s (cauthy contracts pull writes it)", *graphFile))
	}
	g, err := graph.Parse(raw)
	if err != nil {
		return fail(env, usagef("%s: %v", *graphFile, err))
	}
	c, err := g.Read(*system, *linkID)
	var refusal *graph.RefusalError
	if errors.As(err, &refusal) {
		fmt.Fprintf(env.Err, "cauthy: the application would refuse to start: %s\n", pact.Clean(refusal.Reason))
		return ExitFail
	}
	if err != nil {
		return fail(env, err)
	}
	fmt.Fprint(env.Out, pact.Describe(c))

	r := &pact.Report{}
	pact.Offline(r, c, env.Getenv)
	if status := pact.CheckStatus(r, env.HTTP, c, env.Getenv("CAUTHY_TOKEN"), *offline); status != "" {
		fmt.Fprint(env.Out, "\n"+status)
	}
	if *live != "" {
		kid := ""
		if k, err := cauthykey.Parse(env.Getenv("CAUTHY_KEY")); err == nil {
			kid = k.Derive().KID
		}
		pact.Live(r, env.HTTP, c, *live, *base, *level, kid)
	}
	fmt.Fprint(env.Out, "\n"+r.Text())
	return r.ExitCode()
}
