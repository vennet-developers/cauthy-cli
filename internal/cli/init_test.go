package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-cli/internal/scaffold"
	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
)

// nextApp writes a Next.js application into env.Dir.
func nextApp(t *testing.T, env Env) {
	t.Helper()
	for p, content := range map[string]string{
		"package.json":  `{"name":"acme-crm","dependencies":{"next":"16.0.1"}}`,
		"tsconfig.json": "{}",
	} {
		if err := os.WriteFile(filepath.Join(env.Dir, p), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(env.Dir, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// nextSection is what init prints from "Next:" on.
func nextSection(t *testing.T, out string) string {
	t.Helper()
	i := strings.Index(out, "\nNext:\n")
	if i < 0 {
		t.Fatalf("no next steps in\n%s", out)
	}
	return out[i:]
}

// init wires the application on CAUTHY_KEY alone: no link, no graph, no
// contract, whatever core-contracts is at hand.
func TestInitOnKeyAlone(t *testing.T) {
	src := contractsFolder(t)
	key := startCauthy(t, acmeCRM)
	k, _ := cauthykey.Parse(key)
	env, out, errOut := testEnv(t, map[string]string{"CONTRACTS_DIR": src, "CAUTHY_KEY": key})
	nextApp(t, env)
	args := []string{"init"}
	if code := Run(args, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	text := out.String()
	for _, want := range []string{
		"Found Node with Next.js.",
		"CAUTHY_KEY from the environment\n",
		"  Client     " + testKeyClient + "\n",
		"  Key id     " + testKeyKID + "\n",
		"  Workspace  acme\n",
		"  System     acme-crm\n",
		"  Addresses  http://localhost:3000 https://crm.example.com\n",
		"  wrote     lib/auth.ts",
		"  wrote     app/api/auth/[...cauthy]/route.ts",
		"  added     CAUTHY_KEY= in " + scaffold.ExampleFile,
		"Install: npm install @cauthyhq/node\n",
		"open http://localhost:3000/api/auth/login",
		"Deploy with the same CAUTHY_KEY",
		"\"Protect sign-in\" switches",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	for _, not := range []string{"contract", "Pact", "pact", "graph", "Sign-in    "} {
		if strings.Contains(text+errOut.String(), not) {
			t.Errorf("%q in\n%s%s", not, text, errOut)
		}
	}
	if strings.Contains(text+errOut.String(), k.Secret) || strings.Contains(text+errOut.String(), key) {
		t.Fatal("the key was printed")
	}
	auth, _ := os.ReadFile(filepath.Join(env.Dir, "lib", "auth.ts"))
	if !strings.Contains(string(auth), "export const auth = createCauthy()\n") || strings.Contains(string(auth), "contract") {
		t.Fatalf("%s", auth)
	}
	for _, none := range []string{"contracts", "app/.well-known", "migrations"} {
		if _, err := os.Stat(filepath.Join(env.Dir, none)); err == nil {
			t.Errorf("init wrote %s", none)
		}
	}

	// Again: nothing changes, nothing is added twice.
	out.Reset()
	if code := Run(args, env); code != ExitOK {
		t.Fatalf("second run: exit %d\n%s", code, errOut)
	}
	if strings.Contains(out.String(), "  wrote ") || !strings.Contains(out.String(), "  unchanged lib/auth.ts") || !strings.Contains(out.String(), "  present   CAUTHY_KEY=") {
		t.Fatalf("second run:\n%s", out)
	}
	example, _ := os.ReadFile(filepath.Join(env.Dir, scaffold.ExampleFile))
	if strings.Count(string(example), "CAUTHY_KEY=") != 1 || strings.Contains(string(example), "PACT_ROOT_KEY") {
		t.Fatalf("%q", example)
	}

	// A file the person changed is kept.
	if err := os.WriteFile(filepath.Join(env.Dir, "lib", "auth.ts"), []byte("// mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	Run(args, env)
	if !strings.Contains(out.String(), "  kept      lib/auth.ts (it differs; --force replaces it)") {
		t.Fatalf("%s", out)
	}
}

// An application once connected by contract: its key is the link's service
// client, and init says which client people sign in through.
func TestInitSaysTheSignInClient(t *testing.T) {
	who := acmeCRM
	who.SignIn = "pact.acme-crm-signs-in-with-cauthy"
	key := startCauthy(t, who)
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
	nextApp(t, env)
	if code := Run([]string{"init", "--no-files"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out.String(), "  Sign-in    pact.acme-crm-signs-in-with-cauthy (people sign in through it; the SDK reads it from Cauthy)\n") {
		t.Fatalf("%s", out)
	}
	if entries, _ := os.ReadDir(env.Dir); len(entries) != 3 {
		t.Fatalf("--no-files wrote %d files", len(entries)-3)
	}
}

// The contract flags draft a contract: without --contracts they are refused.
func TestInitRefusesContractFlagsWithoutContracts(t *testing.T) {
	key := startCauthy(t, acmeCRM)
	env, _, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
	nextApp(t, env)
	for _, args := range [][]string{
		{"init", "--app-url", "https://crm.example.com"},
		{"init", "--system", "acme-crm"},
		{"init", "--no-pull"},
		{"init", "--to", "contracts"},
	} {
		errOut.Reset()
		if code := Run(args, env); code != ExitUsage || !strings.Contains(errOut.String(), args[1]+" drafts a contract, and contracts are retired") {
			t.Errorf("%v: exit %d %s", args, code, errOut)
		}
	}
	if entries, _ := os.ReadDir(env.Dir); len(entries) != 3 {
		t.Fatalf("init wrote %d files before refusing", len(entries)-3)
	}
}

// Without a CAUTHY_KEY there is no application to wire.
func TestInitNeedsTheKey(t *testing.T) {
	env, out, errOut := testEnv(t, nil)
	nextApp(t, env)
	if code := Run([]string{"init"}, env); code != ExitUsage {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(errOut.String(), "No CAUTHY_KEY: connect the application in Cauthy's dashboard (Integrate → Applications) and put the line in .env") {
		t.Fatalf("stderr: %s", errOut)
	}
	if entries, _ := os.ReadDir(env.Dir); len(entries) != 3 {
		t.Fatalf("init wrote %d files without a key", len(entries)-3)
	}
}

// What Cauthy says the application is fills the link: no --system, --tenant
// or --app-url.
func TestInitFromWhatCauthySays(t *testing.T) {
	key := startCauthy(t, acmeCRM)
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
	nextApp(t, env)
	cc := filepath.Join(t.TempDir(), "core-contracts")
	if err := os.MkdirAll(cc, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"init", "--contracts", cc, "--no-pull", "--no-files"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	linkYAML, err := os.ReadFile(filepath.Join(cc, "links", "acme-crm-signs-in-with-cauthy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"  tenant: acme\n",
		"\nservice: {}\n",
		"  require_system_proof: auto\n",
		"  require_verified_email: true\n",
		"    - http://localhost:3000/api/auth/callback\n    - https://crm.example.com/api/auth/callback\n",
	} {
		if !strings.Contains(string(linkYAML), want) {
			t.Errorf("link: missing %q in\n%s", want, linkYAML)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(cc, "systems", "acme-crm.yaml"))
	if err != nil || !strings.Contains(string(manifest), "jwks_url: https://crm.example.com/.well-known/pact-jwks.json\n") {
		t.Fatalf("manifest: %v\n%s", err, manifest)
	}
	// Nothing on stderr but that contracts are retired.
	if rest := strings.TrimPrefix(errOut.String(), retired("init --contracts")); rest != "" {
		t.Errorf("stderr: %s", rest)
	}
}

// A flag still wins over what Cauthy says, with a warning.
func TestInitFlagsOverrideWhatCauthySays(t *testing.T) {
	key := startCauthy(t, acmeCRM)
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
	nextApp(t, env)
	cc := filepath.Join(t.TempDir(), "core-contracts")
	if err := os.MkdirAll(cc, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"init", "--system", "other", "--contracts", cc, "--no-pull", "--no-files"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if _, err := os.Stat(filepath.Join(cc, "links", "other-signs-in-with-cauthy.yaml")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "--system other differs from what Cauthy says (acme-crm)") {
		t.Fatalf("stderr: %s", errOut)
	}
}

// A contract needs the address where the application publishes its key.
func TestInitNeedsAnHTTPSAddress(t *testing.T) {
	key := startCauthy(t, whoCauthySays{System: "acme-crm", Tenant: "acme", AppURLs: []string{"http://localhost:3000"}})
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
	nextApp(t, env)
	cc := filepath.Join(t.TempDir(), "core-contracts")
	if err := os.MkdirAll(cc, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"init", "--contracts", cc, "--no-pull"}, env); code != ExitFail {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(errOut.String(), "add the production address on the application's card first: a contract needs the address where the application publishes its key") {
		t.Fatalf("stderr: %s", errOut)
	}
	if entries, _ := os.ReadDir(env.Dir); len(entries) != 3 {
		t.Fatalf("init wrote %d files before refusing", len(entries)-3)
	}
}

// The next steps by contract: merge, pull, pass the contract, deploy with
// the same key.
func TestInitNextStepsByContract(t *testing.T) {
	for _, tc := range []struct {
		name, mod, wiring string
	}{
		{"node", "", "createCauthy({ contract: { graph, system: 'acme-crm' } })"},
		{"go", "module github.com/acme/acme-crm\n\ngo 1.27\n\nrequire github.com/go-chi/chi/v5 v5.2.0\n", `cauthy.Options{Contract: &cauthy.Contract{Graph: graph, System: "acme-crm"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := startCauthy(t, acmeCRM)
			env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
			if tc.mod != "" {
				writeFile(t, env.Dir, "go.mod", tc.mod)
			} else {
				nextApp(t, env)
			}
			cc := filepath.Join(t.TempDir(), "core-contracts")
			if err := os.MkdirAll(cc, 0o755); err != nil {
				t.Fatal(err)
			}
			if code := Run([]string{"init", "--contracts", cc, "--no-pull"}, env); code != ExitOK {
				t.Fatalf("exit %d\n%s%s", code, out, errOut)
			}
			next := nextSection(t, out.String())
			for _, want := range []string{
				"In core-contracts: pnpm check",
				"npx @cauthyhq/cli contracts pull",
				tc.wiring,
				"deploy with the same CAUTHY_KEY",
			} {
				if !strings.Contains(next, want) {
					t.Errorf("missing %q in\n%s", want, next)
				}
			}
			for _, not := range []string{"PACT_ROOT_KEY", "keys new-root"} {
				if strings.Contains(next, not) {
					t.Errorf("%q in\n%s", not, next)
				}
			}
		})
	}
}

func TestInitAsVennet(t *testing.T) {
	key := startCauthy(t, acmeCRM)
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
	nextApp(t, env)
	cc := filepath.Join(t.TempDir(), "core-contracts")
	if err := os.MkdirAll(cc, 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"init", "--contracts", cc, "--no-pull", "--no-files"}
	if code := Run(args, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	linkYAML, err := os.ReadFile(filepath.Join(cc, "links", "acme-crm-signs-in-with-cauthy.yaml"))
	if err != nil || !strings.Contains(string(linkYAML), "require_system_proof: auto") {
		t.Fatalf("%v\n%s", err, linkYAML)
	}
	if _, err := os.Stat(filepath.Join(cc, "systems", "acme-crm.yaml")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Pact connection request") {
		t.Fatal("Vennet's run prints no request")
	}
	out.Reset()
	if code := Run(args, env); code != ExitOK || !strings.Contains(out.String(), "(it exists: change it by hand)") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	again, _ := os.ReadFile(filepath.Join(cc, "links", "acme-crm-signs-in-with-cauthy.yaml"))
	if string(again) != string(linkYAML) {
		t.Fatal("the link was written over")
	}
}

func TestInitGo(t *testing.T) {
	key := startCauthy(t, acmeCRM)
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
	if err := os.WriteFile(filepath.Join(env.Dir, "go.mod"), []byte("module github.com/acme/acme-crm\n\ngo 1.27\n\nrequire github.com/go-chi/chi/v5 v5.2.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"init"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out.String(), "Found Go with chi.") || !strings.Contains(out.String(), "  wrote     internal/cauthyauth/cauthyauth.go") ||
		!strings.Contains(out.String(), `r.Handle("/api/auth/*", auth)`) || strings.Contains(out.String(), "JWKSPath") {
		t.Fatalf("%s", out)
	}
	wiring, _ := os.ReadFile(filepath.Join(env.Dir, "internal", "cauthyauth", "cauthyauth.go"))
	if !strings.Contains(string(wiring), "cauthy.New(cauthy.Options{})") || strings.Contains(string(wiring), "graph") {
		t.Fatalf("%s", wiring)
	}
}

func TestInitUsage(t *testing.T) {
	env, _, errOut := testEnv(t, nil)
	if code := Run([]string{"init"}, env); code != ExitUsage || !strings.Contains(errOut.String(), "no package.json or go.mod") {
		t.Fatalf("exit %d %s", code, errOut)
	}
	key := startCauthy(t, acmeCRM)
	env, _, errOut = testEnv(t, map[string]string{"CAUTHY_KEY": key})
	nextApp(t, env)
	for _, args := range [][]string{
		{"init", "--contracts", "nowhere", "--app-url", "http://localhost:3000"},
		{"init", "--contracts", "nowhere", "--system", "Acme CRM"},
		{"init", "--contracts", "nowhere", "--no-pull"},
		{"init", "--key-env", "ACME_KEY"},
	} {
		errOut.Reset()
		if code := Run(args, env); code != ExitUsage {
			t.Errorf("%v: exit %d %s", args, code, errOut)
		}
	}
}

func TestInitRefusesNextBefore155(t *testing.T) {
	env, out, errOut := testEnv(t, nil)
	if err := os.WriteFile(filepath.Join(env.Dir, "package.json"), []byte(`{"name":"acme-crm","dependencies":{"next":"15.4.2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"init"}, env); code != ExitFail || !strings.Contains(errOut.String(), "Next.js 15.5 or newer is required (found 15.4)") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if entries, _ := os.ReadDir(env.Dir); len(entries) != 1 {
		t.Fatalf("init wrote %d files before refusing", len(entries)-1)
	}
}
