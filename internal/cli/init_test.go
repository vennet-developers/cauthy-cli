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

func TestInitAsTheOwner(t *testing.T) {
	src := contractsFolder(t)
	key := startCauthy(t, acmeCRM)
	env, out, errOut := testEnv(t, map[string]string{"CONTRACTS_DIR": src, "CAUTHY_KEY": key})
	nextApp(t, env)
	args := []string{"init"}
	if code := Run(args, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	text := out.String()
	for _, want := range []string{
		"Found Node with Next.js.",
		"Subject: Pact connection request — acme-crm",
		"To: support@vennet.dev\n",
		"1. Send the request above to support@vennet.dev and wait",
		"  wrote     lib/auth.ts",
		"  wrote     app/.well-known/pact-jwks.json/route.ts",
		"  wrote     migrations/20261107120000_cauthy_pact_keys.sql",
		"  added     PACT_ROOT_KEY= in " + scaffold.ExampleFile,
		"graph unversioned → contracts (2 systems, 1 links); every copy identical",
		"Install: npm install @cauthyhq/node pg",
		"cauthy pact check --system acme-crm --live https://crm.example.com",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	if strings.Contains(errOut.String(), "differs from what Cauthy says") {
		t.Errorf("stderr: %s", errOut)
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

// Without a CAUTHY_KEY there is no application to add a contract to.
func TestInitNeedsTheKey(t *testing.T) {
	env, out, errOut := testEnv(t, nil)
	nextApp(t, env)
	if code := Run([]string{"init", "--app-url", "https://crm.example.com"}, env); code != ExitUsage {
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
	if errOut.Len() > 0 {
		t.Errorf("stderr: %s", errOut)
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
	if code := Run([]string{"init", "--no-pull"}, env); code != ExitFail {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(errOut.String(), "add the production address on the application's card first: a contract needs the address where the application publishes its key") {
		t.Fatalf("stderr: %s", errOut)
	}
	if entries, _ := os.ReadDir(env.Dir); len(entries) != 3 {
		t.Fatalf("init wrote %d files before refusing", len(entries)-3)
	}
}

// The connection request says the application is already connected, and
// never carries the key.
func TestInitConnectionRequest(t *testing.T) {
	key := startCauthy(t, acmeCRM)
	k, _ := cauthykey.Parse(key)
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
	nextApp(t, env)
	if code := Run([]string{"init", "--no-pull", "--no-files"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	text := out.String()
	for _, want := range []string{
		"slug acme-crm",
		"production https://crm.example.com; development http://localhost:3000",
		"Node, Next.js",
		"Workspace: acme",
		"already connected with CAUTHY_KEY (client " + testKeyClient + ", key id " + testKeyKID + ")",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
	if strings.Contains(text+errOut.String(), key) || strings.Contains(text+errOut.String(), k.Secret) {
		t.Fatal("the key was printed")
	}
}

// The next steps: merge, pull, pass the contract, deploy with the same key.
func TestInitNextSteps(t *testing.T) {
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
			if code := Run([]string{"init", "--no-pull"}, env); code != ExitOK {
				t.Fatalf("exit %d\n%s%s", code, out, errOut)
			}
			next := nextSection(t, out.String())
			for _, want := range []string{
				"Send the request above",
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
	if code := Run([]string{"init", "--no-pull"}, env); code != ExitOK {
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
	key := startCauthy(t, acmeCRM)
	env, _, errOut = testEnv(t, map[string]string{"CAUTHY_KEY": key})
	nextApp(t, env)
	for _, args := range [][]string{
		{"init", "--app-url", "http://localhost:3000"},
		{"init", "--system", "Acme CRM"},
		{"init", "--contracts", "nowhere", "--no-pull"},
		{"init", "--key-env", "ACME_KEY"},
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
	key := startCauthy(t, acmeCRM)
	env, out, errOut := testEnv(t, map[string]string{"CONTRACTS_DIR": src, "CAUTHY_KEY": key})
	nextApp(t, env)
	if code := Run([]string{"init", "--no-pull", "--no-files"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out.String(), "To: pact@example.com\n") || !strings.Contains(out.String(), "Send the request above to pact@example.com") {
		t.Fatalf("%s", out)
	}
}

func TestInitRefusesNextBefore155(t *testing.T) {
	env, out, errOut := testEnv(t, nil)
	if err := os.WriteFile(filepath.Join(env.Dir, "package.json"), []byte(`{"name":"acme-crm","dependencies":{"next":"15.4.2"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"init", "--app-url", "https://crm.example.com", "--no-pull"}, env); code != ExitFail || !strings.Contains(errOut.String(), "Next.js 15.5 or newer is required (found 15.4)") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if entries, _ := os.ReadDir(env.Dir); len(entries) != 1 {
		t.Fatalf("init wrote %d files before refusing", len(entries)-1)
	}
}
