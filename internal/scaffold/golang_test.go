package scaffold

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-cli/internal/detect"
)

func TestGoWiring(t *testing.T) {
	p := GoKey(&detect.App{Lang: "go", Framework: "chi", Module: "github.com/acme/acme-crm"})
	if paths(p) != GoFile {
		t.Fatalf("%s", paths(p))
	}
	src := files(p)[GoFile]
	for _, want := range []string{`cauthy "github.com/vennet-developers/cauthy-sdk-go"`, "func New() (*cauthy.Auth, error)", "CAUTHY_KEY"} {
		if !strings.Contains(src, want) {
			t.Errorf("missing %q in\n%s", want, src)
		}
	}
	for _, not := range []string{"database/sql", "keys.", "Keys:", "PACT_ROOT_KEY", "sql.DB"} {
		if strings.Contains(src, not) {
			t.Errorf("%q in\n%s", not, src)
		}
	}
	steps := strings.Join(p.Steps, "\n")
	if !strings.Contains(steps, `import "github.com/acme/acme-crm/internal/cauthyauth"`) || !strings.Contains(steps, `r.Handle("/api/auth/*", auth)`) ||
		!strings.Contains(steps, "auth, err := cauthyauth.New()\n") || strings.Contains(steps, "db") {
		t.Fatalf("%s", steps)
	}
	if p.Install != "go get github.com/vennet-developers/cauthy-sdk-go@latest" {
		t.Fatalf("%q", p.Install)
	}
	gin := GoKey(&detect.App{Lang: "go", Framework: "gin", Module: "example.com/svc"})
	if !strings.Contains(gin.Install, "cauthy-sdk-go/gin@latest") || !strings.Contains(strings.Join(gin.Steps, "\n"), "cauthygin.Mount(r, auth) // /api/auth/*") {
		t.Fatalf("%q %s", gin.Install, gin.Steps)
	}
}

func TestGoOnKeyAlone(t *testing.T) {
	p := GoKey(&detect.App{Lang: "go", Framework: "net/http", Module: "example.com/svc"})
	src := files(p)[GoFile]
	if _, err := parser.ParseFile(token.NewFileSet(), "cauthyauth.go", src, parser.AllErrors); err != nil {
		t.Fatalf("the file is not Go: %v\n%s", err, src)
	}
	if !strings.Contains(src, "cauthy.New(cauthy.Options{})") || strings.Contains(src, "Contract") || strings.Contains(src, "graph") {
		t.Fatalf("%s", src)
	}
	steps := strings.Join(p.Steps, "\n")
	if !strings.Contains(steps, "auth.Mount(mux) // /api/auth/*") || strings.Contains(steps, "JWKSPath") || strings.Contains(steps, "graph") {
		t.Fatalf("%s", steps)
	}
	echo := GoKey(&detect.App{Lang: "go", Framework: "echo", Module: "example.com/svc"})
	echoSteps := strings.Join(echo.Steps, "\n")
	if !strings.Contains(echo.Install, "cauthy-sdk-go/echo@latest") || !strings.Contains(echoSteps, "cauthyecho.Mount(e, auth) // /api/auth/*") || strings.Contains(echoSteps, "JWKS") {
		t.Fatalf("%q %s", echo.Install, echo.Steps)
	}
}
