package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vennet-developers/cauthy-sdk-spec/conformance/runner"
)

const conformanceHelp = `cauthy conformance --origin <app url> [--target <url>] [--email <address>]
                   [--level basic|strict|switches] [--json <file>] [--jwks-path <path>]
                   [--cauthy <url> --tenant <slug> [--client-id <id>] [--switch-wait 10s]]
                   [--key-env <name>]

Certifies a backend against the auth-bff protocol: the suite of
cauthy-sdk-spec, driving the backend as a black box.

  --origin     the application's origin, e.g. http://localhost:3000
  --target     its protocol prefix; default <origin>/api/auth
  --email      a password account in the backend's tenant; without it the
               signed-in checks are skipped (exit 3)
  --level      basic (cauthy-auth 0.x), strict (protocol 1 in full) or
               switches (strict, plus the dashboard switches; needs --cauthy)
  --json       also write the report as JSON
  --jwks-path  the JWKS path the backend serves, if it serves one
  --cauthy     Cauthy's address: the dashboard-switch checks flip the
               tenant's sign-in settings there, check the backend follows,
               and put everything back (https, or http on this machine)
  --tenant     the backend's workspace slug in Cauthy
  --client-id  the backend's OAuth client, for the persistent-sessions switch
  --switch-wait  how long the backend has to follow a switch (default 10s)
  --key-env    the variable holding the CAUTHY_KEY the backend runs on: adds
               the key.* checks (the key parses, Cauthy knows it, a sign-in
               needs nothing else, and with --jwks-path its public half is
               published); never printed

The account's password is read from CONFORMANCE_PASSWORD, and with --cauthy
an admin token of the tenant from CAUTHY_ADMIN_TOKEN; never flags. Use a
test workspace: the switch checks change its settings while they run.

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
	cauthy := fs.String("cauthy", "", "")
	tenant := fs.String("tenant", "", "")
	clientID := fs.String("client-id", "", "")
	switchWait := fs.Duration("switch-wait", 10*time.Second, "")
	keyEnv := fs.String("key-env", "", "")
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
	var adminToken string
	if *cauthy != "" {
		if adminToken = env.Getenv("CAUTHY_ADMIN_TOKEN"); adminToken == "" {
			return fail(env, usagef("set CAUTHY_ADMIN_TOKEN to an admin token of the tenant (never a flag)"))
		}
	}
	var key string
	if *keyEnv != "" {
		if key = env.Getenv(*keyEnv); key == "" {
			return fail(env, usagef("--key-env %s: the variable is empty", *keyEnv))
		}
	}
	res, err := runner.Run(env.context(), runner.Options{Target: *target, Origin: *origin, Email: *email, Password: password, Level: *level, JWKSPath: *jwksPath,
		Cauthy: *cauthy, Tenant: *tenant, AdminToken: adminToken, ClientID: *clientID, SwitchWait: *switchWait, Key: key})
	if errors.Is(err, runner.ErrUsage) {
		return fail(env, usagef("%s", strings.TrimPrefix(err.Error(), runner.ErrUsage.Error()+": ")))
	}
	if err != nil {
		return fail(env, err)
	}
	fmt.Fprint(env.Out, cleanLines(res.Text)) // the details quote the backend's answers
	if *jsonOut != "" {
		if err := os.WriteFile(env.abs(*jsonOut), res.JSON, 0o644); err != nil {
			return fail(env, usagef("%v", err))
		}
	}
	return res.ExitCode()
}
