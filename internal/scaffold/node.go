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

// Supported refuses what init cannot wire: Next.js before 15.5, whose
// middleware cannot run on Node.js (the SDK needs it). A Next version init
// cannot read is taken as the current one.
func Supported(a *detect.App) error {
	if a.Framework == "next" && a.NextMajor > 0 && (a.NextMajor < 15 || a.NextMajor == 15 && a.NextMinor < 5) {
		return fmt.Errorf("Next.js 15.5 or newer is required (found %d.%d): before it the middleware runs only on the Edge runtime, where @cauthyhq/node cannot. Upgrade Next.js, then run cauthy init again", a.NextMajor, a.NextMinor)
	}
	return nil
}

// NodeKey is the plan for a Node application on CAUTHY_KEY alone, as the
// five-minute guide wires it: createCauthy() and the route, nothing else.
func NodeKey(a *detect.App) Plan { return node(a, "") }

// Node is the plan for a Node application in contract mode.
//
// Contracts are retired: init writes it only with the deprecated --contracts.
func Node(a *detect.App, system string) Plan { return node(a, system) }

// node is the plan by contract for system, or on CAUTHY_KEY alone when
// system is "".
func node(a *detect.App, system string) Plan {
	contract := system != ""
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
	for _, dep := range []string{NodePackage} {
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
		p.Files = append(p.Files, File{authFile, nodeAuth(system, graphImport(contract, authFile), true, false)})
		route := join(a.Root, "app/api/auth/[...cauthy]", "route."+ext)
		p.Files = append(p.Files, File{route, fmt.Sprintf("// Serves Cauthy's routes under /api/auth. Written by cauthy init.\nimport { cauthy } from '%s'\n\nexport const { GET, POST } = cauthy\n", importPath(route, authFile))})
		if jwks := join(a.Root, "app/.well-known/pact-jwks.json", "route."+ext); contract {
			p.Files = append(p.Files, File{jwks, fmt.Sprintf("// Publishes the public half of the application's key at the jwks_url of its\n// manifest. Written by cauthy init.\nimport { auth } from '%s'\n\nexport const GET = () => auth.pact%s.jwks()\n", importPath(jwks, authFile), bang)})
		}
		// Next 16's proxy runs on Node.js; 15.5's middleware must say so, or
		// it runs on the Edge runtime, where the SDK cannot.
		name, runtime := "proxy", ""
		if a.NextMajor > 0 && a.NextMajor < 16 {
			name, runtime = "middleware", "runtime: 'nodejs', "
		}
		before := join(a.Root, name+"."+ext)
		p.Files = append(p.Files, File{before, fmt.Sprintf("// Keeps the session renewed. Written by cauthy init.\nimport { cauthy } from '%s'\n\nexport default cauthy.middleware()\n\nexport const config = { %smatcher: ['/((?!_next/|favicon.ico).*)'] }\n", importPath(before, authFile), runtime)})
		p.Steps = append(p.Steps, "Start the application ("+dev+") and open http://localhost:3000/api/auth/login")
	case "tanstack-start":
		authFile := join(a.Root, "auth."+ext)
		p.Files = append(p.Files, File{authFile, nodeAuth(system, graphImport(contract, authFile), false, false)})
		route := join(a.Root, "routes/api/auth", "$."+ext)
		p.Files = append(p.Files, File{route, fmt.Sprintf("// Serves Cauthy's routes under /api/auth. Written by cauthy init.\nimport { createFileRoute } from '@tanstack/react-router'\nimport { auth } from '%s'\n\nexport const Route = createFileRoute('/api/auth/$')({\n  server: {\n    handlers: {\n      GET: ({ request }) => auth.handler(request),\n      POST: ({ request }) => auth.handler(request),\n    },\n  },\n})\n", importPath(route, authFile))})
		if jwks := join(a.Root, "routes/[.]well-known", "pact-jwks[.]json."+ext); contract {
			p.Files = append(p.Files, File{jwks, fmt.Sprintf("// Publishes the public half of the application's key at the jwks_url of its\n// manifest. Written by cauthy init.\nimport { createFileRoute } from '@tanstack/react-router'\nimport { auth } from '%s'\n\nexport const Route = createFileRoute('/.well-known/pact-jwks.json')({\n  server: { handlers: { GET: () => auth.pact%s.jwks() } },\n})\n", importPath(jwks, authFile), bang)})
		}
		p.Steps = append(p.Steps, "Start the application ("+dev+") and open http://localhost:3000/api/auth/login")
	default:
		authFile := join(a.Root, "auth."+ext)
		read := ""
		if contract {
			read = "import { readFileSync } from 'node:fs'\n"
			if cjs {
				read = "const { readFileSync } = require('node:fs')\n"
			}
		}
		p.Files = append(p.Files, File{authFile, nodeAuth(system, read, false, cjs)})
		if a.Framework == "" {
			if contract {
				p.Steps = append(p.Steps, "Serve auth.handler(request) under /api/auth/* and auth.pact"+bang+".jwks() at /.well-known/pact-jwks.json (the @cauthyhq/node guide, \"Mount the route\").")
			} else {
				p.Steps = append(p.Steps, "Serve auth.handler(request) under /api/auth/* (the @cauthyhq/node guide, \"Mount the route\").")
			}
			break
		}
		start := "Start the server and open http://localhost:3000/api/auth/login"
		if contract {
			start = "Start the server from the application's folder (it reads contracts/graph.json there) and open http://localhost:3000/api/auth/login"
		}
		p.Steps = append(p.Steps, "Add to your server, where the app is made:\n\n"+serverLines(a.Framework, cjs), start)
	}
	return p
}

// graphImport is the line that imports the graph by contract, in a bundled
// framework; "" on CAUTHY_KEY alone.
func graphImport(contract bool, authFile string) string {
	if !contract {
		return ""
	}
	return "import graph from '" + importPath(authFile, "contracts/graph.json") + "'\n"
}

// nodeAuth is the file that makes the SDK on CAUTHY_KEY: no key store, no
// database. By contract (system not "") graphLine either imports the graph
// (bundled frameworks) or imports readFileSync (servers).
func nodeAuth(system, graphLine string, next, cjs bool) string {
	var b strings.Builder
	b.WriteString(graphLine)
	if cjs {
		b.WriteString("const { createCauthy } = require('" + NodePackage + "')\n")
	} else {
		b.WriteString("import { createCauthy } from '" + NodePackage + "'\n")
		if next {
			b.WriteString("import { cauthyNext } from '" + NodePackage + "/next'\n")
		}
	}
	if strings.Contains(graphLine, "readFileSync") {
		b.WriteString("\n// Read from the working directory: start the server from the application's folder.\nconst graph = JSON.parse(readFileSync('contracts/graph.json', 'utf8'))\n")
	}
	exp := "export const auth = "
	if cjs {
		exp = "exports.auth = "
	}
	if system == "" {
		fmt.Fprintf(&b, "\n// Reads CAUTHY_KEY from the environment.\n%screateCauthy()\n", exp)
	} else {
		fmt.Fprintf(&b, "\n// Reads CAUTHY_KEY; the contract adds the system proof and its policies.\n%screateCauthy({ contract: { graph, system: '%s' } })\n", exp, system)
	}
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
