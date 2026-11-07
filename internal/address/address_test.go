package address

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	for in, want := range map[string]string{
		"https://user:pw@contracts.example.com/graph.json?token=SECRET#frag": "https://contracts.example.com/graph.json",
		"https://contracts.example.com/graph.json":                           "https://contracts.example.com/graph.json",
		"http://localhost:3000/x?a=SECRET":                                   "http://localhost:3000/x",
		"https://u:pw@bad host/x?token=SECRET":                               "https://bad host/x",
		"not an address?token=SECRET":                                        "not%20an%20address",
	} {
		if got := Redact(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

func TestHTTPSNeverShowsASecret(t *testing.T) {
	for _, raw := range []string{
		"http://user:pw@contracts.example.com/g?token=SECRET",
		"ftp://contracts.example.com/g?token=SECRET",
		"https://u:pw@bad host/x?token=SECRET",
		"/relative?token=SECRET",
	} {
		_, err := HTTPS(raw, "graph address")
		if err == nil || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "pw") {
			t.Errorf("%q: %v", raw, err)
		}
	}
	for _, ok := range []string{"https://contracts.example.com/g", "http://localhost:3000", "http://127.0.0.1:8080/x"} {
		if _, err := HTTPS(ok, "address"); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
}

func TestErrRedactsTheRequestAddress(t *testing.T) {
	_, err := http.Get("http://127.0.0.1:1/graph.json?token=SECRET")
	if err == nil {
		t.Skip("something listens on port 1")
	}
	if got := Err(err); strings.Contains(got.Error(), "SECRET") || !strings.Contains(got.Error(), "127.0.0.1:1/graph.json") {
		t.Fatalf("%v", got)
	}
	plain := errors.New("plain")
	if Err(plain) != plain {
		t.Fatal("an error that is not a request's is answered as it is")
	}
}

func TestRedirectsStayOnHTTPS(t *testing.T) {
	var srv *httptest.Server
	hops := 0
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/down":
			http.Redirect(w, r, "http://contracts.example.com/graph.json?token=SECRET", http.StatusFound)
		case "/local":
			http.Redirect(w, r, "http://localhost:1/graph.json", http.StatusFound)
		case "/loop":
			hops++
			http.Redirect(w, r, srv.URL+"/loop", http.StatusFound)
		case "/three":
			hops++
			if hops <= 3 {
				http.Redirect(w, r, srv.URL+"/three", http.StatusFound)
				return
			}
			w.Write([]byte("ok"))
		}
	}))
	defer srv.Close()
	c := Client(srv.Client())
	if c == srv.Client() || srv.Client().CheckRedirect != nil {
		t.Fatal("Client must answer a copy and leave the given client alone")
	}
	for _, path := range []string{"/down", "/local"} {
		_, err := c.Get(srv.URL + path)
		err = Err(err) // the request's own error names the address it was sent to
		if err == nil || !strings.Contains(err.Error(), "refused a redirect") || strings.Contains(err.Error(), "SECRET") {
			t.Errorf("%s: %v", path, err)
		}
	}
	hops = 0
	if _, err := c.Get(srv.URL + "/loop"); err == nil || !strings.Contains(err.Error(), "more than 3 redirects") {
		t.Fatalf("loop: %v", err)
	}
	hops = 0
	res, err := c.Get(srv.URL + "/three")
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("three redirects are allowed: %v", err)
	}
	res.Body.Close()
}
