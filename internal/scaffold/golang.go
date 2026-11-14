package scaffold

import (
	"fmt"

	"github.com/vennet-developers/cauthy-cli/internal/detect"
)

// GoModule is the SDK the Go file imports. The names it uses (cauthy.New,
// Options, *cauthy.Auth's Mount, the adapters' Mount) are what cauthy-sdk-go
// 1.2 exports.
const GoModule = "github.com/vennet-developers/cauthy-sdk-go"

// GoFile is where the wiring goes in a Go application.
const GoFile = "internal/cauthyauth/cauthyauth.go"

// GoKey is the plan for a Go application on CAUTHY_KEY alone, as the
// five-minute guide wires it: cauthy.New and the route, nothing else.
func GoKey(a *detect.App) Plan {
	p := Plan{Files: []File{{GoFile, goKeyAuth()}}}
	install := "go get " + GoModule + "@latest"
	importLine := fmt.Sprintf("  import \"%s/%s\"\n", a.Module, "internal/cauthyauth")
	open := "  auth, err := cauthyauth.New()\n  if err != nil {\n    log.Fatal(err) // a missing or damaged key: never start without sign-in\n  }\n"
	var mount string
	switch a.Framework {
	case "chi":
		mount = "  r.Handle(\"/api/auth/*\", auth)"
	case "gin", "fiber", "echo":
		install += " " + GoModule + "/" + a.Framework + "@latest"
		router := map[string]string{"gin": "r", "fiber": "app", "echo": "e"}[a.Framework]
		importLine += fmt.Sprintf("  import cauthy%s \"%s/%s\"\n", a.Framework, GoModule, a.Framework)
		mount = fmt.Sprintf("  cauthy%s.Mount(%s, auth) // /api/auth/*", a.Framework, router)
	default:
		mount = "  auth.Mount(mux) // /api/auth/*"
	}
	p.Install = install
	p.Steps = append(p.Steps, "Add to main, where the router is made:\n\n"+importLine+"\n"+open+mount,
		"Start the server and open http://localhost:8080/api/auth/login")
	return p
}

func goKeyAuth() string {
	return fmt.Sprintf(`// Package cauthyauth signs people in with Cauthy. The application's only
// setting is CAUTHY_KEY: Cauthy's address, the workspace and the client come
// from it and from Cauthy. Written by cauthy init.
package cauthyauth

import cauthy "%s"

// New reads CAUTHY_KEY from the environment. It refuses to start when the
// key is missing or damaged.
func New() (*cauthy.Auth, error) {
	return cauthy.New(cauthy.Options{})
}
`, GoModule)
}
