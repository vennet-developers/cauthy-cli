// Package link drafts what joining the Pact network takes: the link and the
// manifest Vennet merges into core-contracts, and the connection request an
// application's owner emails to Vennet. No personal data is ever written: the
// owner fills their name and address into the email themselves.
package link

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/graph"
)

// GuideAddress is where a connection request goes when no joining guide is at
// hand: the address Vennet publishes in core-contracts'
// docs/joining-the-network.md. A guide that names another wins.
const GuideAddress = "support@vennet.dev"

// Guide is the joining guide's path in core-contracts.
const Guide = "docs/joining-the-network.md"

var mailto = regexp.MustCompile(`mailto:([^\s()<>\[\]"']+@[^\s()<>\[\]"']+)`)

// RequestAddress is the address the joining guide of the core-contracts
// checkout at dir publishes (its first mailto: link), or "" when there is no
// guide or it names none.
func RequestAddress(dir string) string {
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(Guide)))
	if err != nil {
		return ""
	}
	if m := mailto.FindSubmatch(raw); m != nil {
		return string(m[1])
	}
	return ""
}

// Request is what init knows of the application.
type Request struct {
	System             string
	URLs               []string // every address people sign in at; at least one https
	CallbackPath       string   // default /api/auth/callback
	Tenant             string   // the tenant's slug; default the system
	Service            bool     // the application acts in Cauthy as itself (and proves its sign-ins)
	PersistentSessions bool
	Language           string // Node or Go
	Framework          string
	To                 string // where the connection request goes; "" for GuideAddress
}

var tenantSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,63}$`)

// Check validates the request and fills its defaults.
func (r *Request) Check() error {
	if !graph.ValidSystemID(r.System) {
		return fmt.Errorf("the system id is lowercase letters, digits, - or _: %q (pass --system)", r.System)
	}
	if r.CallbackPath == "" {
		r.CallbackPath = "/api/auth/callback"
	}
	if !strings.HasPrefix(r.CallbackPath, "/") {
		return errors.New("--callback-path starts with /")
	}
	if r.Tenant == "" {
		r.Tenant = strings.ReplaceAll(r.System, "_", "-")
	}
	if !tenantSlug.MatchString(r.Tenant) {
		return fmt.Errorf("the tenant slug is lowercase letters, digits and -: %q (pass --tenant)", r.Tenant)
	}
	if len(r.URLs) == 0 {
		return errors.New("give --app-url, once for each address people sign in at (the production one is https)")
	}
	https := false
	for i, raw := range r.URLs {
		u, err := graph.HTTPSAddress(raw, "--app-url")
		if err != nil {
			return err
		}
		r.URLs[i] = u.Scheme + "://" + u.Host
		https = https || u.Scheme == "https"
	}
	if !https {
		return errors.New("give the production address too: --app-url https://… (the graph needs an https url and jwks_url)")
	}
	return nil
}

// Production is the first https address.
func (r *Request) Production() string {
	for _, u := range r.URLs {
		if strings.HasPrefix(u, "https://") {
			return u
		}
	}
	return ""
}

// File is the link's path in core-contracts.
func File(system string) string { return "links/" + system + "-signs-in-with-cauthy.yaml" }

// ManifestFile is the manifest's path in core-contracts.
func ManifestFile(system string) string { return "systems/" + system + ".yaml" }

// NewSubject is a random UUID (v4): the id Cauthy gives the link's client.
func NewSubject(random io.Reader) (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// YAML is the link, in core-contracts' schema with tenants by slug,
// require_system_proof: auto and require_verified_email.
func YAML(r Request, subject string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `# %[1]s signs its people in with Cauthy through its own backend (@cauthyhq/node
# or cauthy-sdk-go in contract mode). The link IS the OAuth client: Cauthy
# applies it and makes a public client with id identity.subject and client_id
# pact.%[1]s-signs-in-with-cauthy; %[1]s reads the same from the graph.
# Drafted by cauthy init; Vennet reviews and merges it. Merging is the approval.

consumer:
  system: %[1]s
  description: People sign in to %[1]s through Cauthy.

identity:
  issuer: cauthy
  # A tenant Cauthy lists, or a new one by its slug: Cauthy creates it when it
  # applies the link. No owner here: the owner asks by a Pact connection request.
  tenant: %[2]s
  subject: %[3]s   # the client's internal id; Cauthy creates it with this id

provider:
  system: cauthy
  capability: cauthy.identity.v1
  scopes: []
`, r.System, r.Tenant, subject)
	if r.Service {
		fmt.Fprintf(&b, `
# %[1]s acts in Cauthy as itself with a key it makes and publishes at the
# jwks_url of systems/%[1]s.yaml: the client pact.%[1]s-signs-in-with-cauthy.service.
service: {}
`, r.System)
	}
	b.WriteString("\nsign_in:\n")
	if r.Service {
		b.WriteString("  # Required from the first sign-in that carries the application's proof.\n  require_system_proof: auto\n")
	} else {
		b.WriteString("  # No service block, so no proof to require.\n  require_system_proof: false\n")
	}
	fmt.Fprintf(&b, `  # No token until the person confirms their address.
  require_verified_email: true
  # People may ask to stay signed in (30 days without use, 90 in all).
  persistent_sessions: %t
  redirect_uris:
`, r.PersistentSessions)
	for _, u := range r.URLs {
		fmt.Fprintf(&b, "    - %s%s\n", u, r.CallbackPath)
	}
	return b.String()
}

// Manifest is the system's manifest, the least the graph accepts.
func Manifest(r Request) string {
	prod := r.Production()
	var b strings.Builder
	fmt.Fprintf(&b, `# %[1]s in the network. Drafted by cauthy init: Vennet completes it (its
# description, roles, what it owns and consumes) before merging.

system: %[1]s
description: %[1]s signs its people in with Cauthy.
url: %[2]s
`, r.System, prod)
	if r.Service {
		fmt.Fprintf(&b, "# Where %s publishes the public half of its key (the SDK serves it).\njwks_url: %s/.well-known/pact-jwks.json\n", r.System, prod)
	}
	b.WriteString(`
hosts: []

uses:
  - package: cauthy.identity.v1
    for: Signing people in, and verifying their tokens with Cauthy's public keys.
`)
	return b.String()
}

// ConnectionRequest is the email the owner sends Vennet (core-contracts,
// docs/joining-the-network.md, section 2.1), filled with what init knows.
func ConnectionRequest(r Request) string {
	var prod, dev []string
	for _, u := range r.URLs {
		if strings.HasPrefix(u, "https://") {
			prod = append(prod, u)
		} else {
			dev = append(dev, u)
		}
	}
	devText := "none"
	if len(dev) > 0 {
		devText = strings.Join(dev, ", ")
	}
	proof := "auto"
	if !r.Service {
		proof = "off (no service identity)"
	}
	to := r.To
	if to == "" {
		to = GuideAddress
	}
	return fmt.Sprintf(`To: %s
Subject: Pact connection request — %s

1. System: name <name>, slug %s, one-line description <description>,
   repository <repository URL>.
2. Owner: <your full name>, <your email>, <your role>
   (becomes the workspace owner in Cauthy).
3. Addresses: production %s; development %s.
4. Backend language and framework: %s, %s.
5. What it needs from Cauthy:
   - sign-in (always);
   - acts as a system (creates organizations, invites people): %s;
   - consumes events: none.
6. Policies:
   - verified email required: yes;
   - persistent sessions: %s;
   - self-service sign-ups: <yes/no>;
   - allowed email domains: <none, or the domains>;
   - system proof: %s.
Tenant asked for: %s (new, unless Cauthy already lists it).
Callback path: %s.
`, to, r.System, r.System, strings.Join(prod, ", "), devText, r.Language, r.Framework,
		yesNo(r.Service), yesNo(r.PersistentSessions), proof, r.Tenant, r.CallbackPath)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
