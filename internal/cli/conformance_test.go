package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
	"github.com/vennet-developers/cauthy-sdk-spec/conformance/runner/runnertest"
)

func TestConformanceCertifiesAWellBehavedBackend(t *testing.T) {
	b := runnertest.NewBackend()
	defer b.Close()
	env, out, errOut := testEnv(t, map[string]string{"CONFORMANCE_PASSWORD": b.Password})
	code := Run([]string{"conformance", "--origin", b.URL, "--email", b.Email, "--json", "report.json"}, env)
	if code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if !strings.Contains(out.String(), "0 failed") || strings.Contains(out.String(), b.Password) {
		t.Fatalf("%s", out)
	}
	raw, err := os.ReadFile(filepath.Join(env.Dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Passed, Complete bool
	}
	if err := json.Unmarshal(raw, &rep); err != nil || !rep.Passed || !rep.Complete {
		t.Fatalf("%v %s", err, raw)
	}
	out.Reset()
	if code := Run([]string{"conformance", "--origin", b.URL, "--email", b.Email, "--level", "strict"}, env); code != ExitFail {
		t.Fatalf("a basic backend must fail strict: exit %d", code)
	}
}

func TestConformanceWithoutAccountIsIncomplete(t *testing.T) {
	b := runnertest.NewBackend()
	defer b.Close()
	env, out, _ := testEnv(t, nil)
	if code := Run([]string{"conformance", "--origin", b.URL}, env); code != ExitIncomplete || !strings.Contains(out.String(), "INCOMPLETE") {
		t.Fatalf("exit %d\n%s", code, out)
	}
}

func TestConformanceUsage(t *testing.T) {
	env, _, errOut := testEnv(t, nil)
	for _, args := range [][]string{
		{"conformance"},
		{"conformance", "--origin", "http://localhost:3000", "--email", "a@example.com"},
		{"conformance", "--origin", "http://localhost:3000", "--level", "loose"},
	} {
		errOut.Reset()
		if code := Run(args, env); code != ExitUsage {
			t.Errorf("%v: exit %d %s", args, code, errOut)
		}
	}
}

func TestConformanceStopsWhenTheContextIsDone(t *testing.T) {
	b := runnertest.NewBackend()
	defer b.Close()
	env, _, errOut := testEnv(t, map[string]string{"CONFORMANCE_PASSWORD": b.Password})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	env.Ctx = ctx
	if code := Run([]string{"conformance", "--origin", b.URL, "--email", b.Email}, env); code != ExitFail || !strings.Contains(errOut.String(), "canceled") {
		t.Fatalf("exit %d %s", code, errOut)
	}
	if b.SignIns() != 0 {
		t.Fatal("a canceled run drove the backend")
	}
}

// With --cauthy and the admin token in CAUTHY_ADMIN_TOKEN, level switches
// checks the backend follows the dashboard; the token is never printed.
func TestConformanceSwitches(t *testing.T) {
	b := runnertest.NewStrictBackend()
	defer b.Close()
	c := b.StartCauthy("s3cret-admin")
	env, out, errOut := testEnv(t, map[string]string{"CONFORMANCE_PASSWORD": b.Password, "CAUTHY_ADMIN_TOKEN": "s3cret-admin"})
	code := Run([]string{"conformance", "--origin", b.URL, "--email", b.Email, "--level", "switches",
		"--cauthy", c.URL, "--tenant", c.Tenant, "--client-id", c.ClientID}, env)
	if code != ExitOK || !strings.Contains(out.String(), "switches.password") {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	if strings.Contains(out.String()+errOut.String(), "s3cret-admin") {
		t.Fatal("the admin token was printed")
	}
}

func TestConformanceSwitchesUsage(t *testing.T) {
	env, _, errOut := testEnv(t, nil)
	for _, args := range [][]string{
		{"conformance", "--origin", "http://localhost:3000", "--level", "switches"},
		{"conformance", "--origin", "http://localhost:3000", "--cauthy", "http://localhost:8080", "--tenant", "acme"},
	} {
		errOut.Reset()
		if code := Run(args, env); code != ExitUsage {
			t.Errorf("%v: exit %d %s", args, code, errOut)
		}
	}
}

// --key-env hands the runner the key the backend runs on: the key.* checks
// run, and the key is never printed.
func TestConformanceKeyEnv(t *testing.T) {
	b := runnertest.NewBackend()
	defer b.Close()
	c := b.StartCauthy("admin-token")
	key := b.IssueKey(c.URL)
	k, _ := cauthykey.Parse(key)
	env, out, errOut := testEnv(t, map[string]string{"CONFORMANCE_PASSWORD": b.Password, "CONFORMANCE_KEY": key})
	if code := Run([]string{"conformance", "--origin", b.URL, "--email", b.Email, "--key-env", "CONFORMANCE_KEY"}, env); code != ExitOK {
		t.Fatalf("exit %d\n%s%s", code, out, errOut)
	}
	for _, want := range []string{"key.parses", "key.identity", "key.signs_in"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out.String()+errOut.String(), k.Secret) {
		t.Fatal("the key was printed")
	}
	env, _, errOut = testEnv(t, map[string]string{"CONFORMANCE_PASSWORD": b.Password})
	if code := Run([]string{"conformance", "--origin", b.URL, "--key-env", "CONFORMANCE_KEY"}, env); code != ExitUsage || !strings.Contains(errOut.String(), "--key-env CONFORMANCE_KEY: the variable is empty") {
		t.Fatalf("exit %d %s", code, errOut)
	}
}
