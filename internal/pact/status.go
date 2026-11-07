package pact

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/graph"
)

// Item is one difference between the contract and what Cauthy holds.
type Item struct {
	Kind     string `json:"kind"`
	ClientID string `json:"client_id"`
	Value    string `json:"value"`
	Detail   string `json:"detail"`
}

// Status is Cauthy's answer to GET /v1/pact/status. Moments are UTC.
type Status struct {
	System       string  `json:"system"`
	InStep       bool    `json:"in_step"`
	ReconciledAt *string `json:"reconciled_at"`
	Outside      []Item  `json:"outside"`
	Missing      []Item  `json:"missing"`
}

// FetchStatus asks Cauthy how the system stands. The token never appears in
// what it answers, even when Cauthy echoes it.
func FetchStatus(client *http.Client, c *graph.Contract, token string) (*Status, error) {
	req, err := http.NewRequest(http.MethodGet, c.Issuer+"/v1/pact/status?system="+url.QueryEscape(c.System), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Tenant-Key", c.Tenant)
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asking Cauthy: %v", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	var body struct {
		Data   *Status `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(raw, &body)
	if res.StatusCode != http.StatusOK || body.Data == nil {
		why := http.StatusText(res.StatusCode)
		if len(body.Errors) > 0 && body.Errors[0].Message != "" {
			why = body.Errors[0].Message
		}
		return nil, fmt.Errorf("Cauthy answered %d: %s", res.StatusCode, strings.ReplaceAll(why, token, "[redacted]"))
	}
	return body.Data, nil
}

// DescribeStatus is how the system stands, in lines.
func DescribeStatus(s *Status) string {
	if s.InStep {
		if s.ReconciledAt != nil {
			return fmt.Sprintf("In step with its contract (last daily check %s).\n", *s.ReconciledAt)
		}
		return "In step with its contract.\n"
	}
	lines := []string{"Not in step with its contract."}
	if len(s.Outside) > 0 {
		lines = append(lines, "  outside the contract (in Cauthy, the contract does not name it):")
		for _, i := range s.Outside {
			lines = append(lines, "    "+itemText(i))
		}
	}
	if len(s.Missing) > 0 {
		lines = append(lines, "  missing (the contract names it, Cauthy does not hold it as it says):")
		for _, i := range s.Missing {
			lines = append(lines, "    "+itemText(i))
		}
	}
	lines = append(lines, "Nothing is deleted for it: remove what does not belong, or change the contract.")
	return strings.Join(lines, "\n") + "\n"
}

func itemText(i Item) string {
	s := i.Kind + " " + i.Value
	if i.ClientID != "" && i.ClientID != i.Value {
		s += " on " + i.ClientID
	}
	if i.Detail != "" {
		s += " (" + i.Detail + ")"
	}
	return s
}

// CheckStatus adds the cauthy.status line and answers the status in lines:
// skipped without a token, not applicable with --offline.
func CheckStatus(r *Report, client *http.Client, c *graph.Contract, token string, offline bool) string {
	switch {
	case offline:
		r.Add("cauthy.status", NA, "--offline: Cauthy was not asked")
		return ""
	case token == "":
		r.Add("cauthy.status", Skip, "set CAUTHY_TOKEN (a personal access token with oauth_clients:manage) to ask Cauthy, or pass --offline")
		return ""
	}
	s, err := FetchStatus(client, c, token)
	if err != nil {
		r.Add("cauthy.status", Fail, "%v", err)
		return ""
	}
	if s.InStep {
		r.Add("cauthy.status", Pass, "in step with its contract")
	} else {
		r.Add("cauthy.status", Fail, "not in step with its contract: %d outside, %d missing", len(s.Outside), len(s.Missing))
	}
	return DescribeStatus(s)
}
