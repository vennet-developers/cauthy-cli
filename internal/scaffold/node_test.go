package scaffold

import (
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-cli/internal/detect"
)

func files(p Plan) map[string]string {
	m := map[string]string{}
	for _, f := range p.Files {
		m[f.Path] = f.Content
	}
	return m
}

func paths(p Plan) string {
	var out []string
	for _, f := range p.Files {
		out = append(out, f.Path)
	}
	return strings.Join(out, " ")
}

func TestImportPath(t *testing.T) {
	for _, c := range [][3]string{
		{"src/lib/auth.ts", "contracts/graph.json", "../../contracts/graph.json"},
		{"lib/auth.ts", "contracts/graph.json", "../contracts/graph.json"},
		{"app/api/auth/[...cauthy]/route.ts", "lib/auth.ts", "../../../../lib/auth"},
		{"src/routes/[.]well-known/pact-jwks[.]json.ts", "src/auth.ts", "../../auth"},
		{"middleware.ts", "lib/auth.ts", "./lib/auth"},
	} {
		if got := importPath(c[0], c[1]); got != c[2] {
			t.Errorf("%s → %s: %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestNextInContractMode(t *testing.T) {
	p := Node(&detect.App{Lang: "node", Framework: "next", Root: "src", TypeScript: true, NextMajor: 16, PackageManager: "pnpm", Deps: map[string]bool{"next": true}}, "acme-crm")
	if paths(p) != "src/lib/auth.ts src/app/api/auth/[...cauthy]/route.ts src/app/.well-known/pact-jwks.json/route.ts src/proxy.ts" {
		t.Fatalf("%s", paths(p))
	}
	f := files(p)
	auth := f["src/lib/auth.ts"]
	want := `import graph from '../../contracts/graph.json'
import { createCauthy } from '@cauthyhq/node'
import { cauthyNext } from '@cauthyhq/node/next'

// Reads CAUTHY_KEY; the contract adds the system proof and its policies.
export const auth = createCauthy({ contract: { graph, system: 'acme-crm' } })
export const cauthy = cauthyNext(auth)
`
	if auth != want {
		t.Fatalf("auth file:\n%s\nwant:\n%s", auth, want)
	}
	if !strings.Contains(f["src/app/.well-known/pact-jwks.json/route.ts"], "export const GET = () => auth.pact!.jwks()") {
		t.Fatal(f["src/app/.well-known/pact-jwks.json/route.ts"])
	}
	if p.Install != "pnpm add @cauthyhq/node" {
		t.Fatalf("install %q", p.Install)
	}
	old := Node(&detect.App{Lang: "node", Framework: "next", NextMajor: 15, NextMinor: 5, PackageManager: "npm", Deps: map[string]bool{"@cauthyhq/node": true}}, "acme-crm")
	if !strings.HasSuffix(paths(old), " middleware.js") || old.Install != "" {
		t.Fatalf("%s %q", paths(old), old.Install)
	}
	// Before 16 the middleware runs on the Edge runtime unless it says
	// otherwise, and the SDK runs on Node.js.
	if mw := files(old)["middleware.js"]; !strings.Contains(mw, "runtime: 'nodejs'") {
		t.Fatalf("%s", mw)
	}
	if strings.Contains(f["src/proxy.ts"], "runtime") {
		t.Fatal("Next 16's proxy runs on Node.js: no runtime to say")
	}
}

func TestNextBefore155IsRefused(t *testing.T) {
	for _, v := range [][2]int{{15, 4}, {14, 2}, {13, 0}} {
		err := Supported(&detect.App{Lang: "node", Framework: "next", NextMajor: v[0], NextMinor: v[1]})
		if err == nil || !strings.Contains(err.Error(), "Next.js 15.5 or newer") {
			t.Errorf("%v: %v", v, err)
		}
	}
	for _, a := range []*detect.App{
		{Lang: "node", Framework: "next", NextMajor: 15, NextMinor: 5},
		{Lang: "node", Framework: "next", NextMajor: 16},
		{Lang: "node", Framework: "next"}, // a version init cannot read: the current one
		{Lang: "node", Framework: "hono"},
		{Lang: "go", Framework: "chi"},
	} {
		if err := Supported(a); err != nil {
			t.Errorf("%+v: %v", a, err)
		}
	}
}

func TestTanStackStart(t *testing.T) {
	p := Node(&detect.App{Lang: "node", Framework: "tanstack-start", Root: "src", TypeScript: true, PackageManager: "npm", Deps: map[string]bool{}}, "acme-crm")
	if paths(p) != "src/auth.ts src/routes/api/auth/$.ts src/routes/[.]well-known/pact-jwks[.]json.ts" {
		t.Fatalf("%s", paths(p))
	}
	f := files(p)
	if !strings.Contains(f["src/routes/[.]well-known/pact-jwks[.]json.ts"], "import { auth } from '../../auth'") {
		t.Fatalf("%v", f)
	}
	want := `import graph from '../contracts/graph.json'
import { createCauthy } from '@cauthyhq/node'

// Reads CAUTHY_KEY; the contract adds the system proof and its policies.
export const auth = createCauthy({ contract: { graph, system: 'acme-crm' } })
`
	if f["src/auth.ts"] != want {
		t.Fatalf("auth file:\n%s\nwant:\n%s", f["src/auth.ts"], want)
	}
}

func TestServersGetTheLinesToAdd(t *testing.T) {
	cjs := Node(&detect.App{Lang: "node", Framework: "express", PackageManager: "npm", Deps: map[string]bool{"express": true}}, "acme-crm")
	auth := files(cjs)["auth.js"]
	want := `const { readFileSync } = require('node:fs')
const { createCauthy } = require('@cauthyhq/node')

// Read from the working directory: start the server from the application's folder.
const graph = JSON.parse(readFileSync('contracts/graph.json', 'utf8'))

// Reads CAUTHY_KEY; the contract adds the system proof and its policies.
exports.auth = createCauthy({ contract: { graph, system: 'acme-crm' } })
`
	if auth != want {
		t.Fatalf("auth file:\n%s\nwant:\n%s", auth, want)
	}
	if esm := files(Node(&detect.App{Lang: "node", Framework: "hono", ESM: true, PackageManager: "npm", Deps: map[string]bool{}}, "acme-crm"))["auth.js"]; !strings.HasPrefix(esm, "import { readFileSync } from 'node:fs'\nimport { createCauthy } from '@cauthyhq/node'\n") {
		t.Fatalf("%s", esm)
	}
	steps := strings.Join(cjs.Steps, "\n")
	// The @cauthyhq/node 1.0 adapters serve the JWKS path themselves: no extra route.
	if !strings.Contains(steps, "const { cauthyExpress } = require('@cauthyhq/node/express')") || !strings.Contains(steps, "app.use(cauthyExpress(auth))") || strings.Contains(steps, "pact-jwks") {
		t.Fatalf("%s", steps)
	}
	hono := Node(&detect.App{Lang: "node", Framework: "hono", Root: "src", TypeScript: true, ESM: true, PackageManager: "npm", Deps: map[string]bool{}}, "acme-crm")
	if paths(hono) != "src/auth.ts" || !strings.Contains(strings.Join(hono.Steps, "\n"), "app.use(cauthyHono(auth))") || strings.Contains(strings.Join(hono.Steps, "\n"), "pact-jwks") {
		t.Fatalf("%s\n%s", paths(hono), hono.Steps)
	}
	fastify := Node(&detect.App{Lang: "node", Framework: "fastify", ESM: true, PackageManager: "npm", Deps: map[string]bool{}}, "acme-crm")
	if !strings.Contains(strings.Join(fastify.Steps, "\n"), "await app.register(cauthyFastify, { auth })") || strings.Contains(strings.Join(fastify.Steps, "\n"), "pact-jwks") {
		t.Fatalf("%s", fastify.Steps)
	}
	unknown := Node(&detect.App{Lang: "node", PackageManager: "npm", Deps: map[string]bool{}}, "acme-crm")
	if paths(unknown) != "auth.js" || !strings.Contains(unknown.Steps[0], "Mount the route") {
		t.Fatalf("%s %s", paths(unknown), unknown.Steps)
	}
}

// On CAUTHY_KEY the wiring has no key store: no pg, no DATABASE_URL, no
// PACT_ROOT_KEY.
func TestNodeWiringHasNoKeyStore(t *testing.T) {
	for _, a := range []*detect.App{
		{Lang: "node", Framework: "next", TypeScript: true, NextMajor: 16, PackageManager: "npm", Deps: map[string]bool{}},
		{Lang: "node", Framework: "tanstack-start", TypeScript: true, PackageManager: "npm", Deps: map[string]bool{}},
		{Lang: "node", Framework: "express", PackageManager: "npm", Deps: map[string]bool{}},
		{Lang: "node", PackageManager: "npm", Deps: map[string]bool{}},
	} {
		p := Node(a, "acme-crm")
		for path, content := range files(p) {
			for _, not := range []string{"pg", "postgresKeyStore", "DATABASE_URL", "PACT_ROOT_KEY", "keys:"} {
				if strings.Contains(content, not) {
					t.Errorf("%s %s: %q in\n%s", a.Framework, path, not, content)
				}
			}
		}
		if p.Install != "npm install @cauthyhq/node" {
			t.Errorf("%s: install %q", a.Framework, p.Install)
		}
	}
}
