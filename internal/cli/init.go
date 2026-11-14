package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/detect"
	"github.com/vennet-developers/cauthy-cli/internal/identity"
	"github.com/vennet-developers/cauthy-cli/internal/scaffold"
)

const initHelp = `cauthy init [options]

Wires an application to Cauthy on CAUTHY_KEY alone, as the five-minute
guide does. It reads the key (from the environment, else from .env or
.env.local in this folder; never printed), asks Cauthy who the application
is and says what the key names (Cauthy, client, workspace, system,
addresses), reads the language and framework (Node: Next.js, TanStack
Start, Hono, Express, Fastify; Go: net/http, chi, Gin, Fiber, Echo), and
writes the auth wiring for it (@cauthyhq/node's createCauthy(), or
cauthy-sdk-go's cauthy.New) and the line CAUTHY_KEY= in .env.example (never
a value). Security is the dashboard's switches: nothing else to set.

It never writes over a file without --force. Run it again at any time:
what is there is left alone.

  --key-env <name>       read this variable instead of CAUTHY_KEY
  --lang node|go         when the folder has both package.json and go.mod
  --no-files             write nothing into the application
  --force                replace application files that differ
`

func init() { register("init", runInit) }

func runInit(args []string, env Env) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	lang := fs.String("lang", "", "")
	noFiles := fs.Bool("no-files", false, "")
	force := fs.Bool("force", false, "")
	keyEnv := fs.String("key-env", "CAUTHY_KEY", "")
	if code, done := parse(fs, args, env, initHelp); done {
		return code
	}
	return initOnKey(env, *lang, *keyEnv, *noFiles, *force)
}

// printSteps prints the numbered next steps.
func printSteps(env Env, steps []string) {
	fmt.Fprint(env.Out, "\nNext:\n\n")
	for i, s := range steps {
		indented := strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\n   "), "\n   \n", "\n\n")
		fmt.Fprintf(env.Out, "%d. %s\n", i+1, indented)
	}
}

// initOnKey wires the application on CAUTHY_KEY alone, as the five-minute
// guide does.
func initOnKey(env Env, lang, keyEnv string, noFiles, force bool) int {
	app, err := detect.Detect(env.Dir, lang)
	if err != nil {
		return fail(env, usagef("%v", err))
	}
	framework := detect.FrameworkNames[app.Framework]
	if framework == "" {
		framework = "a framework init does not know"
	}
	language := map[string]string{"node": "Node", "go": "Go"}[app.Lang]
	fmt.Fprintf(env.Out, "Found %s with %s.\n", language, framework)
	if !noFiles {
		if err := scaffold.Supported(app); err != nil {
			return fail(env, err)
		}
	}

	key, from, err := readKey(&env, env.Dir, keyEnv)
	if err != nil {
		return fail(env, err)
	}
	id, err := identity.Ask(env.context(), env.HTTP, key)
	if err != nil {
		return fail(env, fmt.Errorf("asking Cauthy who the application on %s is: %w", keyEnv, err))
	}
	fmt.Fprintf(env.Out, "\n%s from %s\n", keyEnv, source(from))
	fmt.Fprintf(env.Out, "  Cauthy     %s\n", key.Issuer)
	fmt.Fprintf(env.Out, "  Client     %s\n", key.ClientID)
	if id.SignInClientID != "" && id.SignInClientID != key.ClientID {
		fmt.Fprintf(env.Out, "  Sign-in    %s (people sign in through it; the SDK reads it from Cauthy)\n", id.SignInClientID)
	}
	fmt.Fprintf(env.Out, "  Key id     %s\n", key.Derive().KID)
	fmt.Fprintf(env.Out, "  Workspace  %s\n", id.Tenant.Slug)
	fmt.Fprintf(env.Out, "  System     %s\n", id.System)
	if len(id.AppURLs) > 0 {
		fmt.Fprintf(env.Out, "  Addresses  %s\n", strings.Join(id.AppURLs, " "))
	}

	plan := scaffold.Plan{}
	if !noFiles {
		if app.Lang == "go" {
			plan = scaffold.GoKey(app)
		} else {
			plan = scaffold.NodeKey(app)
		}
		written, err := scaffold.Write(env.Dir, plan.Files, force)
		if err != nil {
			return fail(env, err)
		}
		fmt.Fprintln(env.Out)
		for _, w := range written {
			note := ""
			if w.Action == "kept" {
				note = " (it differs; --force replaces it)"
			}
			fmt.Fprintf(env.Out, "  %-9s %s%s\n", w.Action, w.Path, note)
		}
		said, err := scaffold.EnvExample(env.Dir)
		if err != nil {
			return fail(env, err)
		}
		fmt.Fprintf(env.Out, "  %-9s CAUTHY_KEY= in %s\n", said, scaffold.ExampleFile)
	}

	var steps []string
	if plan.Install != "" {
		steps = append(steps, "Install: "+plan.Install)
	}
	steps = append(steps, plan.Steps...)
	steps = append(steps,
		"Deploy with the same CAUTHY_KEY in the hosting provider's variables (production gets its own key on the application's card): nothing else to set.",
		"Raise security from Cauthy's dashboard, with no code and no deploy: sign-in methods, second step, sessions, and the \"Protect sign-in\" switches on the application's card.")
	printSteps(env, steps)
	return ExitOK
}
