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
// contract.
func TestInitOnKeyAlone(t *testing.T) {
	key := startCauthy(t, acmeCRM)
	k, _ := cauthykey.Parse(key)
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
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

// When people sign in through a client other than the key's, init says which.
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

// The contract flags were removed with contracts: init knows none of them.
func TestInitRefusesTheRemovedContractFlags(t *testing.T) {
	key := startCauthy(t, acmeCRM)
	env, _, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
	nextApp(t, env)
	for _, args := range [][]string{
		{"init", "--contracts", "../core-contracts"},
		{"init", "--app-url", "https://crm.example.com"},
		{"init", "--system", "acme-crm"},
		{"init", "--tenant", "acme"},
		{"init", "--callback-path", "/api/auth/callback"},
		{"init", "--no-service"},
		{"init", "--persistent-sessions"},
		{"init", "--from", "../core-contracts"},
		{"init", "--to", "contracts"},
		{"init", "--no-pull"},
	} {
		errOut.Reset()
		if code := Run(args, env); code != ExitUsage || !strings.Contains(errOut.String(), "flag provided but not defined: "+strings.TrimPrefix(args[1], "-")) {
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
		{"init", "--lang", "node", "extra"},
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
