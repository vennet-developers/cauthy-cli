package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-sdk-spec/conformance/runner/runnertest"
)

// TestNoCommandPrintsASecret runs every command that touches a secret, on its
// happy and its failing paths, and looks for the secrets in what it printed.
func TestNoCommandPrintsASecret(t *testing.T) {
	b := runnertest.NewBackend()
	defer b.Close()
	rootKey := strings.Repeat("R", 43)
	token := "cty_pat_SENTINEL_TOKEN"
	clientSecret := "SENTINEL_CLIENT_SECRET"
	cauthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintf(w, `{"errors":[{"message":"bad token %s"}]}`, token) // a server that echoes it
	}))
	defer cauthy.Close()
	vars := map[string]string{
		"PACT_ROOT_KEY": rootKey, "PACT_ROOT_KEY_PREVIOUS": "short", "CAUTHY_TOKEN": token,
		"CAUTHY_CLIENT_SECRET": clientSecret, "CONFORMANCE_PASSWORD": b.Password,
	}
	secrets := []string{rootKey, token, clientSecret, b.Password}

	env, out, errOut := testEnv(t, vars)
	env.HTTP = cauthy.Client()
	vendored(t, env, cauthy.URL)
	nextApp(t, env)
	runs := [][]string{
		{"pact", "check", "--system", "acme-crm"},
		{"pact", "check", "--system", "acme-crm", "--live", cauthy.URL},
		{"conformance", "--origin", b.URL, "--email", b.Email},
		{"conformance", "--origin", b.URL, "--email", b.Email, "--level", "strict"},
		{"keys", "new-root", "--out", "root.key"},
		{"init", "--app-url", "https://crm.example.com", "--no-pull"},
	}
	for _, args := range runs {
		Run(args, env)
	}
	printed := out.String() + errOut.String()
	for _, s := range secrets {
		if strings.Contains(printed, s) {
			t.Errorf("a secret was printed: %q", s)
		}
	}
	if !strings.Contains(printed, "CAUTHY_CLIENT_SECRET set") {
		t.Fatalf("the variable's name is said, its value never:\n%s", printed)
	}
}
