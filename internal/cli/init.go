package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/detect"
	"github.com/vennet-developers/cauthy-cli/internal/link"
	"github.com/vennet-developers/cauthy-cli/internal/pull"
	"github.com/vennet-developers/cauthy-cli/internal/scaffold"
)

const initHelp = `cauthy init --app-url <url> [--app-url <url>]... [options]

Connects the application in this folder to Cauthy by contract. It reads the
language and framework (Node: Next.js, TanStack Start, Hono, Express,
Fastify; Go: net/http, chi, Gin, Fiber, Echo) and:

  1. drafts the Pact link: with --contracts it writes the link and the
     manifest into that core-contracts checkout (for Vennet's team);
     otherwise it prints the connection request the owner emails to Vennet;
  2. writes the auth wiring for the framework (@cauthyhq/node or
     cauthy-sdk-go), the cauthy_pact_keys migration, and the line
     PACT_ROOT_KEY= in .env.example (never a value);
  3. runs cauthy contracts pull when the contracts are at hand.

It never writes over a file without --force, and never over a link or a
manifest at all. Run it again at any time: what is there is left alone.

  --app-url <url>        each address people sign in at; one is https (production)
  --system <id>          default: from package.json or go.mod
  --tenant <slug>        the Cauthy tenant; default: the system id
  --callback-path <path> default: /api/auth/callback
  --no-service           no identity of its own (and so no system proof)
  --persistent-sessions  people may ask to stay signed in
  --lang node|go         when the folder has both package.json and go.mod
  --contracts <folder>   a core-contracts checkout to draft the link into
  --from <folder|url>    where to pull the graph from; default CONTRACTS_DIR, else
                         ../core-contracts (a folder's scripts/graph.mjs is run with node)
  --to <folder>          where to vendor it (repeat); default contracts
  --no-pull              do not pull the graph
  --no-files             write nothing into the application
  --force                replace application files that differ
`

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
	if code, done := parse(fs, args, env, initHelp); done {
		return code
	}

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

	req := link.Request{System: *system, URLs: urls, CallbackPath: *callback, Tenant: *tenant,
		Service: !*noService, PersistentSessions: *persistent, Language: language, Framework: framework}
	if req.System == "" {
		req.System = app.System
	}
	if err := req.Check(); err != nil {
		return fail(env, usagef("%v", err))
	}
	// The request goes where Vennet's joining guide says, read from the
	// core-contracts at hand: no address is hard-coded.
	if *contracts != "" {
		req.To = link.RequestAddress(env.abs(*contracts))
	} else if source := pullSource(env, *from); !pull.IsAddress(source) {
		req.To = link.RequestAddress(env.abs(source))
	}

	// 1. The link: drafted into core-contracts, or asked for by email.
	if *contracts != "" {
		if err := draftContracts(env, *contracts, req); err != nil {
			return fail(env, err)
		}
	} else {
		fmt.Fprintf(env.Out, "\nSend this Pact connection request to Vennet (fill in the <…> parts). Vennet writes\nthe link and the manifest, merges them, and answers with the tenant and the link id:\n\n%s", link.ConnectionRequest(req))
	}

	// 2. The application's files.
	plan := scaffold.Plan{}
	var migration []scaffold.File
	if !*noFiles {
		if app.Lang == "go" {
			plan = scaffold.Go(app, req.System)
		} else {
			plan = scaffold.Node(app, req.System)
		}
		if migration, err = scaffold.Migration(env.Dir, env.Now()); err != nil {
			return fail(env, err)
		}
		written, err := scaffold.Write(env.Dir, append(plan.Files, migration...), *force)
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
		fmt.Fprintf(env.Out, "  %-9s PACT_ROOT_KEY= in %s\n", said, scaffold.ExampleFile)
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

	nextSteps(env, req, plan, migration, *contracts != "", pulled)
	if pullFailed {
		return ExitFail
	}
	return ExitOK
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

func nextSteps(env Env, req link.Request, plan scaffold.Plan, migration []scaffold.File, drafted, pulled bool) {
	var steps []string
	if drafted {
		steps = append(steps, "In core-contracts: pnpm check, then a pull request. Merging is the approval; Cauthy applies the link on its next deploy.")
	} else {
		to := req.To
		if to == "" {
			to = link.GuideAddress
		}
		steps = append(steps, "Send the request above to "+to+" and wait for Vennet's answer.")
	}
	if plan.Install != "" {
		steps = append(steps, "Install: "+plan.Install)
	}
	if len(migration) > 0 {
		steps = append(steps, "Run the migration "+migration[0].Path+" with the application's migration tool.")
	}
	steps = append(steps, "Make the root key and put it in the deployment's secret manager as PACT_ROOT_KEY:\n\n  cauthy keys new-root --out pact-root.key")
	steps = append(steps, plan.Steps...)
	if !pulled {
		steps = append(steps, "Once the link is merged: cauthy contracts pull --from <core-contracts folder or graph address>")
	}
	steps = append(steps, "Check it: cauthy pact check --system "+req.System+", and once deployed: cauthy pact check --system "+req.System+" --live "+req.Production())
	fmt.Fprint(env.Out, "\nNext:\n\n")
	for i, s := range steps {
		indented := strings.ReplaceAll(strings.ReplaceAll(s, "\n", "\n   "), "\n   \n", "\n\n")
		fmt.Fprintf(env.Out, "%d. %s\n", i+1, indented)
	}
}
