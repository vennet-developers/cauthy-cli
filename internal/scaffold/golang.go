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

// GoKey is the plan for a Go application on CAUTHY_KEY alone, as the
// five-minute guide wires it: cauthy.New and the route, nothing else.
func GoKey(a *detect.App) Plan { return goPlan(a, "") }

// Go is the plan for a Go application in contract mode.
//
// Contracts are retired: init writes it only with the deprecated --contracts.
func Go(a *detect.App, system string) Plan { return goPlan(a, system) }

// goPlan is the plan by contract for system, or on CAUTHY_KEY alone when
// system is "".
func goPlan(a *detect.App, system string) Plan {
	contract := system != ""
	p := Plan{Files: []File{{GoFile, goKeyAuth()}}}
	if contract {
		p.Files[0].Content = goAuth(system)
	}
	install := "go get " + GoModule + "@latest"
	importLine := fmt.Sprintf("  import \"%s/%s\"\n", a.Module, "internal/cauthyauth")
	open := "  auth, err := cauthyauth.New()\n  if err != nil {\n    log.Fatal(err) // the contract or the environment is wrong: never start without sign-in\n  }\n"
	if !contract {
		open = "  auth, err := cauthyauth.New()\n  if err != nil {\n    log.Fatal(err) // a missing or damaged key: never start without sign-in\n  }\n"
	}
	var mount string
	switch a.Framework {
	case "chi":
		mount = "  r.Handle(\"/api/auth/*\", auth)"
		if contract {
			mount += "\n  r.Handle(auth.JWKSPath(), auth)"
		}
	case "gin", "fiber", "echo":
		install += " " + GoModule + "/" + a.Framework + "@latest"
		router := map[string]string{"gin": "r", "fiber": "app", "echo": "e"}[a.Framework]
		importLine += fmt.Sprintf("  import cauthy%s \"%s/%s\"\n", a.Framework, GoModule, a.Framework)
		what := "/api/auth/*"
		if contract {
			what += " and the JWKS path"
		}
		mount = fmt.Sprintf("  cauthy%s.Mount(%s, auth) // %s", a.Framework, router, what)
	default:
		mount = "  auth.Mount(mux) // /api/auth/*"
		if contract {
			mount = "  mux.Handle(\"/api/auth/\", auth)\n  mux.Handle(auth.JWKSPath(), auth)"
		}
	}
	p.Install = install
	start := "Start the server and open http://localhost:8080/api/auth/login"
	if contract {
		start = "Start the server from the application's folder (it reads contracts/graph.json there) and open http://localhost:8080/api/auth/login"
	}
	p.Steps = append(p.Steps, "Add to main, where the router is made:\n\n"+importLine+"\n"+open+mount, start)
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
