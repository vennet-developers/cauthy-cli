package pact

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatus(t *testing.T) {
	const token = "cty_pat_s3cret"
	var asked *http.Request
	answer := `{"data":{"system":"acme-crm","in_step":true,"reconciled_at":"2026-11-03T06:00:00Z","outside":[],"missing":[]}}`
	code := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = r
		w.WriteHeader(code)
		fmt.Fprint(w, answer)
	}))
	defer srv.Close()
	c := contract()
	c.Issuer = srv.URL

	r := &Report{}
	text := CheckStatus(r, srv.Client(), c, token, false)
	if asked.URL.Path != "/v1/pact/status" || asked.URL.Query().Get("system") != "acme-crm" ||
		asked.Header.Get("Authorization") != "Bearer "+token || asked.Header.Get("X-Tenant-Key") != "acme" {
		t.Fatalf("asked %s %v", asked.URL, asked.Header)
	}
	if line(r, "cauthy.status").Status != Pass || text != "In step with its contract (last daily check 2026-11-03T06:00:00Z).\n" {
		t.Fatalf("%+v %q", r.Lines, text)
	}

	answer = `{"data":{"system":"acme-crm","in_step":false,"reconciled_at":null,
	  "outside":[{"kind":"client","client_id":"client_by_hand","value":"client_by_hand","detail":"Acme CRM"}],
	  "missing":[{"kind":"redirect_uri","client_id":"pact.acme-crm-signs-in-with-cauthy","value":"https://crm.example.com/api/auth/callback"}]}}`
	r = &Report{}
	text = CheckStatus(r, srv.Client(), c, token, false)
	if line(r, "cauthy.status").Status != Fail ||
		!strings.Contains(text, "    client client_by_hand (Acme CRM)") ||
		!strings.Contains(text, "    redirect_uri https://crm.example.com/api/auth/callback on pact.acme-crm-signs-in-with-cauthy") {
		t.Fatalf("%+v\n%s", r.Lines, text)
	}

	// Cauthy echoing the token in an error never puts it on screen.
	code, answer = http.StatusUnauthorized, `{"errors":[{"message":"token cty_pat_s3cret is not valid"}]}`
	r = &Report{}
	CheckStatus(r, srv.Client(), c, token, false)
	if l := line(r, "cauthy.status"); l.Status != Fail || strings.Contains(l.Detail, token) || !strings.Contains(l.Detail, "401") {
		t.Fatalf("%+v", l)
	}

	r = &Report{}
	CheckStatus(r, srv.Client(), c, "", false)
	if line(r, "cauthy.status").Status != Skip || r.ExitCode() != 3 {
		t.Fatalf("%+v", r.Lines)
	}
	r = &Report{}
	CheckStatus(r, srv.Client(), c, "", true)
	if line(r, "cauthy.status").Status != NA || r.ExitCode() != 0 {
		t.Fatalf("%+v", r.Lines)
	}
}
