package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-cli/internal/scaffold"
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

func TestInitAsTheOwner(t *testing.T) {
	src := contractsFolder(t)
	env, out, errOut := testEnv(t, map[string]string{"CONTRACTS_DIR": src})
	nextApp(t, env)
	args := []string{"init", "--app-url", "https://crm.example.com", "--app-url", "http://localhost:3000"}
	if code := Run(args, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	text := out.String()
	for _, want := range []string{
		"Found Node with Next.js.",
		"Subject: Pact connection request — acme-crm",
		"To: the address in Vennet's joining guide (core-contracts, docs/joining-the-network.md)",
		"1. Send the request above to the address in Vennet's joining guide",
		"  wrote     lib/auth.ts",
		"  wrote     app/.well-known/pact-jwks.json/route.ts",
		"  wrote     migrations/20261107120000_cauthy_pact_keys.sql",
		"  added     PACT_ROOT_KEY= in " + scaffold.ExampleFile,
		"graph unversioned → contracts (2 systems, 1 links); every copy identical",
		"Install: npm install @cauthyhq/node pg",
		"cauthy keys new-root --out pact-root.key",
		"cauthy pact check --system acme-crm --live https://crm.example.com",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}

	// Again: nothing changes, nothing is added twice.
	out.Reset()
	if code := Run(args, env); code != ExitOK {
		t.Fatalf("second run: exit %d\n%s", code, errOut)
	}
	if strings.Contains(out.String(), "  wrote ") || !strings.Contains(out.String(), "  unchanged lib/auth.ts") || !strings.Contains(out.String(), "  present   PACT_ROOT_KEY=") {
		t.Fatalf("second run:\n%s", out)
	}
	example, _ := os.ReadFile(filepath.Join(env.Dir, scaffold.ExampleFile))
	if strings.Count(string(example), "PACT_ROOT_KEY=") != 1 {
		t.Fatalf("%q", example)
	}
	if migrations, _ := os.ReadDir(filepath.Join(env.Dir, "migrations")); len(migrations) != 1 {
		t.Fatalf("%d migrations", len(migrations))
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

func TestInitAsVennet(t *testing.T) {
	env, out, errOut := testEnv(t, nil)
	nextApp(t, env)
	cc := filepath.Join(t.TempDir(), "core-contracts")
	if err := os.MkdirAll(cc, 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"init", "--app-url", "https://crm.example.com", "--contracts", cc, "--no-pull", "--no-files"}
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
	env, out, errOut := testEnv(t, nil)
	if err := os.WriteFile(filepath.Join(env.Dir, "go.mod"), []byte("module github.com/acme/acme-crm\n\ngo 1.27\n\nrequire github.com/go-chi/chi/v5 v5.2.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"init", "--app-url", "https://crm.example.com", "--no-pull"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out.String(), "Found Go with chi.") || !strings.Contains(out.String(), "  wrote     internal/cauthyauth/cauthyauth.go") {
		t.Fatalf("%s", out)
	}
}

func TestInitUsage(t *testing.T) {
	env, _, errOut := testEnv(t, nil)
	if code := Run([]string{"init", "--app-url", "https://crm.example.com"}, env); code != ExitUsage || !strings.Contains(errOut.String(), "no package.json or go.mod") {
		t.Fatalf("exit %d %s", code, errOut)
	}
	nextApp(t, env)
	for _, args := range [][]string{
		{"init"},
		{"init", "--app-url", "http://localhost:3000"},
		{"init", "--app-url", "https://crm.example.com", "--system", "Acme CRM"},
		{"init", "--app-url", "https://crm.example.com", "--contracts", "nowhere", "--no-pull"},
	} {
		errOut.Reset()
		if code := Run(args, env); code != ExitUsage {
			t.Errorf("%v: exit %d %s", args, code, errOut)
		}
	}
}

// The request goes to the address the joining guide of the contracts at hand
// publishes; none is hard-coded.
func TestInitReadsTheRequestAddressFromTheGuide(t *testing.T) {
	src := contractsFolder(t)
	guide := filepath.Join(src, "docs", "joining-the-network.md")
	if err := os.MkdirAll(filepath.Dir(guide), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guide, []byte("Send it to [Vennet](mailto:pact@example.com).\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env, out, errOut := testEnv(t, map[string]string{"CONTRACTS_DIR": src})
	nextApp(t, env)
	if code := Run([]string{"init", "--app-url", "https://crm.example.com", "--no-pull", "--no-files"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out.String(), "To: pact@example.com\n") || !strings.Contains(out.String(), "Send the request above to pact@example.com") {
		t.Fatalf("%s", out)
	}
}
