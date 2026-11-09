package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
)

// vendored writes the graph where pact check reads it, with Cauthy at issuer.
func vendored(t *testing.T, env Env, issuer string) {
	t.Helper()
	g := strings.Replace(graphFixture, "https://id.example.com", issuer, 1)
	if err := os.MkdirAll(filepath.Join(env.Dir, "contracts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.Dir, "contracts", "graph.json"), []byte(g), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPactCheckOffline(t *testing.T) {
	key, err := cauthykey.Compose("https://id.example.com", testKeyClient, testKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	env, out, _ := testEnv(t, map[string]string{"PACT_ROOT_KEY": strings.Repeat("r", 43), "CAUTHY_KEY": key})
	vendored(t, env, "https://id.example.com")
	code := Run([]string{"pact", "check", "--system", "acme-crm", "--offline"}, env)
	if code != ExitOK {
		t.Fatalf("exit %d\n%s", code, out)
	}
	for _, want := range []string{"sign-in client  pact.acme-crm-signs-in-with-cauthy", "N/A      cauthy.status", "PASS     env.cauthy_key", "ADVISORY env.root_key"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out.String(), strings.Repeat("r", 43)) || strings.Contains(out.String(), testKeySecret) {
		t.Fatal("a key was printed")
	}
}

func TestPactCheckWithoutTokenIsIncomplete(t *testing.T) {
	env, out, _ := testEnv(t, nil)
	vendored(t, env, "https://id.example.com")
	if code := Run([]string{"pact", "check", "--system", "acme-crm"}, env); code != ExitIncomplete || !strings.Contains(out.String(), "SKIP     cauthy.status") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestPactCheckAsksCauthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"system":"acme-crm","in_step":true,"reconciled_at":null,"outside":[],"missing":[]}}`)
	}))
	defer srv.Close()
	env, out, _ := testEnv(t, map[string]string{"CAUTHY_TOKEN": "cty_pat_s3cret"})
	env.HTTP = srv.Client()
	vendored(t, env, srv.URL)
	if code := Run([]string{"pact", "check", "--system", "acme-crm"}, env); code != ExitOK || !strings.Contains(out.String(), "In step with its contract.") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestPactCheckRefusalsAndUsage(t *testing.T) {
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_SECRET": "x"})
	vendored(t, env, "https://id.example.com")
	if code := Run([]string{"pact", "check", "--system", "acme-crm", "--offline"}, env); code != ExitFail || !strings.Contains(out.String(), "FAIL     env.variables_mode") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if code := Run([]string{"pact", "check", "--system", "other"}, env); code != ExitFail || !strings.Contains(errOut.String(), "would refuse to start") {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	for _, args := range [][]string{
		{"pact"}, {"pact", "status"}, {"pact", "check"},
		{"pact", "check", "--system", "acme-crm", "--level", "loose"},
		{"pact", "check", "--system", "acme-crm", "--offline", "--live", "https://crm.example.com"},
		{"pact", "check", "--system", "acme-crm", "--graph", "missing.json"},
	} {
		if code := Run(args, env); code != ExitUsage {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}

func TestPactCheckRefusalCarriesNoControlCharacter(t *testing.T) {
	env, _, errOut := testEnv(t, nil)
	vendored(t, env, "https://id.example.com")
	path := filepath.Join(env.Dir, "contracts", "graph.json")
	raw, _ := os.ReadFile(path)
	evil := strings.ReplaceAll(string(raw), `"link":"acme-crm-signs-in-with-cauthy"`, `"link":"acme\u001b]0;owned\u0007"`)
	evil = strings.ReplaceAll(evil, `"redirect_uris":["https://crm.example.com/api/auth/callback"]`, `"redirect_uris":[]`)
	if err := os.WriteFile(path, []byte(evil), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"pact", "check", "--system", "acme-crm", "--offline"}, env); code != ExitFail || strings.ContainsAny(errOut.String(), "\x1b\x07") || !strings.Contains(errOut.String(), "acme]0;owned") {
		t.Fatalf("exit %d %q", code, errOut)
	}
}
