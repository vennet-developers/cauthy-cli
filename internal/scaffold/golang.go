package scaffold

import (
	"fmt"

	"github.com/vennet-developers/cauthy-cli/internal/detect"
)

// GoModule is the SDK the Go file imports. The names it uses (cauthy.New,
// Options, Contract, *cauthy.Auth as an http.Handler, JWKSPath, the
// adapters' Mount) are what cauthy-sdk-go 1.2 exports.
const GoModule = "github.com/vennet-developers/cauthy-sdk-go"

// GoFile is where the wiring goes in a Go application.
const GoFile = "internal/cauthyauth/cauthyauth.go"

// Go is the plan for a Go application in contract mode.
func Go(a *detect.App, system string) Plan {
	p := Plan{Files: []File{{GoFile, goAuth(system)}}}
	install := "go get " + GoModule + "@latest"
	importLine := fmt.Sprintf("  import \"%s/%s\"\n", a.Module, "internal/cauthyauth")
	open := "  auth, err := cauthyauth.New()\n  if err != nil {\n    log.Fatal(err) // the contract or the environment is wrong: never start without sign-in\n  }\n"
	var mount string
	switch a.Framework {
	case "chi":
		mount = "  r.Handle(\"/api/auth/*\", auth)\n  r.Handle(auth.JWKSPath(), auth)"
	case "gin", "fiber", "echo":
		install += " " + GoModule + "/" + a.Framework + "@latest"
		router := map[string]string{"gin": "r", "fiber": "app", "echo": "e"}[a.Framework]
		importLine += fmt.Sprintf("  import cauthy%s \"%s/%s\"\n", a.Framework, GoModule, a.Framework)
		mount = fmt.Sprintf("  cauthy%s.Mount(%s, auth) // /api/auth/* and the JWKS path", a.Framework, router)
	default:
		mount = "  mux.Handle(\"/api/auth/\", auth)\n  mux.Handle(auth.JWKSPath(), auth)"
	}
	p.Install = install
	p.Steps = append(p.Steps,
		"Add to main, where the router is made:\n\n"+importLine+"\n"+open+mount,
		"Start the server from the application's folder (it reads contracts/graph.json there) and open http://localhost:8080/api/auth/login")
	return p
}

func goAuth(system string) string {
	return fmt.Sprintf(`// Package cauthyauth signs people in with Cauthy by contract. The
// application's only setting is CAUTHY_KEY; Cauthy's address, the workspace
// and the clients come from contracts/graph.json (cauthy contracts pull), and
// the contract adds the system proof and its policies. Written by cauthy init.
package cauthyauth

import (
	"os"

	cauthy "%[1]s"
)

// New reads contracts/graph.json from the working directory (start the
// server from the application's folder) and CAUTHY_KEY from the
// environment. It refuses to start on anything contract/pact.md lists.
func New() (*cauthy.Auth, error) {
	graph, err := os.ReadFile("contracts/graph.json")
	if err != nil {
		return nil, err
	}
	return cauthy.New(cauthy.Options{Contract: &cauthy.Contract{Graph: graph, System: %[2]q}})
}
`, GoModule, system)
}
