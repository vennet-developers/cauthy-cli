package detect

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// folder makes an application folder; a path ending in / is a directory.
func folder(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, content := range files {
		full := filepath.Join(dir, p)
		if strings.HasSuffix(p, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestNode(t *testing.T) {
	a, err := Detect(folder(t, map[string]string{
		"package.json":   `{"name":"@acme/Books App","dependencies":{"next":"^16.0.1","react":"19"}}`,
		"src/app/":       "",
		"tsconfig.json":  "{}",
		"pnpm-lock.yaml": "",
	}), "")
	if err != nil {
		t.Fatal(err)
	}
	if a.Lang != "node" || a.Framework != "next" || a.Root != "src" || !a.TypeScript || a.NextMajor != 16 ||
		a.System != "books-app" || a.PackageManager != "pnpm" || a.DevURL != "http://localhost:3000" {
		t.Fatalf("%+v", a)
	}
	cases := map[string]string{
		`{"dependencies":{"@tanstack/react-start":"1"}}`: "tanstack-start",
		`{"dependencies":{"hono":"4","express":"5"}}`:    "hono",
		`{"dependencies":{"fastify":"5"}}`:               "fastify",
		`{"dependencies":{"express":"5"}}`:               "express",
		`{"dependencies":{"react":"19"}}`:                "",
	}
	for pkg, want := range cases {
		a, err := Detect(folder(t, map[string]string{"package.json": pkg}), "")
		if err != nil || a.Framework != want {
			t.Errorf("%s: %v %q, want %q", pkg, err, a.Framework, want)
		}
	}
	esm, _ := Detect(folder(t, map[string]string{"package.json": `{"type":"module","devDependencies":{"typescript":"5","express":"5"}}`}), "")
	if !esm.ESM || !esm.TypeScript || esm.Root != "" {
		t.Fatalf("%+v", esm)
	}
}

func TestGo(t *testing.T) {
	a, err := Detect(folder(t, map[string]string{"go.mod": "module github.com/acme/acme-crm\n\ngo 1.27\n\nrequire (\n\tgithub.com/go-chi/chi/v5 v5.2.0\n\tgithub.com/jackc/pgx/v5 v5.7.0 // indirect\n)\n"}), "")
	if err != nil {
		t.Fatal(err)
	}
	if a.Lang != "go" || a.Framework != "chi" || a.Module != "github.com/acme/acme-crm" || a.System != "acme-crm" || a.DevURL != "http://localhost:8080" {
		t.Fatalf("%+v", a)
	}
	for mod, want := range map[string]string{
		"require github.com/gin-gonic/gin v1.10.0":    "gin",
		"require github.com/gofiber/fiber/v2 v2.52.0": "fiber",
		"require github.com/labstack/echo/v4 v4.12.0": "echo",
		"require golang.org/x/text v0.20.0":           "net/http",
	} {
		a, err := Detect(folder(t, map[string]string{"go.mod": "module example.com/svc\n\n" + mod + "\n"}), "")
		if err != nil || a.Framework != want {
			t.Errorf("%s: %v %+v", mod, err, a)
		}
	}
}

func TestWhatCannotBeDetected(t *testing.T) {
	both := folder(t, map[string]string{"package.json": `{}`, "go.mod": "module x\n"})
	if _, err := Detect(both, ""); err == nil || !strings.Contains(err.Error(), "--lang") {
		t.Fatalf("got %v", err)
	}
	if a, err := Detect(both, "go"); err != nil || a.Lang != "go" {
		t.Fatalf("%v %+v", err, a)
	}
	if _, err := Detect(both, "python"); err == nil {
		t.Fatal("--lang python accepted")
	}
	if _, err := Detect(t.TempDir(), ""); err == nil || !strings.Contains(err.Error(), "no package.json or go.mod") {
		t.Fatalf("got %v", err)
	}
	if _, err := Detect(folder(t, map[string]string{"package.json": `{`}), ""); err == nil {
		t.Fatal("a broken package.json was accepted")
	}
}

func TestSystemName(t *testing.T) {
	for in, want := range map[string]string{"@scope/My_Shop.web": "my_shop-web", "": "app", "---": "app", "acme-crm": "acme-crm"} {
		if got := SystemName(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
