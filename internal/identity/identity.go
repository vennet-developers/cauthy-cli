// Package identity asks Cauthy who a CAUTHY_KEY is (key/cauthy-key.md §7 in
// cauthy-sdk-spec): a client_credentials token signed with the key's
// assertion, then GET /v1/connect/self. Nothing here prints, logs or keeps
// the key; errors name Cauthy's answer, the issuer and at most the client id.
package identity

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
)

// Identity is who the application is, as Cauthy answers it. The same shape
// as the Go SDK's (copied: the CLI does not depend on the SDK).
type Identity struct {
	Issuer   string `json:"issuer"`
	Audience string `json:"audience"`
	Tenant   struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	} `json:"tenant"`
	System         string   `json:"system"`
	ClientID       string   `json:"client_id"`
	SignInClientID string   `json:"sign_in_client_id"`
	AppURLs        []string `json:"app_urls"`
	CallbackPath   string   `json:"callback_path"`
	Key            *struct {
		Kid       string  `json:"kid"`
		RetiresAt *string `json:"retires_at"`
	} `json:"key"`
	Contract *struct {
		Link                 string `json:"link"`
		SystemProof          string `json:"system_proof"`
		RequireVerifiedEmail bool   `json:"require_verified_email"`
	} `json:"contract"`
}

// Refusal is Cauthy answering no: the step, its status and its error code.
type Refusal struct {
	Issuer string
	Step   string // "token" or "self"
	Status int
	Code   string
}

func (r *Refusal) Error() string {
	answer := fmt.Sprintf("%d", r.Status)
	if r.Code != "" {
		answer += " " + r.Code
	}
	return fmt.Sprintf("Cauthy at %s answered %s (%s)", r.Issuer, answer, r.Step)
}

// maxBody bounds what is read of any answer.
const maxBody = 1 << 20

// Ask asks Cauthy who key is.
func Ask(ctx context.Context, hc *http.Client, key *cauthykey.Key) (*Identity, error) {
	if hc == nil {
		hc = http.DefaultClient
	}
	token, err := systemToken(ctx, hc, key)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, key.Issuer+"/v1/connect/self", nil)
	if err != nil {
		return nil, fmt.Errorf("cannot ask Cauthy at %s", key.Issuer)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	res, err := hc.Do(req)
	if err != nil {
		return nil, unreachable(key.Issuer, err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if res.StatusCode != http.StatusOK {
		return nil, &Refusal{Issuer: key.Issuer, Step: "self", Status: res.StatusCode, Code: errorCode(body)}
	}
	var out struct {
		Data *Identity `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Data == nil {
		return nil, fmt.Errorf("Cauthy at %s answered something that is not an identity", key.Issuer)
	}
	return out.Data, nil
}

// systemToken is client_credentials with the key's assertion (§6).
func systemToken(ctx context.Context, hc *http.Client, key *cauthykey.Key) (string, error) {
	jti, err := uuid4()
	if err != nil {
		return "", err
	}
	endpoint := key.Issuer + "/v1/oauth/token"
	form := url.Values{
		"grant_type":            {"client_credentials"},
		"client_id":             {key.ClientID},
		"client_assertion_type": {cauthykey.AssertionType},
		"client_assertion":      {key.Derive().Assertion(key.ClientID, endpoint, jti, time.Now())},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("cannot ask Cauthy at %s", key.Issuer)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := hc.Do(req)
	if err != nil {
		return "", unreachable(key.Issuer, err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if res.StatusCode != http.StatusOK {
		return "", &Refusal{Issuer: key.Issuer, Step: "token", Status: res.StatusCode, Code: errorCode(body)}
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return "", fmt.Errorf("Cauthy at %s answered no access token", key.Issuer)
	}
	return tok.AccessToken, nil
}

// errorCode reads an error's code from an OAuth answer ({"error"}), Cauthy's
// API envelope ({"errors":[{"code"}]}) or a bare {"code"}.
func errorCode(body []byte) string {
	var e struct {
		Error  string `json:"error"`
		Code   string `json:"code"`
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &e) != nil {
		return ""
	}
	switch {
	case e.Error != "":
		return e.Error
	case e.Code != "":
		return e.Code
	case len(e.Errors) > 0:
		return e.Errors[0].Code
	}
	return ""
}

func unreachable(issuer string, err error) error {
	return fmt.Errorf("cannot reach Cauthy at %s: %w", issuer, err)
}

// uuid4 is a random RFC 9562 version 4 UUID, the assertion's jti.
func uuid4() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("no randomness for the assertion: %w", err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
