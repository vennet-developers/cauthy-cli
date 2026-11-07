// Package graph reads the Pact context graph (core-contracts' graph.json) and
// the contract an application finds in it, by the rules of the spec's
// contract/pact.md.
package graph

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Graph is the part of graph.json the CLI reads. Unknown fields are ignored.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Node struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	URL       string   `json:"url"`
	EventsURL string   `json:"events_url"`
	JWKSURL   string   `json:"jwks_url"`
	Tenants   []Tenant `json:"tenants"`
}

type Tenant struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
}

type Edge struct {
	From     string    `json:"from"`
	Relation string    `json:"relation"`
	To       string    `json:"to"`
	Link     string    `json:"link"`
	Identity *Identity `json:"identity"`
	SignIn   *SignIn   `json:"sign_in"`
	Service  *Service  `json:"service"`
}

// Identity is the link's identity. TenantID and CreateTenant are written by
// the graph for a tenant named by its slug (core-contracts, link-schema-tenants).
type Identity struct {
	Issuer       string `json:"issuer"`
	Tenant       string `json:"tenant"`
	Subject      string `json:"subject"`
	TenantID     string `json:"tenant_id"`
	CreateTenant bool   `json:"create_tenant"`
}

type SignIn struct {
	ClientID             string   `json:"client_id"`
	RedirectURIs         []string `json:"redirect_uris"`
	RequireSystemProof   bool     `json:"require_system_proof"`
	SystemProof          string   `json:"system_proof"` // off | auto | required; absent in older graphs
	RequireVerifiedEmail bool     `json:"require_verified_email"`
	PersistentSessions   bool     `json:"persistent_sessions"`
}

type Service struct {
	ClientID string `json:"client_id"`
	System   string `json:"system"`
	JWKSURL  string `json:"jwks_url"`
}

// Parse reads graph.json, refusing what is not a context graph.
func Parse(raw []byte) (*Graph, error) {
	var probe struct {
		Nodes json.RawMessage `json:"nodes"`
		Edges json.RawMessage `json:"edges"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("not JSON: %w", err)
	}
	if len(probe.Nodes) == 0 || len(probe.Edges) == 0 || probe.Nodes[0] != '[' || probe.Edges[0] != '[' {
		return nil, errors.New("it is not a context graph (no nodes and edges)")
	}
	var g Graph
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, fmt.Errorf("it is not a context graph: %w", err)
	}
	return &g, nil
}

// Counts says how many systems and links the graph holds.
func (g *Graph) Counts() (systems, links int) {
	for _, n := range g.Nodes {
		if n.Type == "system" {
			systems++
		}
	}
	for _, e := range g.Edges {
		if e.Relation == "links" {
			links++
		}
	}
	return systems, links
}

func (g *Graph) system(id string) *Node {
	for i := range g.Nodes {
		if g.Nodes[i].Type == "system" && g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}
