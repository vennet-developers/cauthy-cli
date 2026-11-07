package scaffold

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-cli/internal/detect"
)

func TestGoWiring(t *testing.T) {
	p := Go(&detect.App{Lang: "go", Framework: "chi", Module: "github.com/acme/acme-crm"}, "acme-crm")
	if paths(p) != GoFile {
		t.Fatalf("%s", paths(p))
	}
	src := files(p)[GoFile]
	if _, err := parser.ParseFile(token.NewFileSet(), "cauthyauth.go", src, parser.AllErrors); err != nil {
		t.Fatalf("the file is not Go: %v\n%s", err, src)
	}
	for _, want := range []string{`const System = "acme-crm"`, `os.ReadFile("contracts/graph.json")`, "keys.Postgres(db)", `cauthy "github.com/vennet-developers/cauthy-sdk-go"`} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q", want)
		}
	}
	steps := strings.Join(p.Steps, "\n")
	if !strings.Contains(steps, `import "github.com/acme/acme-crm/internal/cauthyauth"`) || !strings.Contains(steps, `r.Handle("/api/auth/*", auth)`) {
		t.Fatalf("%s", steps)
	}
	if p.Install != "go get github.com/vennet-developers/cauthy-sdk-go@latest" {
		t.Fatalf("%q", p.Install)
	}
	gin := Go(&detect.App{Lang: "go", Framework: "gin", Module: "example.com/svc"}, "svc")
	if !strings.Contains(gin.Install, "cauthy-sdk-go/gin@latest") || !strings.Contains(strings.Join(gin.Steps, "\n"), "cauthygin.Mount(r, auth)") {
		t.Fatalf("%q %s", gin.Install, gin.Steps)
	}
	plain := Go(&detect.App{Lang: "go", Framework: "net/http", Module: "example.com/svc"}, "svc")
	if !strings.Contains(strings.Join(plain.Steps, "\n"), `mux.Handle("/api/auth/", auth)`) {
		t.Fatalf("%s", plain.Steps)
	}
}
