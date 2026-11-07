package pact

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-cli/internal/graph"
)

// app is a deployed application as the probes see it; its fields break it.
type app struct {
	keys      []map[string]string
	protocol  string
	clientID  string
	redirect  string
	noSession bool
}

func publicJWK(t *testing.T, bits int) map[string]string {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{"kty": "RSA", "alg": "RS256", "use": "sig", "kid": fmt.Sprintf("k%d", bits),
		"n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.E)).Bytes())}
}

// serve starts the application and answers the contract that points at it.
func serve(t *testing.T, a *app) (*httptest.Server, *graph.Contract) {
	t.Helper()
	c := contract()
	c.Issuer = "http://localhost:8080"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/pact-jwks.json":
			json.NewEncoder(w).Encode(map[string]any{"keys": a.keys})
		case "/api/auth/session":
			if a.noSession {
				http.NotFound(w, r)
				return
			}
			if a.protocol != "" {
				w.Header().Set(ProtocolHeader, a.protocol)
			}
			fmt.Fprint(w, `{"session":null,"status":"none"}`)
		case "/api/auth/login":
			q := url.Values{"client_id": {a.clientID}, "redirect_uri": {a.redirect}, "response_type": {"code"},
				"state": {"s"}, "code_challenge": {"abc"}, "code_challenge_method": {"S256"}}
			http.Redirect(w, r, c.Issuer+"/v1/oauth/authorize?"+q.Encode(), http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	c.JWKSURL = srv.URL + "/.well-known/pact-jwks.json"
	c.RedirectURIs = []string{srv.URL + "/api/auth/callback"}
	if a.redirect == "" {
		a.redirect = c.RedirectURIs[0]
	}
	if a.clientID == "" {
		a.clientID = c.ClientID
	}
	return srv, c
}

func TestLiveWellDeployed(t *testing.T) {
	a := &app{keys: []map[string]string{publicJWK(t, 2048)}, protocol: "1"}
	srv, c := serve(t, a)
	defer srv.Close()
	r := &Report{}
	Live(r, srv.Client(), c, srv.URL, "/api/auth", "strict")
	if r.ExitCode() != 0 {
		t.Fatalf("%s", r.Text())
	}
	if l := line(r, "live.jwks"); !strings.Contains(l.Detail, "one RS256 key") {
		t.Fatalf("%+v", l)
	}
}

func TestLiveFindsWhatIsWrong(t *testing.T) {
	good := publicJWK(t, 2048)
	private := map[string]string{"d": "AQAB"}
	for k, v := range good {
		private[k] = v
	}
	cases := map[string]struct {
		a      app
		level  string
		id     string
		status string
		detail string
	}{
		"private key published": {app{keys: []map[string]string{private}, protocol: "1"}, "basic", "live.jwks", Fail, "PRIVATE key material"},
		"no key":                {app{keys: []map[string]string{}, protocol: "1"}, "basic", "live.jwks", Fail, "publishes no key"},
		"weak key":              {app{keys: []map[string]string{publicJWK(t, 1024)}, protocol: "1"}, "basic", "live.jwks", Fail, "not a 2048-bit"},
		"rotation window":       {app{keys: []map[string]string{good, good}, protocol: "1"}, "basic", "live.jwks", Pass, "24-hour window"},
		"three keys":            {app{keys: []map[string]string{good, good, good}, protocol: "1"}, "basic", "live.jwks", Fail, "two at most"},
		"0.x backend, basic":    {app{keys: []map[string]string{good}}, "basic", "live.session", Advisory, "cauthy-auth 0.x"},
		"0.x backend, strict":   {app{keys: []map[string]string{good}}, "strict", "live.session", Fail, "level strict"},
		"other protocol":        {app{keys: []map[string]string{good}, protocol: "2"}, "basic", "live.session", Fail, "speaks protocol 2"},
		"not mounted":           {app{keys: []map[string]string{good}, noSession: true}, "basic", "live.session", Fail, "is the SDK mounted"},
		"another client":        {app{keys: []map[string]string{good}, protocol: "1", clientID: "client_by_hand"}, "basic", "live.sign_in", Fail, `client_id is "client_by_hand"`},
		"another return":        {app{keys: []map[string]string{good}, protocol: "1", redirect: "https://evil.example.com/cb"}, "basic", "live.sign_in", Fail, "is not one the link names"},
	}
	for name, tc := range cases {
		a := tc.a
		srv, c := serve(t, &a)
		r := &Report{}
		Live(r, srv.Client(), c, srv.URL, "/api/auth", tc.level)
		srv.Close()
		if l := line(r, tc.id); l.Status != tc.status || !strings.Contains(l.Detail, tc.detail) {
			t.Errorf("%s: %+v, want %s with %q", name, l, tc.status, tc.detail)
		}
	}
}

func TestLiveRefusesAPlainHTTPAddress(t *testing.T) {
	r := &Report{}
	Live(r, http.DefaultClient, contract(), "http://crm.example.com", "/api/auth", "basic")
	if l := line(r, "live.address"); l.Status != Fail {
		t.Fatalf("%+v", r.Lines)
	}
}
