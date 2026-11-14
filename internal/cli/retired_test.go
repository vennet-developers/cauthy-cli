package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// retired is what a contract command says first, on stderr.
func retired(what string) string {
	return "cauthy: " + what + " is deprecated: contracts are retired. An application runs on CAUTHY_KEY alone, and the same key keeps its sessions. " +
		"Protect sign-in with the \"Protect sign-in\" switches on the application's card in Cauthy's dashboard.\n"
}

func TestContractCommandsSayContractsAreRetired(t *testing.T) {
	src := contractsFolder(t)
	for _, sub := range []string{"pull", "check"} {
		env, _, errOut := testEnv(t, map[string]string{"CONTRACTS_DIR": src})
		if sub == "check" {
			Run([]string{"contracts", "pull"}, env)
			errOut.Reset()
		}
		if code := Run([]string{"contracts", sub}, env); code != ExitOK {
			t.Fatalf("contracts %s: exit %d %s", sub, code, errOut)
		}
		if !strings.HasPrefix(errOut.String(), retired("contracts "+sub)) {
			t.Fatalf("contracts %s: %q", sub, errOut)
		}
	}

	env, _, errOut := testEnv(t, nil)
	vendored(t, env, "https://id.example.com")
	Run([]string{"pact", "check", "--system", "acme-crm", "--offline"}, env)
	if !strings.HasPrefix(errOut.String(), retired("pact check")) {
		t.Fatalf("pact check: %q", errOut)
	}

	// --help says it too, and warns of nothing.
	env, out, errOut := testEnv(t, nil)
	if code := Run([]string{"pact", "check", "--help"}, env); code != ExitOK || !strings.Contains(out.String(), "Deprecated: contracts are retired") || errOut.Len() != 0 {
		t.Fatalf("exit %d %q %q", code, out, errOut)
	}
}

func TestInitWithContractsSaysContractsAreRetired(t *testing.T) {
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
	if !strings.HasPrefix(errOut.String(), retired("init --contracts")) {
		t.Fatalf("%q", errOut)
	}
}

func TestInitWithoutContractsSaysNothingRetired(t *testing.T) {
	src := contractsFolder(t)
	key := startCauthy(t, acmeCRM)
	env, out, errOut := testEnv(t, map[string]string{"CONTRACTS_DIR": src, "CAUTHY_KEY": key})
	nextApp(t, env)
	if code := Run([]string{"init"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if strings.Contains(errOut.String(), "deprecated") || strings.Contains(out.String(), "deprecated") {
		t.Fatalf("stdout %q stderr %q", out, errOut)
	}
}
