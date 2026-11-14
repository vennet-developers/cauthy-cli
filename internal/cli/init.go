package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/detect"
	"github.com/vennet-developers/cauthy-cli/internal/identity"
	"github.com/vennet-developers/cauthy-cli/internal/link"
	"github.com/vennet-developers/cauthy-cli/internal/pull"
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

Deprecated, until 2.0 (contracts are retired): --contracts <core-contracts
checkout> drafts a Pact link there and writes the contract wiring instead;
with it, and only with it:

  --app-url <url>        each address people sign in at; one is https (production);
                         default: the application's addresses in Cauthy
  --system <id>          default: the system Cauthy names
  --tenant <slug>        default: the application's workspace
  --callback-path <path> default: the return path Cauthy names
  --no-service           no identity of its own (and so no system proof)
  --persistent-sessions  people may ask to stay signed in
  --from <folder|url>    where to pull the graph from; default CONTRACTS_DIR, else
                         ../core-contracts (a folder's scripts/graph.mjs is run with node)
  --to <folder>          where to vendor it (repeat); default contracts
  --no-pull              do not pull the graph
`

// contractOnly are init's flags that draft a contract: taken only with --contracts.
var contractOnly = map[string]bool{"app-url": true, "system": true, "tenant": true, "callback-path": true, "no-service": true,
	"persistent-sessions": true, "from": true, "to": true, "no-pull": true}

func init() { register("init", runInit) }

func runInit(args []string, env Env) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	var urls, to multi
	fs.Var(&urls, "app-url", "")
	fs.Var(&to, "to", "")
	system := fs.String("system", "", "")
	tenant := fs.String("tenant", "", "")
	callback := fs.String("callback-path", "", "")
	noService := fs.Bool("no-service", false, "")
	persistent := fs.Bool("persistent-sessions", false, "")
	lang := fs.String("lang", "", "")
	contracts := fs.String("contracts", "", "")
	from := fs.String("from", "", "")
	noPull := fs.Bool("no-pull", false, "")
	noFiles := fs.Bool("no-files", false, "")
	force := fs.Bool("force", false, "")
	keyEnv := fs.String("key-env", "CAUTHY_KEY", "")
	if code, done := parse(fs, args, env, initHelp); done {
		return code
	}
	if *contracts == "" {
		var drafting []string
		fs.Visit(func(f *flag.Flag) {
			if contractOnly[f.Name] {
				drafting = append(drafting, "--"+f.Name)
			}
		})
		if len(drafting) > 0 {
			return fail(env, usagef("%s drafts a contract, and contracts are retired: init wires CAUTHY_KEY alone (it is taken only with the deprecated --contracts)", strings.Join(drafting, ", ")))
		}
		return initOnKey(env, *lang, *keyEnv, *noFiles, *force)
	}
	contractsRetired(env, "init --contracts")

	app, err := detect.Detect(env.Dir, *lang)
	if err != nil {
		return fail(env, usagef("%v", err))
	}
	framework := detect.FrameworkNames[app.Framework]
	if framework == "" {
		framework = "a framework init does not know"
	}
	language := map[string]string{"node": "Node", "go": "Go"}[app.Lang]
	fmt.Fprintf(env.Out, "Found %s with %s.\n", language, framework)
	if !*noFiles {
		if err := scaffold.Supported(app); err != nil {
			return fail(env, err)
		}
	}

	// The application is already connected: who it is comes from Cauthy.
	key, _, err := readKey(&env, env.Dir, *keyEnv)
	if err != nil {
		return fail(env, err)
	}
	id, err := identity.Ask(env.context(), env.HTTP, key)
	if err != nil {
		return fail(env, fmt.Errorf("asking Cauthy who the application on %s is: %w", *keyEnv, err))
	}

	req := link.Request{System: *system, URLs: urls, CallbackPath: *callback, Tenant: *tenant,
		Service: !*noService, PersistentSessions: *persistent, Language: language, Framework: framework,
		ClientID: id.ClientID, KID: key.Derive().KID}
	if req.ClientID == "" {
		req.ClientID = key.ClientID
	}
	fromCauthy(env, &req, id)
	if req.System == "" {
		req.System = app.System
	}
	if len(urls) == 0 && !hasHTTPS(req.URLs) {
		return fail(env, errors.New("add the production address on the application's card first: a contract needs the address where the application publishes its key"))
	}
	if err := req.Check(); err != nil {
		return fail(env, usagef("%v", err))
	}
	if len(urls) > 0 && len(id.AppURLs) > 0 && !sameSet(req.URLs, id.AppURLs) {
		warn(env, "--app-url %s differs from what Cauthy says (%s)", strings.Join(req.URLs, " "), strings.Join(id.AppURLs, " "))
	}
	// 1. The link, drafted into core-contracts.
	req.To = link.RequestAddress(env.abs(*contracts))
	if err := draftContracts(env, *contracts, req); err != nil {
		return fail(env, err)
	}

	// 2. The application's files.
	plan := scaffold.Plan{}
	if !*noFiles {
		if app.Lang == "go" {
			plan = scaffold.Go(app, req.System)
		} else {
			plan = scaffold.Node(app, req.System)
		}
		written, err := scaffold.Write(env.Dir, plan.Files, *force)
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

	// 3. The graph.
	pulled, pullFailed := false, false
	if !*noPull {
		source := pullSource(env, *from)
		if len(to) == 0 {
			to = multi{"contracts"}
		}
		if _, err := os.Stat(env.abs(source)); err != nil && !pull.IsAddress(source) {
			fmt.Fprintf(env.Out, "\nNo contracts at %s: pull the graph once Vennet merges the link.\n", source)
		} else if line, err := pullInto(env, source, to, false); err != nil {
			fmt.Fprintf(env.Err, "cauthy: contracts pull: %v\n", err)
			pullFailed = true
		} else {
			fmt.Fprintf(env.Out, "\n%s\n", line)
			pulled = true
		}
	}

	nextSteps(env, req, app.Lang, plan, pulled)
	if pullFailed {
		return ExitFail
	}
	return ExitOK
}

// fromCauthy fills what no flag gave from what Cauthy says the application
// is, and warns where a flag says otherwise.
func fromCauthy(env Env, req *link.Request, id *identity.Identity) {
	for _, f := range []struct {
		flag  string
		value *string
		says  string
	}{
		{"--system", &req.System, id.System},
		{"--tenant", &req.Tenant, id.Tenant.Slug},
		{"--callback-path", &req.CallbackPath, id.CallbackPath},
	} {
		switch {
		case *f.value == "":
			*f.value = f.says
		case f.says != "" && *f.value != f.says:
			warn(env, "%s %s differs from what Cauthy says (%s)", f.flag, *f.value, f.says)
		}
	}
	if len(req.URLs) == 0 {
		req.URLs = append([]string(nil), id.AppURLs...)
	}
}

func warn(env Env, format string, a ...any) {
	fmt.Fprintf(env.Err, "cauthy: warning: "+format+"\n", a...)
}

func hasHTTPS(urls []string) bool {
	for _, u := range urls {
		if strings.HasPrefix(u, "https://") {
			return true
		}
	}
	return false
}

// sameSet says whether a and b hold the same addresses, in any order.
func sameSet(a, b []string) bool {
	in := map[string]int{}
	for _, u := range a {
		in[strings.TrimRight(u, "/")]++
	}
	for _, u := range b {
		in[strings.TrimRight(u, "/")]--
	}
	for _, n := range in {
		if n != 0 {
			return false
		}
	}
	return true
}

// pullSource is where the graph is pulled from: --from, else CONTRACTS_DIR,
// else ../core-contracts.
func pullSource(env Env, from string) string {
	if from != "" {
		return from
	}
	return defaultFrom(env)
}

// draftContracts writes the link and the manifest into a core-contracts
// checkout. Neither is ever written over: a link that exists is changed by hand.
func draftContracts(env Env, dir string, req link.Request) error {
	root := env.abs(dir)
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return usagef("no core-contracts checkout at %s", dir)
	}
	subject, err := link.NewSubject(env.Rand)
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Out)
	for _, f := range []scaffold.File{{Path: link.File(req.System), Content: link.YAML(req, subject)}, {Path: link.ManifestFile(req.System), Content: link.Manifest(req)}} {
		full := filepath.Join(root, filepath.FromSlash(f.Path))
		if _, err := os.Lstat(full); err == nil {
			fmt.Fprintf(env.Out, "  kept      %s (it exists: change it by hand)\n", filepath.Join(dir, f.Path))
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := scaffold.CreateNew(full, []byte(f.Content)); err != nil {
			return err
		}
		fmt.Fprintf(env.Out, "  wrote     %s\n", filepath.Join(dir, f.Path))
	}
	return nil
}

func nextSteps(env Env, req link.Request, lang string, plan scaffold.Plan, pulled bool) {
	steps := []string{"In core-contracts: pnpm check, then a pull request. Merging is the approval; Cauthy applies the link on its next deploy."}
	if plan.Install != "" {
		steps = append(steps, "Install: "+plan.Install)
	}
	steps = append(steps, plan.Steps...)
	if pulled {
		steps = append(steps, "Once the link is merged, pull the graph again: npx @cauthyhq/cli contracts pull")
	} else {
		steps = append(steps, "Once the link is merged: npx @cauthyhq/cli contracts pull --from <core-contracts folder or graph address>")
	}
	if lang == "go" {
		steps = append(steps, fmt.Sprintf("The SDK takes the contract (the wiring above does it): cauthy.Options{Contract: &cauthy.Contract{Graph: graph, System: %q}}", req.System))
	} else {
		steps = append(steps, fmt.Sprintf("The SDK takes the contract (the wiring above does it): createCauthy({ contract: { graph, system: '%s' } })", req.System))
	}
	steps = append(steps, "Then deploy with the same CAUTHY_KEY: nothing else to set. Cauthy adopts the application's key for the contract when it applies the link.")
	steps = append(steps, "Check it: cauthy pact check --system "+req.System+", and once deployed: cauthy pact check --system "+req.System+" --live "+req.Production())
	printSteps(env, steps)
}

// printSteps prints the numbered next steps.
func printSteps(env Env, steps []string) {
	fmt.Fprint(env.Out, "\nNext:\n\n")
	for i, s := range steps {
		indented := strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\n   "), "\n   \n", "\n\n")
		fmt.Fprintf(env.Out, "%d. %s\n", i+1, indented)
	}
}

// initOnKey is init without --contracts: the application on CAUTHY_KEY
// alone, as the five-minute guide wires it. No link, no graph.
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
