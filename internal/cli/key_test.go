package cli

import (
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
	"github.com/vennet-developers/cauthy-sdk-spec/conformance/runner/runnertest"
)

func TestKeyInspect(t *testing.T) {
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": testKey})
	if code := Run([]string{"key", "inspect"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	want := "CAUTHY_KEY from the environment\n" +
		"  Cauthy     https://cauthy.api.example.com\n" +
		"  Client     client_0123456789abcdef0123456789abcdef\n" +
		"  Key id     dVpPrRtlQgOGn_9Q6DE2cgFRjUetrT2WPZCQyM48luc\n" +
		"  Checksum   ok\n"
	if out.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", out, want)
	}
}

func TestKeyInspectFromDotEnvAndOtherName(t *testing.T) {
	env, out, errOut := testEnv(t, nil)
	writeFile(t, env.Dir, ".env", "ACME_KEY="+testKey+"\n")
	if code := Run([]string{"key", "inspect", "--key-env", "ACME_KEY"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.HasPrefix(out.String(), "ACME_KEY from .env\n") || !strings.Contains(out.String(), testKeyKID) {
		t.Fatalf("output:\n%s", out)
	}
}

func TestKeyInspectDamaged(t *testing.T) {
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": damagedKey})
	if code := Run([]string{"key", "inspect"}, env); code != ExitFail {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out.String(), "  Checksum   damaged (key_checksum): copy it again from the dashboard\n") {
		t.Fatalf("output:\n%s", out)
	}
	if strings.Contains(out.String()+errOut.String(), testKeySecret) {
		t.Fatal("the secret was printed")
	}
}

func TestKeyInspectRefused(t *testing.T) {
	// A key with a correct checksum around an issuer no SDK accepts.
	bad := cauthykey.ComposeUnchecked("http://cauthy.example.com", testKeyClient, testKeySecret)
	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": bad})
	if code := Run([]string{"key", "inspect"}, env); code != ExitFail {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out.String(), "key_issuer") {
		t.Fatalf("output:\n%s", out)
	}
	if strings.Contains(out.String()+errOut.String(), testKeySecret) {
		t.Fatal("the secret was printed")
	}
}

func TestKeyInspectNoKey(t *testing.T) {
	env, _, errOut := testEnv(t, nil)
	if code := Run([]string{"key", "inspect"}, env); code != ExitUsage {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(errOut.String(), "No CAUTHY_KEY") {
		t.Fatalf("stderr: %s", errOut)
	}
}

func TestKeyInspectCheck(t *testing.T) {
	b := runnertest.NewBackend()
	defer b.Close()
	c := b.StartCauthy("admin-token")
	key := b.IssueKey(c.URL)
	k, _ := cauthykey.Parse(key)

	env, out, errOut := testEnv(t, map[string]string{"CAUTHY_KEY": key})
	if code := Run([]string{"key", "inspect", "--check"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out.String(), "  Checksum   ok\n  Cauthy knows it   yes (workspace acme, system acme-crm)\n") {
		t.Fatalf("output:\n%s", out)
	}
	if strings.Contains(out.String()+errOut.String(), k.Secret) {
		t.Fatal("the secret was printed")
	}

	// Registered at another Cauthy: this one does not know it.
	other := runnertest.NewBackend()
	defer other.Close()
	unknown := other.IssueKey(c.URL)
	env, out, errOut = testEnv(t, map[string]string{"CAUTHY_KEY": unknown})
	if code := Run([]string{"key", "inspect", "--check"}, env); code != ExitFail {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out.String(), "  Cauthy knows it   no: ") || !strings.Contains(out.String(), "401 invalid_client") {
		t.Fatalf("output:\n%s", out)
	}
	u, _ := cauthykey.Parse(unknown)
	if strings.Contains(out.String()+errOut.String(), u.Secret) {
		t.Fatal("the secret was printed")
	}
}

func TestKeyUsage(t *testing.T) {
	env, out, _ := testEnv(t, nil)
	if code := Run([]string{"key", "--help"}, env); code != ExitOK || !strings.Contains(out.String(), "cauthy key inspect") {
		t.Fatalf("help: exit %d\n%s", code, out)
	}
	env, _, errOut := testEnv(t, nil)
	if code := Run([]string{"key", "show"}, env); code != ExitUsage || !strings.Contains(errOut.String(), `unknown key command "show"`) {
		t.Fatalf("unknown: exit %d\n%s", code, errOut)
	}
}
