package pull

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const graphJSON = `{"nodes":[{"id":"cauthy","type":"system"},{"id":"acme-crm","type":"system"},{"id":"e","type":"event"}],"edges":[{"relation":"links"},{"relation":"emits"}]}`

const commit = "f4bf762aabbccddeeff00112233445566778899a"

// repo makes a core-contracts checkout and an Exec that answers git and node
// like the real ones: graph.mjs writes graphJSON to the path it is given.
func repo(t *testing.T, dirty bool) (string, *[]string, func(dir, name string, args ...string) ([]byte, error)) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "core-contracts")
	if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scripts", "graph.mjs"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var ran []string
	exec := func(d, name string, args ...string) ([]byte, error) {
		ran = append(ran, name+" "+strings.Join(args, " "))
		switch {
		case name == "git" && args[0] == "rev-parse":
			return []byte(commit + "\n"), nil
		case name == "git" && args[0] == "status":
			if dirty {
				return []byte(" M links/acme-crm.yaml\n"), nil
			}
			return nil, nil
		case name == "node":
			return nil, os.WriteFile(args[1], []byte(graphJSON), 0o644)
		}
		return nil, fmt.Errorf("unexpected %s", name)
	}
	return dir, &ran, exec
}

func TestPullFromACheckoutIntoTwoFolders(t *testing.T) {
	src, ran, exec := repo(t, false)
	app := t.TempDir()
	d := Deps{Dir: app, Exec: exec}
	s, err := Fetch(d, src, false)
	if err != nil {
		t.Fatal(err)
	}
	// The graph is generated into a temporary file, never into the checkout.
	if len(*ran) != 3 || (*ran)[0] != "git rev-parse HEAD" || (*ran)[1] != "git status --porcelain" ||
		!strings.HasPrefix((*ran)[2], "node scripts/graph.mjs ") || strings.HasPrefix(strings.Fields((*ran)[2])[2], src) {
		t.Fatalf("ran %q", *ran)
	}
	line, err := Write(d, s, []string{"contracts", "web/contracts"})
	if err != nil {
		t.Fatal(err)
	}
	if line != "graph f4bf762a → contracts, web/contracts (2 systems, 1 links); every copy identical" {
		t.Fatalf("said %q", line)
	}
	for _, dest := range []string{"contracts", "web/contracts"} {
		raw, _ := os.ReadFile(filepath.Join(app, dest, "graph.json"))
		version, _ := os.ReadFile(filepath.Join(app, dest, "VERSION"))
		if string(raw) != graphJSON+"\n" || string(version) != commit+"\n" {
			t.Fatalf("%s: %q %q", dest, raw, version)
		}
	}
}

func TestADirtyCheckoutIsRefusedUnlessAllowed(t *testing.T) {
	src, _, exec := repo(t, true)
	d := Deps{Dir: t.TempDir(), Exec: exec}
	if _, err := Fetch(d, src, false); err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("got %v", err)
	}
	s, err := Fetch(d, src, true)
	if err != nil || s.Version != commit+"-dirty" || Short(s.Version) != "f4bf762a-dirty" {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestAFolderWithoutGitOrScript(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "graph.json"), []byte(graphJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	notGit := func(string, string, ...string) ([]byte, error) { return nil, errors.New("not a git repository") }
	s, err := Fetch(Deps{Dir: t.TempDir(), Exec: notGit}, dir, false)
	if err != nil || s.Version != "unversioned" {
		t.Fatalf("%v %+v", err, s)
	}
	if _, err := Fetch(Deps{Dir: t.TempDir(), Exec: notGit}, filepath.Join(dir, "nowhere"), false); err == nil || !strings.Contains(err.Error(), "no contracts repository") {
		t.Fatalf("got %v", err)
	}
}

func TestPullFromAnAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/graph.json":
			fmt.Fprint(w, graphJSON)
		case "/hello.json":
			fmt.Fprint(w, `{"hello":1}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	d := Deps{Dir: t.TempDir(), HTTP: srv.Client()}
	s, err := Fetch(d, srv.URL+"/graph.json?token=s3cret", false)
	if err != nil || !strings.HasPrefix(s.Version, "sha256:") || s.Systems != 2 {
		t.Fatalf("%v %+v", err, s)
	}
	if _, err := Fetch(d, srv.URL+"/hello.json", false); err == nil || !strings.Contains(err.Error(), "not a context graph") {
		t.Fatalf("got %v", err)
	}
	_, err = Fetch(d, srv.URL+"/missing.json?token=s3cret", false)
	if err == nil || !strings.Contains(err.Error(), "answered 404") || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("got %v", err)
	}
	if _, err := Fetch(d, "http://contracts.example.com/graph.json", false); err == nil || !strings.Contains(err.Error(), "must be https") {
		t.Fatalf("got %v", err)
	}
}

func TestCheckFindsDrift(t *testing.T) {
	app := t.TempDir()
	d := Deps{Dir: app}
	s, err := source([]byte(graphJSON), commit)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Write(d, s, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if err := Check(d, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "b", "VERSION"), []byte("0000000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Check(d, []string{"a", "b"}); err == nil || !strings.Contains(err.Error(), "a and b hold different graphs") {
		t.Fatalf("got %v", err)
	}
	if err := Check(d, []string{"a", "c"}); err == nil || !strings.Contains(err.Error(), "c has no graph.json") {
		t.Fatalf("got %v", err)
	}
	if _, err := Write(d, s, []string{"a", "./a"}); err == nil || !strings.Contains(err.Error(), "given twice") {
		t.Fatalf("got %v", err)
	}
}
