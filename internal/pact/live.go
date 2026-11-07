package pact

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/graph"
)

// ProtocolHeader is the header a protocol-v1 backend sends on every answer.
const ProtocolHeader = "Cauthy-Protocol"

type jwk struct {
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	// Private members: never published.
	D  string `json:"d"`
	P  string `json:"p"`
	Q  string `json:"q"`
	DP string `json:"dp"`
	DQ string `json:"dq"`
	QI string `json:"qi"`
}

// Live probes the deployed application at app (its origin); base is the
// protocol's prefix (/api/auth) and level "basic" or "strict".
func Live(r *Report, client *http.Client, c *graph.Contract, app, base, level string) {
	origin, err := graph.HTTPSAddress(app, "--live address")
	if err != nil {
		r.Add("live.address", Fail, "%v", err)
		return
	}
	prefix := origin.Scheme + "://" + origin.Host + base
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	liveJWKS(r, &noRedirect, c)
	liveSession(r, &noRedirect, prefix, level)
	liveSignIn(r, &noRedirect, c, prefix)
}

func get(client *http.Client, target string) (*http.Response, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	return res, body, err
}

func liveJWKS(r *Report, client *http.Client, c *graph.Contract) {
	const id = "live.jwks"
	if c.JWKSURL == "" {
		r.Add(id, NA, "the link has no service block: the application publishes no key")
		return
	}
	res, body, err := get(client, c.JWKSURL)
	if err != nil {
		r.Add(id, Fail, "%s: %v", c.JWKSURL, err)
		return
	}
	if res.StatusCode != http.StatusOK {
		r.Add(id, Fail, "%s answered %d: mount the SDK's JWKS route there", c.JWKSURL, res.StatusCode)
		return
	}
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.Unmarshal(body, &set); err != nil {
		r.Add(id, Fail, "%s is not a JWK set: %v", c.JWKSURL, err)
		return
	}
	for _, k := range set.Keys {
		if k.D != "" || k.P != "" || k.Q != "" || k.DP != "" || k.DQ != "" || k.QI != "" {
			r.Add(id, Fail, "%s publishes PRIVATE key material (kid %s): take it down, then rotate the key and PACT_ROOT_KEY", c.JWKSURL, k.Kid)
			return
		}
	}
	if len(set.Keys) == 0 {
		r.Add(id, Fail, "%s publishes no key", c.JWKSURL)
		return
	}
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Alg != "RS256" || k.Kid == "" {
			r.Add(id, Fail, "%s: every key is RSA with alg RS256 and a kid; found kty %q alg %q kid %q", c.JWKSURL, k.Kty, k.Alg, k.Kid)
			return
		}
		n, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil || new(big.Int).SetBytes(n).BitLen() < 2048 {
			r.Add(id, Fail, "%s: key %s is not a 2048-bit RSA key", c.JWKSURL, k.Kid)
			return
		}
	}
	switch len(set.Keys) {
	case 1:
		r.Add(id, Pass, "one RS256 key published at %s", c.JWKSURL)
	case 2:
		r.Add(id, Pass, "two RS256 keys published at %s (a rotation is in its 24-hour window)", c.JWKSURL)
	default:
		r.Add(id, Fail, "%d keys published at %s: a rotation keeps two at most; remove the old ones", len(set.Keys), c.JWKSURL)
	}
}

func liveSession(r *Report, client *http.Client, prefix, level string) {
	const id = "live.session"
	res, body, err := get(client, prefix+"/session")
	if err != nil {
		r.Add(id, Fail, "%s/session: %v", prefix, err)
		return
	}
	var answer map[string]any
	if res.StatusCode != http.StatusOK || json.Unmarshal(body, &answer) != nil {
		r.Add(id, Fail, "%s/session answered %d, not the protocol's JSON: is the SDK mounted under %s?", prefix, res.StatusCode, prefix)
		return
	}
	switch v := res.Header.Get(ProtocolHeader); {
	case v == "1":
		r.Add(id, Pass, "%s/session answers with %s: 1", prefix, ProtocolHeader)
	case v != "":
		r.Add(id, Fail, "%s/session speaks protocol %s; this cauthy checks 1", prefix, v)
	case level == "strict":
		r.Add(id, Fail, "%s/session answers without %s: the backend does not meet protocol 1 at level strict", prefix, ProtocolHeader)
	default:
		r.Add(id, Advisory, "%s/session answers without %s (a cauthy-auth 0.x backend); @cauthyhq/node 1.0 sends it", prefix, ProtocolHeader)
	}
}

func liveSignIn(r *Report, client *http.Client, c *graph.Contract, prefix string) {
	const id = "live.sign_in"
	res, _, err := get(client, prefix+"/login")
	if err != nil {
		r.Add(id, Fail, "%s/login: %v", prefix, err)
		return
	}
	if res.StatusCode < 300 || res.StatusCode > 399 {
		r.Add(id, Fail, "%s/login answered %d, not a redirect to Cauthy", prefix, res.StatusCode)
		return
	}
	to, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		r.Add(id, Fail, "%s/login redirects to an unreadable address", prefix)
		return
	}
	issuer, _ := url.Parse(c.Issuer)
	q := to.Query()
	var wrong []string
	if to.Scheme != issuer.Scheme || to.Host != issuer.Host || to.Path != strings.TrimRight(issuer.Path, "/")+"/v1/oauth/authorize" {
		wrong = append(wrong, fmt.Sprintf("goes to %s://%s%s, not %s/v1/oauth/authorize", to.Scheme, to.Host, to.Path, c.Issuer))
	}
	if got := q.Get("client_id"); got != c.ClientID {
		wrong = append(wrong, fmt.Sprintf("client_id is %q, the contract's is %q", got, c.ClientID))
	}
	if got := q.Get("redirect_uri"); !slices.Contains(c.RedirectURIs, got) {
		wrong = append(wrong, fmt.Sprintf("redirect_uri %q is not one the link names", got))
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		wrong = append(wrong, "no PKCE S256 challenge")
	}
	if len(wrong) > 0 {
		r.Add(id, Fail, "%s/login: %s", prefix, strings.Join(wrong, "; "))
		return
	}
	r.Add(id, Pass, "%s/login sends people to Cauthy with %s and PKCE", prefix, c.ClientID)
}
