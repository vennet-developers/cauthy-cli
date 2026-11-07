package pact

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/vennet-developers/cauthy-cli/internal/graph"
)

// StandardVariables are the variables of the SDKs' variables mode. With a
// contract none may be set (contract/pact.md, rule 6).
var StandardVariables = []string{
	"CAUTHY_ISSUER", "CAUTHY_CLIENT_ID", "CAUTHY_CLIENT_SECRET", "CAUTHY_SECRET",
	"CAUTHY_SYSTEM", "CAUTHY_TENANT", "CAUTHY_SYSTEM_KEY", "CAUTHY_SYSTEM_KEY_ID",
}

// MinRootKey is the shortest PACT_ROOT_KEY an SDK accepts.
const MinRootKey = 32

var proofText = map[string]string{
	"required": "required on every sign-in through this client",
	"auto":     "auto: required once the first sign-in carries it",
	"off":      "sent, not required",
}

// Describe is what the contract says, as the SDK will read it.
func Describe(c *graph.Contract) string {
	lines := []string{
		fmt.Sprintf("%s signs in with Cauthy by the link %s", c.System, c.Link),
		fmt.Sprintf("  Cauthy          %s (tenant %s, %s)", c.Issuer, c.Tenant, c.TenantID),
		fmt.Sprintf("  sign-in client  %s", c.ClientID),
		fmt.Sprintf("  return to       %s", strings.Join(c.RedirectURIs, ", ")),
		fmt.Sprintf("  system proof    %s", proofText[c.SystemProof]),
		fmt.Sprintf("  verified email  %s", yesNo(c.RequireVerifiedEmail, "required: no token before the address is confirmed", "not required by the link")),
		fmt.Sprintf("  stay signed in  %s", yesNo(c.PersistentSessions, "offered when the person asks", "not offered")),
	}
	if c.ServiceClientID != "" {
		lines = append(lines, fmt.Sprintf("  own identity    %s, keys at %s", c.ServiceClientID, c.JWKSURL))
	} else {
		lines = append(lines, "  own identity    none (the link has no service block)")
	}
	if c.EventsURL != "" {
		lines = append(lines, fmt.Sprintf("  events          %s", c.EventsURL))
	}
	for i := range lines {
		lines[i] = Clean(lines[i])
	}
	return strings.Join(lines, "\n") + "\n"
}

func yesNo(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

// Offline checks what can be known without a network: the environment the
// application would start in, and what the link leaves weak. It never prints
// a variable's value.
func Offline(r *Report, c *graph.Contract, getenv func(string) string) {
	var set []string
	for _, name := range StandardVariables {
		if getenv(name) != "" {
			set = append(set, name)
		}
	}
	if len(set) > 0 {
		r.Add("env.variables_mode", Fail, "%s set: the SDK refuses to start by contract with them; remove them", strings.Join(set, ", "))
	} else {
		r.Add("env.variables_mode", Pass, "no CAUTHY_* variable of the variables mode is set")
	}

	root, previous := getenv("PACT_ROOT_KEY"), getenv("PACT_ROOT_KEY_PREVIOUS")
	switch {
	case root == "":
		r.Add("env.root_key", Advisory, "PACT_ROOT_KEY is not set in this shell; the deployment must set it (cauthy keys new-root makes one)")
	case utf8.RuneCountInString(root) < MinRootKey:
		r.Add("env.root_key", Fail, "PACT_ROOT_KEY is too short: at least %d characters (cauthy keys new-root makes one)", MinRootKey)
	default:
		r.Add("env.root_key", Pass, "PACT_ROOT_KEY is set and long enough")
	}
	switch {
	case previous == "":
	case utf8.RuneCountInString(previous) < MinRootKey:
		r.Add("env.root_key_previous", Fail, "PACT_ROOT_KEY_PREVIOUS is too short: at least %d characters", MinRootKey)
	case previous == root:
		r.Add("env.root_key_previous", Advisory, "PACT_ROOT_KEY_PREVIOUS equals PACT_ROOT_KEY: remove it")
	default:
		r.Add("env.root_key_previous", Pass, "PACT_ROOT_KEY_PREVIOUS is set: a root rotation is under way; reseal, then remove it")
	}

	if c.SystemProof == "off" {
		r.Add("link.system_proof", Advisory, "the link does not require the system proof: a copied client id can finish a sign-in; ask Vennet for require_system_proof: auto")
	} else {
		r.Add("link.system_proof", Pass, "system proof %s", c.SystemProof)
	}
	if c.RequireVerifiedEmail {
		r.Add("link.verified_email", Pass, "a valid token means a confirmed address")
	} else {
		r.Add("link.verified_email", Advisory, "require_verified_email is off: a valid token does not mean a confirmed address")
	}
}
