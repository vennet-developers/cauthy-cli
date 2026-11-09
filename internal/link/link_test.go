package link

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func request() Request {
	return Request{System: "acme-crm", URLs: []string{"https://crm.example.com/", "http://localhost:3000"}, Service: true, Language: "Node", Framework: "Next.js"}
}

func TestCheckFillsDefaults(t *testing.T) {
	r := request()
	if err := r.Check(); err != nil {
		t.Fatal(err)
	}
	if r.CallbackPath != "/api/auth/callback" || r.Tenant != "acme-crm" || r.URLs[0] != "https://crm.example.com" || r.Production() != "https://crm.example.com" {
		t.Fatalf("%+v", r)
	}
	for name, change := range map[string]func(*Request){
		"bad system":     func(r *Request) { r.System = "Acme CRM" },
		"no url":         func(r *Request) { r.URLs = nil },
		"only localhost": func(r *Request) { r.URLs = []string{"http://localhost:3000"} },
		"plain http":     func(r *Request) { r.URLs = []string{"http://crm.example.com"} },
		"bad callback":   func(r *Request) { r.CallbackPath = "callback" },
		"bad tenant":     func(r *Request) { r.Tenant = "Acme" },
	} {
		r := request()
		change(&r)
		if err := r.Check(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSubjectIsAUUIDv4(t *testing.T) {
	s, err := NewSubject(rand.Reader)
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(s) {
		t.Fatalf("%q %v", s, err)
	}
	if _, err := NewSubject(bytes.NewReader(nil)); err == nil {
		t.Fatal("an empty random source was accepted")
	}
}

func TestLinkYAML(t *testing.T) {
	r := request()
	if err := r.Check(); err != nil {
		t.Fatal(err)
	}
	y := YAML(r, "d14c807d-a226-4a8c-91ad-147fa825ba0c")
	for _, want := range []string{
		"consumer:\n  system: acme-crm\n",
		"  tenant: acme-crm\n",
		"  subject: d14c807d-a226-4a8c-91ad-147fa825ba0c",
		"service: {}\n",
		"  require_system_proof: auto\n",
		"  require_verified_email: true\n",
		"  persistent_sessions: false\n",
		"    - https://crm.example.com/api/auth/callback\n    - http://localhost:3000/api/auth/callback\n",
	} {
		if !strings.Contains(y, want) {
			t.Errorf("missing %q in\n%s", want, y)
		}
	}
	if strings.Contains(y, "owner:") {
		t.Fatal("a link carries no owner")
	}
	r.Service = false
	y = YAML(r, "s")
	if strings.Contains(y, "service: {}") || !strings.Contains(y, "require_system_proof: false") {
		t.Fatalf("without service:\n%s", y)
	}
}

func TestManifestAndRequest(t *testing.T) {
	r := request()
	if err := r.Check(); err != nil {
		t.Fatal(err)
	}
	m := Manifest(r)
	for _, want := range []string{"system: acme-crm\n", "url: https://crm.example.com\n", "jwks_url: https://crm.example.com/.well-known/pact-jwks.json\n", "hosts: []\n"} {
		if !strings.Contains(m, want) {
			t.Errorf("manifest missing %q", want)
		}
	}
	q := ConnectionRequest(r)
	for _, want := range []string{
		"Subject: Pact connection request — acme-crm",
		"3. Addresses: production https://crm.example.com; development http://localhost:3000.",
		"4. Backend language and framework: Node, Next.js.",
		"acts as a system (creates organizations, invites people): yes;",
		"<your full name>, <your email>",
		"system proof: auto.",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("request missing %q in\n%s", want, q)
		}
	}
}

// The request goes where Vennet's joining guide says; without the guide at
// hand, to the address Vennet publishes there.
func TestRequestAddress(t *testing.T) {
	r := request()
	if err := r.Check(); err != nil {
		t.Fatal(err)
	}
	q := ConnectionRequest(r)
	if !strings.HasPrefix(q, "To: support@vennet.dev\n") {
		t.Fatalf("without an address:\n%s", q)
	}
	r.To = "pact@example.com"
	if q := ConnectionRequest(r); !strings.HasPrefix(q, "To: pact@example.com\n") {
		t.Fatalf("with an address:\n%s", q)
	}

	contracts := t.TempDir()
	if got := RequestAddress(contracts); got != "" {
		t.Fatalf("no guide: %q", got)
	}
	guide := filepath.Join(contracts, "docs", "joining-the-network.md")
	if err := os.MkdirAll(filepath.Dir(guide), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guide, []byte("Its owner sends this email to Vennet.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RequestAddress(contracts); got != "" {
		t.Fatalf("a guide without an address: %q", got)
	}
	if err := os.WriteFile(guide, []byte("Its owner sends this email to [Vennet](mailto:pact@example.com).\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RequestAddress(contracts); got != "pact@example.com" {
		t.Fatalf("got %q", got)
	}
}

// An application already on CAUTHY_KEY asks for a contract on its workspace;
// the request names its client and key id, never the key.
func TestConnectedRequest(t *testing.T) {
	r := request()
	r.Tenant = "acme"
	r.ClientID, r.KID = "client_0123456789abcdef0123456789abcdef", "dVpPrRtlQgOGn_9Q6DE2cgFRjUetrT2WPZCQyM48luc"
	if err := r.Check(); err != nil {
		t.Fatal(err)
	}
	q := ConnectionRequest(r)
	for _, want := range []string{
		"Workspace: acme; the application is already connected with CAUTHY_KEY (client client_0123456789abcdef0123456789abcdef, key id dVpPrRtlQgOGn_9Q6DE2cgFRjUetrT2WPZCQyM48luc).",
		"(an owner or admin of the workspace)",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("request missing %q in\n%s", want, q)
		}
	}
	if strings.Contains(q, "Tenant asked for") {
		t.Errorf("a connected application asks for no tenant:\n%s", q)
	}
	y := YAML(r, "s")
	if !strings.Contains(y, "with its CAUTHY_KEY") {
		t.Errorf("link:\n%s", y)
	}
}
