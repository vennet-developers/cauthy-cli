package identity

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
	"github.com/vennet-developers/cauthy-sdk-spec/conformance/runner/runnertest"
)

func parse(t *testing.T, s string) *cauthykey.Key {
	t.Helper()
	k, err := cauthykey.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestAskAnswersWhoTheKeyIs(t *testing.T) {
	b := runnertest.NewBackend()
	defer b.Close()
	c := b.StartCauthy("admin-token")
	key := parse(t, b.IssueKey(c.URL))

	id, err := Ask(context.Background(), &http.Client{Timeout: 5 * time.Second}, key)
	if err != nil {
		t.Fatal(err)
	}
	if id.ClientID != c.ClientID || id.Tenant.Slug != c.Tenant || id.Tenant.Slug != "acme" {
		t.Fatalf("identity: %+v", id)
	}
	if id.System == "" || len(id.AppURLs) == 0 || id.Issuer != c.URL {
		t.Fatalf("identity: %+v", id)
	}
	if id.Key == nil || id.Key.Kid != key.Derive().KID {
		t.Fatalf("key: %+v", id.Key)
	}
}

func TestAskNamesTheRefusalNotTheKey(t *testing.T) {
	b := runnertest.NewBackend()
	defer b.Close()
	c := b.StartCauthy("admin-token")
	// A key registered at another Cauthy, naming this one.
	other := runnertest.NewBackend()
	defer other.Close()
	raw := other.IssueKey(c.URL)
	key := parse(t, raw)

	_, err := Ask(context.Background(), &http.Client{Timeout: 5 * time.Second}, key)
	var r *Refusal
	if !errors.As(err, &r) || r.Status != http.StatusUnauthorized || r.Code != "invalid_client" {
		t.Fatalf("err = %v", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "401 invalid_client") || !strings.Contains(msg, c.URL) {
		t.Fatalf("message: %q", msg)
	}
	if strings.Contains(msg, key.Secret) || strings.Contains(msg, raw) {
		t.Fatalf("the error names the key: %q", msg)
	}
}

func TestAskCauthyUnreachable(t *testing.T) {
	key := parse(t, cauthykey.ComposeUnchecked("http://127.0.0.1:1", "client_x", "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"))
	_, err := Ask(context.Background(), &http.Client{Timeout: 2 * time.Second}, key)
	if err == nil || !strings.Contains(err.Error(), "http://127.0.0.1:1") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), key.Secret) {
		t.Fatalf("the error names the secret: %q", err)
	}
}
