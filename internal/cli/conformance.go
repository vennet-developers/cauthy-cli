package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/pact"
	"github.com/vennet-developers/cauthy-sdk-spec/conformance/runner"
)

const conformanceHelp = `cauthy conformance --origin <app url> [--target <url>] [--email <address>]
                   [--level basic|strict] [--json <file>] [--jwks-path <path>]

Certifies a backend against the auth-bff protocol: the suite of
cauthy-sdk-spec, driving the backend as a black box.

  --origin     the application's origin, e.g. http://localhost:3000
  --target     its protocol prefix; default <origin>/api/auth
  --email      a password account in the backend's tenant; without it the
               signed-in checks are skipped (exit 3)
  --level      basic (cauthy-auth 0.x) or strict (protocol 1 in full)
  --json       also write the report as JSON
  --jwks-path  contract mode: the JWKS path the backend serves

The account's password is read from CONFORMANCE_PASSWORD, never a flag.

Exit codes: 0 passed and complete; 1 a check failed; 2 usage; 3 nothing
failed but a check was skipped.
`

func init() { register("conformance", runConformance) }

func runConformance(args []string, env Env) int {
	fs := flag.NewFlagSet("conformance", flag.ContinueOnError)
	origin := fs.String("origin", "", "")
	target := fs.String("target", "", "")
	email := fs.String("email", "", "")
	level := fs.String("level", "basic", "")
	jsonOut := fs.String("json", "", "")
	jwksPath := fs.String("jwks-path", "", "")
	if code, done := parse(fs, args, env, conformanceHelp); done {
		return code
	}
	if *origin == "" {
		return fail(env, usagef("give --origin, the application's origin"))
	}
	if *target == "" {
		*target = strings.TrimRight(*origin, "/") + "/api/auth"
	}
	password := env.Getenv("CONFORMANCE_PASSWORD")
	if *email != "" && password == "" {
		return fail(env, usagef("set CONFORMANCE_PASSWORD to the account's password (never a flag)"))
	}
	res, err := runner.Run(env.context(), runner.Options{Target: *target, Origin: *origin, Email: *email, Password: password, Level: *level, JWKSPath: *jwksPath})
	if errors.Is(err, runner.ErrUsage) {
		return fail(env, usagef("%s", strings.TrimPrefix(err.Error(), runner.ErrUsage.Error()+": ")))
	}
	if err != nil {
		return fail(env, err)
	}
	fmt.Fprint(env.Out, pact.CleanLines(res.Text)) // the details quote the backend's answers
	if *jsonOut != "" {
		if err := os.WriteFile(env.abs(*jsonOut), res.JSON, 0o644); err != nil {
			return fail(env, usagef("%v", err))
		}
	}
	return res.ExitCode()
}
