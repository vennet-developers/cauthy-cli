package graph

import (
	"errors"
	"strings"
	"testing"
)

// fixture is a graph with Acme CRM linked to Cauthy, the way core-contracts
// writes it. Tests change a parsed copy, never the string.
const fixture = `{
  "version": 1,
  "nodes": [
    {"id": "cauthy", "type": "system", "url": "https://id.example.com/",
     "tenants": [{"id": "74851970-8771-438e-a13a-9fb50161b721", "slug": "acme"}]},
    {"id": "acme-crm", "type": "system", "url": "https://crm.example.com",
     "jwks_url": "https://crm.example.com/.well-known/pact-jwks.json", "tenants": []},
    {"id": "cauthy.identity.v1", "type": "capability"}
  ],
  "edges": [
    {"from": "acme-crm", "relation": "links", "to": "cauthy", "link": "acme-crm-signs-in-with-cauthy",
     "identity": {"issuer": "cauthy", "tenant": "74851970-8771-438e-a13a-9fb50161b721", "subject": "d14c807d-a226-4a8c-91ad-147fa825ba0c"},
     "sign_in": {"client_id": "pact.acme-crm-signs-in-with-cauthy",
                 "redirect_uris": ["https://crm.example.com/api/auth/callback", "http://localhost:3000/api/auth/callback"],
                 "require_system_proof": true, "persistent_sessions": false},
     "service": {"client_id": "pact.acme-crm-signs-in-with-cauthy.service", "system": "acme-crm",
                 "jwks_url": "https://crm.example.com/.well-known/pact-jwks.json"}},
    {"from": "cauthy", "relation": "hosts", "to": "cauthy.identity.v1"}
  ]
}`

func parsed(t *testing.T) *Graph {
	t.Helper()
	g, err := Parse([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestParseAndCounts(t *testing.T) {
	g := parsed(t)
	if s, l := g.Counts(); s != 2 || l != 1 {
		t.Fatalf("counts %d systems %d links", s, l)
	}
	for _, raw := range []string{`nope`, `{"hello": 1}`, `{"nodes": {}, "edges": []}`, `{"nodes": []}`} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Errorf("%s: accepted", raw)
		}
	}
}

func TestReadContract(t *testing.T) {
	c, err := parsed(t).Read("acme-crm", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.System != "acme-crm" || c.Link != "acme-crm-signs-in-with-cauthy" || c.Issuer != "https://id.example.com" ||
		c.TenantID != "74851970-8771-438e-a13a-9fb50161b721" || c.Tenant != "acme" ||
		c.ClientID != "pact.acme-crm-signs-in-with-cauthy" || c.SystemProof != "required" ||
		c.ServiceClientID != "pact.acme-crm-signs-in-with-cauthy.service" ||
		c.JWKSURL != "https://crm.example.com/.well-known/pact-jwks.json" || c.JWKSPath != "/.well-known/pact-jwks.json" ||
		len(c.RedirectURIs) != 2 || c.PersistentSessions || c.RequireVerifiedEmail {
		t.Fatalf("got %+v", c)
	}
}

func TestReadNewLinkFields(t *testing.T) {
	g := parsed(t)
	e := &g.Edges[0]
	// A tenant named by its slug, listed: the graph adds tenant_id.
	e.Identity.Tenant, e.Identity.TenantID = "acme", "74851970-8771-438e-a13a-9fb50161b721"
	e.SignIn.RequireSystemProof, e.SignIn.SystemProof, e.SignIn.RequireVerifiedEmail = false, "auto", true
	c, err := g.Read("acme-crm", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Tenant != "acme" || c.SystemProof != "auto" || !c.RequireVerifiedEmail {
		t.Fatalf("got %+v", c)
	}
	// Defaults when the graph names no client ids and the link no jwks_url.
	e.SignIn.ClientID, e.Service.ClientID, e.Service.JWKSURL = "", "", ""
	c, err = g.Read("acme-crm", "")
	if err != nil || c.ClientID != "pact.acme-crm-signs-in-with-cauthy" || c.ServiceClientID != "pact.acme-crm-signs-in-with-cauthy.service" ||
		c.JWKSURL != "https://crm.example.com/.well-known/pact-jwks.json" {
		t.Fatalf("defaults: %v %+v", err, c)
	}
}

func TestReadRefusals(t *testing.T) {
	cases := map[string]struct {
		change func(g *Graph)
		system string
		link   string
		want   string
	}{
		"bad system id":  {func(*Graph) {}, "Acme CRM", "", "must be its id"},
		"unknown system": {func(*Graph) {}, "other", "", `no system "other"`},
		"no cauthy":      {func(g *Graph) { g.Nodes[0].ID = "idp" }, "acme-crm", "", `no system "cauthy"`},
		"no link":        {func(g *Graph) { g.Edges[0].SignIn = nil }, "acme-crm", "", "ask Vennet"},
		"unknown link":   {func(*Graph) {}, "acme-crm", "other-link", `no link "other-link"`},
		"two links":      {func(g *Graph) { g.Edges = append(g.Edges, g.Edges[0]) }, "acme-crm", "", "has 2 links"},
		// The tenant's branches, as @cauthyhq/node reads them (src/contract.ts).
		"tenant to create":       {func(g *Graph) { g.Edges[0].Identity.Tenant, g.Edges[0].Identity.CreateTenant = "acme-new", true }, "acme-crm", "", `asks Cauthy for a new tenant "acme-new"`},
		"slug without tenant_id": {func(g *Graph) { g.Edges[0].Identity.Tenant = "acme" }, "acme-crm", "", `names the tenant "acme" by its slug, and the graph carries no tenant_id`},
		"nil tenant":             {func(g *Graph) { g.Edges[0].Identity.Tenant = nilUUID }, "acme-crm", "", "nil UUID"},
		"nil tenant_id":          {func(g *Graph) { g.Edges[0].Identity.Tenant, g.Edges[0].Identity.TenantID = "acme", nilUUID }, "acme-crm", "", "nil UUID"},
		"tenant_id differs":      {func(g *Graph) { g.Edges[0].Identity.TenantID = "11111111-1111-4111-8111-111111111111" }, "acme-crm", "", "does not match"},
		"invalid slug":           {func(g *Graph) { g.Edges[0].Identity.Tenant = "Acme Corp" }, "acme-crm", "", "identity.tenant is a UUID or a slug"},
		"no identity":            {func(g *Graph) { g.Edges[0].Identity = nil }, "acme-crm", "", "identity.tenant is a UUID or a slug"},
		"unlisted tenant":        {func(g *Graph) { g.Nodes[0].Tenants = nil }, "acme-crm", "", "does not name the slug"},
		// The system proof.
		"unknown system_proof":  {func(g *Graph) { g.Edges[0].SignIn.SystemProof = "sometimes" }, "acme-crm", "", `system_proof "sometimes"`},
		"proof without service": {func(g *Graph) { g.Edges[0].Service = nil }, "acme-crm", "", "needs a service block"},
		"auto without service":  {func(g *Graph) { g.Edges[0].Service, g.Edges[0].SignIn.SystemProof = nil, "auto" }, "acme-crm", "", "(auto)"},
		"http issuer":           {func(g *Graph) { g.Nodes[0].URL = "http://id.example.com" }, "acme-crm", "", "must be https"},
		"no redirect":           {func(g *Graph) { g.Edges[0].SignIn.RedirectURIs = nil }, "acme-crm", "", "names no redirect URI"},
		"no jwks":               {func(g *Graph) { g.Edges[0].Service.JWKSURL, g.Nodes[1].JWKSURL = "", "" }, "acme-crm", "", "names no jwks_url"},
		"http jwks":             {func(g *Graph) { g.Edges[0].Service.JWKSURL = "http://crm.example.com/k" }, "acme-crm", "", "jwks_url must be https"},
		"http events":           {func(g *Graph) { g.Nodes[1].EventsURL = "http://crm.example.com/e" }, "acme-crm", "", "events_url must be https"},
	}
	for name, tc := range cases {
		g := parsed(t)
		tc.change(g)
		_, err := g.Read(tc.system, tc.link)
		var refusal *RefusalError
		if !errors.As(err, &refusal) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want a refusal with %q", name, err, tc.want)
		}
	}
}

func TestLocalhostIsAllowedOverHTTP(t *testing.T) {
	g := parsed(t)
	g.Nodes[0].URL = "http://localhost:8080"
	if c, err := g.Read("acme-crm", ""); err != nil || c.Issuer != "http://localhost:8080" {
		t.Fatalf("%v %+v", err, c)
	}
}

func TestRefusalsNeverShowWhatAnAddressCarries(t *testing.T) {
	for _, change := range []func(g *Graph){
		func(g *Graph) { g.Nodes[0].URL = "http://u:pw@id.example.com/?token=SECRET" },
		func(g *Graph) { g.Edges[0].Service.JWKSURL = "http://u:pw@crm.example.com/k?token=SECRET" },
		func(g *Graph) { g.Nodes[1].EventsURL = "ftp://crm.example.com/e?token=SECRET" },
	} {
		g := parsed(t)
		change(g)
		_, err := g.Read("acme-crm", "")
		if err == nil || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "pw") {
			t.Errorf("got %v", err)
		}
	}
}

func TestReadTenantBranchesThatPass(t *testing.T) {
	const id = "74851970-8771-438e-a13a-9fb50161b721"
	for name, tc := range map[string]struct {
		change func(g *Graph)
		slug   string
	}{
		"uuid, listed":                 {func(*Graph) {}, "acme"},
		"uuid, tenant_id in uppercase": {func(g *Graph) { g.Edges[0].Identity.TenantID = strings.ToUpper(id) }, "acme"},
		"slug and tenant_id":           {func(g *Graph) { g.Edges[0].Identity.Tenant, g.Edges[0].Identity.TenantID = "acme", id }, "acme"},
		"slug and tenant_id, unlisted": {func(g *Graph) {
			g.Nodes[0].Tenants = nil
			g.Edges[0].Identity.Tenant, g.Edges[0].Identity.TenantID = "acme", id
		}, "acme"},
		"created, tenant_id pinned": {func(g *Graph) {
			g.Edges[0].Identity.Tenant, g.Edges[0].Identity.TenantID, g.Edges[0].Identity.CreateTenant = "acme-new", id, true
		}, "acme-new"},
		"off without service": {func(g *Graph) { g.Edges[0].Service, g.Edges[0].SignIn.RequireSystemProof = nil, false }, "acme"},
	} {
		g := parsed(t)
		tc.change(g)
		c, err := g.Read("acme-crm", "")
		if err != nil || c.TenantID != id || c.Tenant != tc.slug {
			t.Errorf("%s: %v %+v", name, err, c)
		}
	}
}
