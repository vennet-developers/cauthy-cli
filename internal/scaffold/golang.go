package scaffold

import (
	"fmt"

	"github.com/vennet-developers/cauthy-cli/internal/detect"
)

// GoModule is the SDK the Go file imports. The names it uses (cauthy.New,
// Options, Contract, keys.Postgres, *cauthy.Auth as an http.Handler,
// JWKSPath, the adapters' Mount) are what stage 4's cauthy-sdk-go must export.
const GoModule = "github.com/vennet-developers/cauthy-sdk-go"

// GoFile is where the wiring goes in a Go application.
const GoFile = "internal/cauthyauth/cauthyauth.go"

// Go is the plan for a Go application in contract mode.
func Go(a *detect.App, system string) Plan {
	p := Plan{Files: []File{{GoFile, goAuth(system)}}}
	install := "go get " + GoModule + "@latest"
	importLine := fmt.Sprintf("  import \"%s/%s\"\n", a.Module, "internal/cauthyauth")
	open := "  auth, err := cauthyauth.New(db) // db is the application's *sql.DB (pgx's stdlib driver or lib/pq)\n  if err != nil {\n    log.Fatal(err) // the contract or the environment is wrong: never start without sign-in\n  }\n"
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
	return fmt.Sprintf(`// Package cauthyauth signs people in with Cauthy by contract: Cauthy's
// address, the tenant and the clients come from contracts/graph.json (cauthy
// contracts pull); the only secret is PACT_ROOT_KEY, and the application's own
// key lives sealed in the cauthy_pact_keys table. Written by cauthy init.
package cauthyauth

import (
	"database/sql"
	"os"

	cauthy "%[1]s"
	"%[1]s/keys"
)

// System is this application's id in the Pact graph.
const System = %[2]q

// New reads contracts/graph.json from the working directory (start the
// server from the application's folder) and PACT_ROOT_KEY from the
// environment. It refuses to start on anything contract/pact.md lists.
func New(db *sql.DB) (*cauthy.Auth, error) {
	graph, err := os.ReadFile("contracts/graph.json")
	if err != nil {
		return nil, err
	}
	return cauthy.New(cauthy.Options{
		Contract: &cauthy.Contract{Graph: graph, System: System},
		Keys:     keys.Postgres(db),
	})
}
`, GoModule, system)
}
