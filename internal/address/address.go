// Package address reads the addresses the CLI is given or finds in the graph,
// and keeps what they may carry (user info, a token in the query) out of
// every message. It also bounds the redirects the CLI follows.
package address

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// MaxRedirects is how many redirects a request follows before it gives up.
const MaxRedirects = 3

// Redact is raw without user info, query or fragment: what a message may show.
func Redact(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		u.User = nil
		u.RawQuery, u.ForceQuery = "", false
		u.Fragment, u.RawFragment = "", ""
		return u.String()
	}
	s := raw
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "://"); i >= 0 {
		rest := s[i+3:]
		host := rest
		if j := strings.Index(rest, "/"); j >= 0 {
			host = rest[:j]
		}
		if k := strings.LastIndex(host, "@"); k >= 0 {
			s = s[:i+3] + rest[k+1:]
		}
	}
	return s
}

// HTTPS parses raw and accepts https, or http on localhost and 127.0.0.1.
// Its errors show the address redacted.
func HTTPS(raw, what string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("the %s is not an address: %q", what, Redact(raw))
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && local(u)) {
		return nil, fmt.Errorf("the %s must be https: %s", what, Redact(raw))
	}
	return u, nil
}

func local(u *url.URL) bool { return u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" }

// Err is a request's error with its address redacted. Any other error is
// answered as it is.
func Err(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	return fmt.Errorf("%s %s: %w", ue.Op, Redact(ue.URL), ue.Err)
}

// Client answers a copy of c that follows at most MaxRedirects redirects,
// each to an address HTTPS accepts and never from https down to http.
func Client(c *http.Client) *http.Client {
	if c == nil {
		c = http.DefaultClient
	}
	out := *c
	out.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > MaxRedirects {
			return fmt.Errorf("more than %d redirects", MaxRedirects)
		}
		target := Redact(req.URL.String())
		if _, err := HTTPS(req.URL.String(), "redirect"); err != nil {
			return fmt.Errorf("refused a redirect to %s: it is not https", target)
		}
		if req.URL.Scheme == "http" && via[0].URL.Scheme == "https" {
			return fmt.Errorf("refused a redirect to %s: from https down to http", target)
		}
		return nil
	}
	return &out
}
