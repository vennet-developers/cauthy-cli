package scaffold

import (
	"fmt"
	"path"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/detect"
)

// NodePackage is the SDK the Node files import.
const NodePackage = "@cauthyhq/node"

// importPath is the import specifier from one file to another, without the
// extension of a script.
func importPath(from, to string) string {
	fromParts := strings.Split(path.Dir(from), "/")
	if path.Dir(from) == "." {
		fromParts = nil
	}
	toParts := strings.Split(to, "/")
	i := 0
	for i < len(fromParts) && i < len(toParts)-1 && fromParts[i] == toParts[i] {
		i++
	}
	rel := strings.Repeat("../", len(fromParts)-i) + strings.Join(toParts[i:], "/")
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx"} {
		rel = strings.TrimSuffix(rel, ext)
	}
	if !strings.HasPrefix(rel, ".") {
		rel = "./" + rel
	}
	return rel
}

func join(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p != "" {
			kept = append(kept, p)
		}
	}
	return path.Join(kept...)
}

// Node is the plan for a Node application in contract mode.
func Node(a *detect.App, system string) Plan {
	ext := "js"
	if a.TypeScript {
		ext = "ts"
	}
	bang := ""
	if a.TypeScript {
		bang = "!"
	}
	cjs := !a.TypeScript && !a.ESM && a.Framework != "next" && a.Framework != "tanstack-start"
	var p Plan
	var missing []string
	for _, dep := range []string{NodePackage, "pg"} {
		if !a.Deps[dep] {
			missing = append(missing, dep)
		}
	}
	if len(missing) > 0 {
		add := a.PackageManager + " add"
		if a.PackageManager == "npm" || a.PackageManager == "" {
			add = "npm install"
		}
		p.Install = add + " " + strings.Join(missing, " ")
	}
	dev := a.PackageManager + " run dev"
	if a.PackageManager == "pnpm" || a.PackageManager == "yarn" || a.PackageManager == "bun" {
		dev = a.PackageManager + " dev"
	}

	switch a.Framework {
	case "next":
		authFile := join(a.Root, "lib", "auth."+ext)
		p.Files = append(p.Files, File{authFile, nodeAuth(system, "import graph from '"+importPath(authFile, "contracts/graph.json")+"'\n", true, false)})
		route := join(a.Root, "app/api/auth/[...cauthy]", "route."+ext)
		p.Files = append(p.Files, File{route, fmt.Sprintf("// Serves Cauthy's routes under /api/auth. Written by cauthy init.\nimport { cauthy } from '%s'\n\nexport const { GET, POST } = cauthy\n", importPath(route, authFile))})
		jwks := join(a.Root, "app/.well-known/pact-jwks.json", "route."+ext)
		p.Files = append(p.Files, File{jwks, fmt.Sprintf("// Publishes the public half of the application's key at the jwks_url of its\n// manifest. Written by cauthy init.\nimport { auth } from '%s'\n\nexport const GET = () => auth.pact%s.jwks()\n", importPath(jwks, authFile), bang)})
		name := "middleware"
		if a.NextMajor >= 16 {
			name = "proxy"
		}
		before := join(a.Root, name+"."+ext)
		p.Files = append(p.Files, File{before, fmt.Sprintf("// Keeps the session renewed. Written by cauthy init.\nimport { cauthy } from '%s'\n\nexport default cauthy.middleware()\n\nexport const config = { matcher: ['/((?!_next/|favicon.ico).*)'] }\n", importPath(before, authFile))})
		p.Steps = append(p.Steps, "Start the application ("+dev+") and open http://localhost:3000/api/auth/login")
	case "tanstack-start":
		authFile := join(a.Root, "auth."+ext)
		p.Files = append(p.Files, File{authFile, nodeAuth(system, "import graph from '"+importPath(authFile, "contracts/graph.json")+"'\n", false, false)})
		route := join(a.Root, "routes/api/auth", "$."+ext)
		p.Files = append(p.Files, File{route, fmt.Sprintf("// Serves Cauthy's routes under /api/auth. Written by cauthy init.\nimport { createFileRoute } from '@tanstack/react-router'\nimport { auth } from '%s'\n\nexport const Route = createFileRoute('/api/auth/$')({\n  server: {\n    handlers: {\n      GET: ({ request }) => auth.handler(request),\n      POST: ({ request }) => auth.handler(request),\n    },\n  },\n})\n", importPath(route, authFile))})
		jwks := join(a.Root, "routes/[.]well-known", "pact-jwks[.]json."+ext)
		p.Files = append(p.Files, File{jwks, fmt.Sprintf("// Publishes the public half of the application's key at the jwks_url of its\n// manifest. Written by cauthy init.\nimport { createFileRoute } from '@tanstack/react-router'\nimport { auth } from '%s'\n\nexport const Route = createFileRoute('/.well-known/pact-jwks.json')({\n  server: { handlers: { GET: () => auth.pact%s.jwks() } },\n})\n", importPath(jwks, authFile), bang)})
		p.Steps = append(p.Steps, "Start the application ("+dev+") and open http://localhost:3000/api/auth/login")
	default:
		authFile := join(a.Root, "auth."+ext)
		read := "import { readFileSync } from 'node:fs'\n"
		if cjs {
			read = "const { readFileSync } = require('node:fs')\n"
		}
		p.Files = append(p.Files, File{authFile, nodeAuth(system, read, false, cjs)})
		if a.Framework == "" {
			p.Steps = append(p.Steps, "Serve auth.handler(request) under /api/auth/* and auth.pact"+bang+".jwks() at /.well-known/pact-jwks.json (the @cauthyhq/node guide, \"Mount the route\").")
			break
		}
		p.Steps = append(p.Steps, "Add to your server, where the app is made:\n\n"+serverLines(a.Framework, cjs),
			"Start the server from the application's folder (it reads contracts/graph.json there) and open http://localhost:3000/api/auth/login")
	}
	return p
}

// nodeAuth is the file that makes the SDK in contract mode. graphLine either
// imports the graph (bundled frameworks) or imports readFileSync (servers).
func nodeAuth(system, graphLine string, next, cjs bool) string {
	var b strings.Builder
	b.WriteString(`// Sign-in with Cauthy by contract: Cauthy's address, the tenant and the
// clients come from contracts/graph.json (cauthy contracts pull); the only
// secret is PACT_ROOT_KEY, and the application's own key lives sealed in the
// cauthy_pact_keys table. Written by cauthy init.
`)
	if cjs {
		b.WriteString("const { createCauthy } = require('" + NodePackage + "')\nconst { postgresKeyStore } = require('" + NodePackage + "/keys')\nconst pg = require('pg')\n")
		b.WriteString(graphLine)
	} else {
		b.WriteString("import { createCauthy } from '" + NodePackage + "'\n")
		if next {
			b.WriteString("import { cauthyNext } from '" + NodePackage + "/next'\n")
		}
		b.WriteString("import { postgresKeyStore } from '" + NodePackage + "/keys'\nimport pg from 'pg'\n")
		b.WriteString(graphLine)
	}
	if strings.Contains(graphLine, "readFileSync") {
		b.WriteString("\n// Read from the working directory: start the server from the application's folder.\nconst graph = JSON.parse(readFileSync('contracts/graph.json', 'utf8'))\n")
	}
	b.WriteString("const pool = new pg.Pool({ connectionString: process.env.DATABASE_URL })\n\n")
	exp := "export const auth = "
	if cjs {
		exp = "exports.auth = "
	}
	fmt.Fprintf(&b, "%screateCauthy({\n  contract: { graph, system: '%s' },\n  keys: postgresKeyStore((text, params) => pool.query(text, params)),\n})\n", exp, system)
	if next {
		b.WriteString("export const cauthy = cauthyNext(auth)\n")
	}
	return b.String()
}

// serverLines mounts the adapter. The @cauthyhq/node 1.0 adapters serve the
// contract's JWKS path themselves, so no route is added for it.
func serverLines(framework string, cjs bool) string {
	imp := func(names, from string) string {
		if cjs {
			return fmt.Sprintf("  const { %s } = require('%s')\n", names, from)
		}
		return fmt.Sprintf("  import { %s } from '%s'\n", names, from)
	}
	switch framework {
	case "hono":
		return imp("cauthyHono", NodePackage+"/hono") + imp("auth", "./auth") + "  app.use(cauthyHono(auth))"
	case "express":
		return imp("cauthyExpress", NodePackage+"/express") + imp("auth", "./auth") + "  app.use(cauthyExpress(auth))"
	default: // fastify
		return imp("cauthyFastify", NodePackage+"/fastify") + imp("auth", "./auth") + "  await app.register(cauthyFastify, { auth })"
	}
}
