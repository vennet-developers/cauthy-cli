package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// graphFixture is Acme CRM linked to Cauthy, as core-contracts writes it.
const graphFixture = `{"version":1,"nodes":[
 {"id":"cauthy","type":"system","url":"https://id.example.com","tenants":[{"id":"74851970-8771-438e-a13a-9fb50161b721","slug":"acme"}]},
 {"id":"acme-crm","type":"system","url":"https://crm.example.com","jwks_url":"https://crm.example.com/.well-known/pact-jwks.json","tenants":[]}],
 "edges":[{"from":"acme-crm","relation":"links","to":"cauthy","link":"acme-crm-signs-in-with-cauthy",
  "identity":{"issuer":"cauthy","tenant":"acme","tenant_id":"74851970-8771-438e-a13a-9fb50161b721","subject":"d14c807d-a226-4a8c-91ad-147fa825ba0c"},
  "sign_in":{"client_id":"pact.acme-crm-signs-in-with-cauthy","redirect_uris":["https://crm.example.com/api/auth/callback"],
   "require_system_proof":false,"system_proof":"auto","require_verified_email":true,"persistent_sessions":false},
  "service":{"client_id":"pact.acme-crm-signs-in-with-cauthy.service","system":"acme-crm","jwks_url":"https://crm.example.com/.well-known/pact-jwks.json"}}]}`

// contractsFolder makes a folder holding graph.json, outside git.
func contractsFolder(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "core-contracts")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "graph.json"), []byte(graphFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestContractsPullAndCheck(t *testing.T) {
	src := contractsFolder(t)
	env, out, errOut := testEnv(t, map[string]string{"CONTRACTS_DIR": src})
	if code := Run([]string{"contracts", "pull", "--to", "contracts", "--to", "web/contracts"}, env); code != ExitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out.String(), "graph unversioned → contracts, web/contracts (2 systems, 1 links); every copy identical") {
		t.Fatalf("%s", out)
	}
	if !strings.Contains(errOut.String(), "cauthy: warning: "+src+" is not a git checkout") {
		t.Fatalf("why it is unversioned goes to stderr: %q", errOut)
	}
	out.Reset()
	if code := Run([]string{"contracts", "check", "--to", "contracts", "--to", "web/contracts"}, env); code != ExitOK || !strings.Contains(out.String(), "identical") {
		t.Fatalf("exit %d %s %s", code, out, errOut)
	}
	if err := os.WriteFile(filepath.Join(env.Dir, "web", "contracts", "graph.json"), []byte(`{"nodes":[],"edges":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"contracts", "check", "--to", "contracts", "--to", "web/contracts"}, env); code != ExitFail || !strings.Contains(errOut.String(), "hold different graphs") {
		t.Fatalf("exit %d %s", code, errOut)
	}
}

func TestContractsUsage(t *testing.T) {
	env, _, errOut := testEnv(t, nil)
	env.Exec = func(string, string, ...string) ([]byte, error) { return nil, errors.New("no git") }
	for _, args := range [][]string{{"contracts"}, {"contracts", "push"}, {"contracts", "check", "--from", "x"}, {"contracts", "pull", "--nope"}} {
		errOut.Reset()
		if code := Run(args, env); code != ExitUsage {
			t.Errorf("%v: exit %d %s", args, code, errOut)
		}
	}
	// An input that cannot be read is exit 2, as a usage error.
	if code := Run([]string{"contracts", "pull", "--from", "nowhere"}, env); code != ExitUsage || !strings.Contains(errOut.String(), "no contracts repository") {
		t.Fatalf("exit %d %s", code, errOut)
	}
	if code := Run([]string{"contracts", "check", "--to", "missing"}, env); code != ExitUsage || !strings.Contains(errOut.String(), "missing has no graph.json") {
		t.Fatalf("exit %d %s", code, errOut)
	}
	env, out, _ := testEnv(t, nil)
	if code := Run([]string{"contracts", "--help"}, env); code != ExitOK || !strings.Contains(out.String(), "scripts/graph.mjs with node") {
		t.Fatalf("the help says what --from <folder> runs:\n%s", out)
	}
}
