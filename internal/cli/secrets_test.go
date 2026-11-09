package cli

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
	"github.com/vennet-developers/cauthy-sdk-spec/conformance/runner/runnertest"
)

// TestNoCommandPrintsASecret runs every command that touches a secret, on its
// happy and its failing paths, and looks for the secrets in what it printed.
func TestNoCommandPrintsASecret(t *testing.T) {
	b := runnertest.NewBackend()
	defer b.Close()
	fakeCauthy := b.StartCauthy("admin-token")
	conformanceKey := b.IssueKey(fakeCauthy.URL)
	rootKey := strings.Repeat("R", 43)
	token := "cty_pat_SENTINEL_TOKEN"
	clientSecret := "SENTINEL_CLIENT_SECRET"
	// The previous key of a rotation: the same client, another test secret.
	previousSecret := "HyAhIiMkJSYnKCkqKywtLi8wMTIzNDU2Nzg5Ojs8PT4"
	previous, err := cauthykey.Compose(testKeyIssuer, testKeyClient, previousSecret)
	if err != nil {
		t.Fatal(err)
	}
	cauthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"errors":[{"message":"bad token %s"}]}`, token) // a server that echoes it
	}))
	defer cauthy.Close()
	vars := map[string]string{
		"PACT_ROOT_KEY": rootKey, "PACT_ROOT_KEY_PREVIOUS": "short", "CAUTHY_TOKEN": token,
		"CAUTHY_CLIENT_SECRET": clientSecret, "CONFORMANCE_PASSWORD": b.Password,
		"CAUTHY_KEY": testKey, "CAUTHY_KEY_PREVIOUS": previous, "CONFORMANCE_KEY": conformanceKey,
	}
	k, _ := cauthykey.Parse(conformanceKey)
	secrets := []string{rootKey, token, clientSecret, b.Password, testKey, testKeySecret, previous, previousSecret, conformanceKey, k.Secret}

	env, out, errOut := testEnv(t, vars)
	// Every address goes to the echoing server: the basic vector's Cauthy
	// (cauthy.api.example.com) is never looked up.
	client := cauthy.Client()
	client.Transport = &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, cauthy.Listener.Addr().String())
	}}
	env.HTTP = client
	vendored(t, env, cauthy.URL)
	nextApp(t, env)
	// Each run's exit code is checked too, so a run that stopped early (and so
	// printed nothing) cannot pass this test by accident.
	runs := []struct {
		args []string
		code int
	}{
		{[]string{"pact", "check", "--system", "acme-crm"}, ExitFail},
		{[]string{"pact", "check", "--system", "acme-crm", "--live", cauthy.URL}, ExitFail},
		{[]string{"conformance", "--origin", b.URL, "--email", b.Email}, ExitOK},
		{[]string{"conformance", "--origin", b.URL, "--email", b.Email, "--level", "strict"}, ExitFail},
		{[]string{"conformance", "--origin", b.URL, "--email", b.Email, "--key-env", "CONFORMANCE_KEY"}, ExitOK},
		{[]string{"keys", "new-root", "--out", "root.key"}, ExitOK},
		{[]string{"key", "inspect"}, ExitOK},
		{[]string{"key", "inspect", "--key-env", "CAUTHY_KEY_PREVIOUS"}, ExitOK},
		{[]string{"key", "inspect", "--check"}, ExitFail},
		{[]string{"init", "--no-pull"}, ExitFail}, // Cauthy cannot be asked
	}
	for _, r := range runs {
		if code := Run(r.args, env); code != r.code {
			t.Errorf("%v: exit %d, want %d", r.args, code, r.code)
		}
	}

	// init's happy path, on a Cauthy that knows the key.
	initVars := map[string]string{"CAUTHY_KEY_PREVIOUS": previous, "PACT_ROOT_KEY": rootKey, "CAUTHY_KEY": startCauthy(t, acmeCRM)}
	initEnv, initOut, initErr := testEnv(t, initVars)
	nextApp(t, initEnv)
	if code := Run([]string{"init", "--no-pull"}, initEnv); code != ExitOK {
		t.Errorf("init: exit %d\n%s%s", code, initOut, initErr)
	}
	secrets = append(secrets, initVars["CAUTHY_KEY"])

	printed := out.String() + errOut.String() + initOut.String() + initErr.String()
	for _, s := range secrets {
		if strings.Contains(printed, s) {
			t.Errorf("a secret was printed: %q", s)
		}
	}
	if !strings.Contains(printed, "CAUTHY_CLIENT_SECRET set") {
		t.Fatalf("the variable's name is said, its value never:\n%s", printed)
	}
}
