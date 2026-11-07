package pact

import (
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-cli/internal/graph"
)

// contract is Acme CRM's contract as graph.Read answers it.
func contract() *graph.Contract {
	return &graph.Contract{
		System: "acme-crm", Link: "acme-crm-signs-in-with-cauthy", Issuer: "https://id.example.com",
		TenantID: "74851970-8771-438e-a13a-9fb50161b721", Tenant: "acme", ClientID: "pact.acme-crm-signs-in-with-cauthy",
		RedirectURIs: []string{"https://crm.example.com/api/auth/callback"}, SystemProof: "auto", RequireVerifiedEmail: true,
		ServiceClientID: "pact.acme-crm-signs-in-with-cauthy.service",
		JWKSURL:         "https://crm.example.com/.well-known/pact-jwks.json", JWKSPath: "/.well-known/pact-jwks.json",
	}
}

func env(vars map[string]string) func(string) string { return func(k string) string { return vars[k] } }

// line is the report's line with that id.
func line(r *Report, id string) Line {
	for _, l := range r.Lines {
		if l.ID == id {
			return l
		}
	}
	return Line{}
}

func TestReportExitCodes(t *testing.T) {
	r := &Report{}
	r.Add("a", Pass, "fine")
	r.Add("b", Advisory, "worth fixing")
	if r.ExitCode() != 0 {
		t.Fatal("pass and advisory exit 0")
	}
	r.Add("c", Skip, "not asked")
	if r.ExitCode() != 3 || !strings.Contains(r.Text(), "INCOMPLETE: 1 skipped") {
		t.Fatalf("skip exits 3:\n%s", r.Text())
	}
	r.Add("d", Fail, "broken")
	if r.ExitCode() != 1 || !strings.Contains(r.Text(), "FAIL     d  broken") {
		t.Fatalf("fail exits 1:\n%s", r.Text())
	}
}

func TestDescribe(t *testing.T) {
	text := Describe(contract())
	for _, want := range []string{
		"acme-crm signs in with Cauthy by the link acme-crm-signs-in-with-cauthy",
		"  Cauthy          https://id.example.com (tenant acme, 74851970-8771-438e-a13a-9fb50161b721)",
		"  sign-in client  pact.acme-crm-signs-in-with-cauthy",
		"  system proof    auto: required once the first sign-in carries it",
		"  verified email  required",
		"  stay signed in  not offered",
		"  own identity    pact.acme-crm-signs-in-with-cauthy.service, keys at https://crm.example.com/.well-known/pact-jwks.json",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in\n%s", want, text)
		}
	}
}

func TestOfflineNeverPrintsAValue(t *testing.T) {
	secret := strings.Repeat("k", 43)
	r := &Report{}
	Offline(r, contract(), env(map[string]string{"PACT_ROOT_KEY": secret, "CAUTHY_SECRET": "s3cret-value", "CAUTHY_ISSUER": "https://x"}))
	if l := line(r, "env.variables_mode"); l.Status != Fail || l.Detail != "CAUTHY_ISSUER, CAUTHY_SECRET set: the SDK refuses to start by contract with them; remove them" {
		t.Fatalf("%+v", l)
	}
	if l := line(r, "env.root_key"); l.Status != Pass {
		t.Fatalf("%+v", l)
	}
	if text := r.Text(); strings.Contains(text, secret) || strings.Contains(text, "s3cret-value") {
		t.Fatalf("a value was printed:\n%s", text)
	}
	if r.ExitCode() != 1 {
		t.Fatalf("exit %d", r.ExitCode())
	}
}

func TestOfflineRootKeys(t *testing.T) {
	cases := []struct {
		vars       map[string]string
		id, status string
	}{
		{map[string]string{}, "env.root_key", Advisory},
		{map[string]string{"PACT_ROOT_KEY": "short"}, "env.root_key", Fail},
		{map[string]string{"PACT_ROOT_KEY": strings.Repeat("a", 43), "PACT_ROOT_KEY_PREVIOUS": "short"}, "env.root_key_previous", Fail},
		{map[string]string{"PACT_ROOT_KEY": strings.Repeat("a", 43), "PACT_ROOT_KEY_PREVIOUS": strings.Repeat("a", 43)}, "env.root_key_previous", Advisory},
		{map[string]string{"PACT_ROOT_KEY": strings.Repeat("a", 43), "PACT_ROOT_KEY_PREVIOUS": strings.Repeat("b", 43)}, "env.root_key_previous", Pass},
	}
	for i, tc := range cases {
		r := &Report{}
		Offline(r, contract(), env(tc.vars))
		if l := line(r, tc.id); l.Status != tc.status {
			t.Errorf("case %d: %+v, want %s", i, l, tc.status)
		}
	}
	weak := contract()
	weak.SystemProof, weak.RequireVerifiedEmail = "off", false
	r := &Report{}
	Offline(r, weak, env(nil))
	if line(r, "link.system_proof").Status != Advisory || line(r, "link.verified_email").Status != Advisory || r.ExitCode() != 0 {
		t.Fatalf("%+v", r.Lines)
	}
}

func TestTextFromTheGraphCarriesNoControlCharacter(t *testing.T) {
	const evil = "\x1b]0;owned\x07\x1b[2J\r‮"
	r := &Report{}
	r.Add("x"+evil, Fail, "detail %s end", evil)
	c := contract()
	c.Link, c.Tenant, c.RedirectURIs = "acme"+evil, "acme"+evil, []string{"https://crm.example.com/cb" + evil}
	s := &Status{Outside: []Item{{Kind: "client", Value: "v" + evil, Detail: "d" + evil}}}
	for _, text := range []string{r.Text(), Describe(c), DescribeStatus(s)} {
		for _, bad := range []string{"\x1b", "\x07", "\r", "‮"} {
			if strings.Contains(text, bad) {
				t.Fatalf("%q in %q", bad, text)
			}
		}
	}
	if !strings.Contains(r.Text(), "detail ]0;owned[2J end") {
		t.Fatalf("only the control characters go: %q", r.Text())
	}
}

func TestRootKeyLengthIsInCharacters(t *testing.T) {
	multibyte := strings.Repeat("é", 20) // 40 bytes, 20 characters
	for _, vars := range []map[string]string{
		{"PACT_ROOT_KEY": multibyte},
		{"PACT_ROOT_KEY": strings.Repeat("a", 43), "PACT_ROOT_KEY_PREVIOUS": multibyte},
	} {
		r := &Report{}
		Offline(r, contract(), env(vars))
		if r.ExitCode() != 1 {
			t.Fatalf("%s", r.Text())
		}
		if !strings.Contains(r.Text(), "too short") || strings.Contains(r.Text(), "is 20 characters") {
			t.Fatalf("say too short, not the key's length:\n%s", r.Text())
		}
	}
}

func TestCleanLinesKeepsTheLines(t *testing.T) {
	if got := CleanLines("PASS  a\x1b[2J\r\nFAIL  b‮\n"); got != "PASS  a[2J\nFAIL  b\n" {
		t.Fatalf("%q", got)
	}
}
