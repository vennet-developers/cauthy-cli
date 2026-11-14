package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
)

// whoCauthySays is what a fake Cauthy answers at /v1/connect/self.
type whoCauthySays struct {
	System  string
	Tenant  string
	AppURLs []string
	// SignIn is the client people sign in through; "" is the key's.
	SignIn string
}

// startCauthy starts a fake Cauthy that knows one application and answers
// the CAUTHY_KEY it was made with (the test secret, never a real key): its
// token endpoint checks the key's assertion, and /v1/connect/self answers
// who. The key's issuer is the fake's own (loopback) address.
func startCauthy(t *testing.T, who whoCauthySays) string {
	t.Helper()
	var d *cauthykey.Derived
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/oauth/token":
			_ = r.ParseForm()
			claims, err := cauthykey.VerifyAssertion(d.Public, r.PostForm.Get("client_assertion"))
			if err != nil || claims.Sub != testKeyClient || r.PostForm.Get("client_id") != testKeyClient {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"system-token","token_type":"Bearer","expires_in":600}`))
		case "/v1/connect/self":
			if r.Header.Get("Authorization") != "Bearer system-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":"unauthorized"}`))
				return
			}
			issuer := "http://" + r.Host
			signIn := who.SignIn
			if signIn == "" {
				signIn = testKeyClient
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"issuer": issuer, "audience": issuer,
				"tenant": map[string]any{"id": "00000000-0000-4000-8000-0000000000a1", "slug": who.Tenant},
				"system": who.System, "client_id": testKeyClient, "sign_in_client_id": signIn,
				"app_urls": who.AppURLs, "callback_path": "/api/auth/callback",
				"key":      map[string]any{"kid": d.KID, "retires_at": nil},
				"contract": nil,
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	// The key is made before the server answers anything: its issuer is the
	// listener's address.
	key, err := cauthykey.Compose("http://"+srv.Listener.Addr().String(), testKeyClient, testKeySecret)
	if err != nil {
		t.Fatal(err)
	}
	k, err := cauthykey.Parse(key)
	if err != nil {
		t.Fatal(err)
	}
	d = k.Derive()
	srv.Start()
	t.Cleanup(srv.Close)
	return key
}

// acmeCRM is the application the init tests connect: Acme CRM, workspace acme.
var acmeCRM = whoCauthySays{System: "acme-crm", Tenant: "acme", AppURLs: []string{"http://localhost:3000", "https://crm.example.com"}}
