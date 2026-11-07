package graph

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/address"
)

// Provider is Cauthy's system id in the graph.
const Provider = "cauthy"

// Contract is what an application reads of its link with Cauthy.
type Contract struct {
	System               string
	Link                 string
	Issuer               string // Cauthy's address, no trailing slash
	TenantID             string
	Tenant               string // the tenant's slug
	ClientID             string // pact.<link>
	RedirectURIs         []string
	SystemProof          string // off | auto | required
	RequireVerifiedEmail bool
	PersistentSessions   bool
	ServiceClientID      string // pact.<link>.service; "" without a service block
	JWKSURL              string
	JWKSPath             string
	EventsURL            string
}

// RefusalError is a reason the application would refuse to start.
type RefusalError struct{ Reason string }

func (e *RefusalError) Error() string { return e.Reason }

func refuse(format string, a ...any) error { return &RefusalError{fmt.Sprintf(format, a...)} }

var (
	systemID = regexp.MustCompile(`^[a-z0-9]+(?:[-_][a-z0-9]+)*$`)
	uuidRe   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
)

const nilUUID = "00000000-0000-0000-0000-000000000000"

// ValidSystemID says whether s may name a system.
func ValidSystemID(s string) bool { return systemID.MatchString(s) }

// HTTPSAddress parses raw and accepts https, or http on localhost and
// 127.0.0.1. Its refusal shows the address without user info or query.
func HTTPSAddress(raw, what string) (*url.URL, error) {
	u, err := address.HTTPS(raw, what)
	if err != nil {
		return nil, &RefusalError{err.Error()}
	}
	return u, nil
}

// Read finds the system's sign-in link with Cauthy. link names one when the
// system has several; "" otherwise. Every error is a *RefusalError.
func (g *Graph) Read(system, link string) (*Contract, error) {
	if !ValidSystemID(system) {
		return nil, refuse("the system must be its id in the graph, e.g. \"acme-crm\": %q", system)
	}
	own := g.system(system)
	if own == nil {
		return nil, refuse("the graph has no system %q", system)
	}
	cauthy := g.system(Provider)
	if cauthy == nil {
		return nil, refuse("the graph has no system %q", Provider)
	}
	var found []Edge
	for _, e := range g.Edges {
		if e.Relation == "links" && e.From == system && e.To == Provider && e.SignIn != nil && (link == "" || e.Link == link) {
			found = append(found, e)
		}
	}
	switch {
	case len(found) == 0 && link != "":
		return nil, refuse("the graph has no link %q from %s to %s with sign_in", link, system, Provider)
	case len(found) == 0:
		return nil, refuse("no link of the graph signs %s's people in with %s: ask Vennet for one (cauthy init prints the request), then pull the graph again", system, Provider)
	case len(found) > 1:
		names := make([]string, len(found))
		for i, e := range found {
			names[i] = e.Link
		}
		return nil, refuse("%s has %d links to %s: name one with --link: %s", system, len(found), Provider, strings.Join(names, ", "))
	}
	e := found[0]
	c := &Contract{System: system, Link: e.Link}

	if err := c.readTenant(e.Identity, cauthy); err != nil {
		return nil, err
	}
	issuer, err := HTTPSAddress(cauthy.URL, "address of "+Provider)
	if err != nil {
		return nil, err
	}
	c.Issuer = strings.TrimRight(issuer.String(), "/")

	c.ClientID = e.SignIn.ClientID
	if c.ClientID == "" {
		c.ClientID = "pact." + e.Link
	}
	for _, u := range e.SignIn.RedirectURIs {
		if u != "" {
			c.RedirectURIs = append(c.RedirectURIs, u)
		}
	}
	if len(c.RedirectURIs) == 0 {
		return nil, refuse("the link %s names no redirect URI", e.Link)
	}
	// system_proof says the whole of it; a graph from before it has only the boolean.
	switch c.SystemProof = e.SignIn.SystemProof; c.SystemProof {
	case "off", "auto", "required":
	case "":
		c.SystemProof = "off"
		if e.SignIn.RequireSystemProof {
			c.SystemProof = "required"
		}
	default:
		return nil, refuse("the link %s has system_proof %q: it is off, auto or required. Update cauthy, or pull the graph again", e.Link, c.SystemProof)
	}
	if c.SystemProof != "off" && e.Service == nil {
		return nil, refuse("the link %s asks for the system's proof (%s) and gives %s no identity to make it with: it needs a service block", e.Link, c.SystemProof, system)
	}
	c.RequireVerifiedEmail = e.SignIn.RequireVerifiedEmail
	c.PersistentSessions = e.SignIn.PersistentSessions

	if e.Service != nil {
		c.ServiceClientID = e.Service.ClientID
		if c.ServiceClientID == "" {
			c.ServiceClientID = "pact." + e.Link + ".service"
		}
		jwks := e.Service.JWKSURL
		if jwks == "" {
			jwks = own.JWKSURL
		}
		if jwks == "" {
			return nil, refuse("the link %s gives %s its own identity, and %s names no jwks_url", e.Link, system, system)
		}
		u, err := HTTPSAddress(jwks, "jwks_url")
		if err != nil {
			return nil, err
		}
		c.JWKSURL, c.JWKSPath = u.String(), u.Path
	}
	if own.EventsURL != "" {
		u, err := HTTPSAddress(own.EventsURL, "events_url")
		if err != nil {
			return nil, err
		}
		c.EventsURL = u.String()
	}
	return c, nil
}

// readTenant resolves the link's tenant to an id and a slug, branch for
// branch as @cauthyhq/node reads it (src/contract.ts). The link names its
// tenant by id (the graph lists its slug) or by slug (the graph pins its id
// as tenant_id). A slug with no tenant_id is a tenant Cauthy has not created
// yet, or a graph pulled before it was.
func (c *Contract) readTenant(id *Identity, cauthy *Node) error {
	if id == nil {
		id = &Identity{}
	}
	named, pinned := id.Tenant, id.TenantID
	if named == nilUUID || pinned == nilUUID {
		return refuse("the link %s names the nil UUID as its tenant: name the tenant by its slug", c.Link)
	}
	if uuidRe.MatchString(named) {
		if pinned != "" && !strings.EqualFold(pinned, named) {
			return refuse("the link %s names the tenant %s and its tenant_id %s does not match: pull the graph again", c.Link, named, pinned)
		}
		for _, t := range cauthy.Tenants {
			if t.ID == named && t.Slug != "" {
				c.TenantID, c.Tenant = named, t.Slug
				return nil
			}
		}
		return refuse("the graph does not name the slug of the tenant %s that the link %s uses", named, c.Link)
	}
	if !ValidSystemID(named) {
		return refuse("the link %s names no tenant: identity.tenant is a UUID or a slug", c.Link)
	}
	if !uuidRe.MatchString(pinned) {
		if id.CreateTenant {
			return refuse("the link %s asks Cauthy for a new tenant %q, and the graph has no tenant_id for it yet: once Cauthy has created it and core-contracts lists it, pull the graph again", c.Link, named)
		}
		return refuse("the link %s names the tenant %q by its slug, and the graph carries no tenant_id for it: pull the graph again", c.Link, named)
	}
	c.TenantID, c.Tenant = pinned, named
	return nil
}
