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
func NodeKey(a *detect.App) Plan {
	ext := "js"
	if a.TypeScript {
		ext = "ts"
	}
	cjs := !a.TypeScript && !a.ESM && a.Framework != "next" && a.Framework != "tanstack-start"
	var p Plan
	if !a.Deps[NodePackage] {
		add := a.PackageManager + " add"
		if a.PackageManager == "npm" || a.PackageManager == "" {
			add = "npm install"
		}
		p.Install = add + " " + NodePackage
	}
	dev := a.PackageManager + " run dev"
	if a.PackageManager == "pnpm" || a.PackageManager == "yarn" || a.PackageManager == "bun" {
		dev = a.PackageManager + " dev"
	}

	switch a.Framework {
	case "next":
		authFile := join(a.Root, "lib", "auth."+ext)
		p.Files = append(p.Files, File{authFile, nodeAuth(true, false)})
		route := join(a.Root, "app/api/auth/[...cauthy]", "route."+ext)
		p.Files = append(p.Files, File{route, fmt.Sprintf("// Serves Cauthy's routes under /api/auth. Written by cauthy init.\nimport { cauthy } from '%s'\n\nexport const { GET, POST } = cauthy\n", importPath(route, authFile))})
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
		p.Files = append(p.Files, File{authFile, nodeAuth(false, false)})
		route := join(a.Root, "routes/api/auth", "$."+ext)
		p.Files = append(p.Files, File{route, fmt.Sprintf("// Serves Cauthy's routes under /api/auth. Written by cauthy init.\nimport { createFileRoute } from '@tanstack/react-router'\nimport { auth } from '%s'\n\nexport const Route = createFileRoute('/api/auth/$')({\n  server: {\n    handlers: {\n      GET: ({ request }) => auth.handler(request),\n      POST: ({ request }) => auth.handler(request),\n    },\n  },\n})\n", importPath(route, authFile))})
		p.Steps = append(p.Steps, "Start the application ("+dev+") and open http://localhost:3000/api/auth/login")
	default:
		authFile := join(a.Root, "auth."+ext)
		p.Files = append(p.Files, File{authFile, nodeAuth(false, cjs)})
		if a.Framework == "" {
			p.Steps = append(p.Steps, "Serve auth.handler(request) under /api/auth/* (the @cauthyhq/node guide, \"Mount the route\").")
			break
		}
		p.Steps = append(p.Steps, "Add to your server, where the app is made:\n\n"+serverLines(a.Framework, cjs),
			"Start the server and open http://localhost:3000/api/auth/login")
	}
	return p
}

// nodeAuth is the file that makes the SDK on CAUTHY_KEY: no key store, no
// database.
func nodeAuth(next, cjs bool) string {
	var b strings.Builder
	if cjs {
		b.WriteString("const { createCauthy } = require('" + NodePackage + "')\n")
	} else {
		b.WriteString("import { createCauthy } from '" + NodePackage + "'\n")
		if next {
			b.WriteString("import { cauthyNext } from '" + NodePackage + "/next'\n")
		}
	}
	exp := "export const auth = "
	if cjs {
		exp = "exports.auth = "
	}
	fmt.Fprintf(&b, "\n// Reads CAUTHY_KEY from the environment.\n%screateCauthy()\n", exp)
	if next {
		b.WriteString("export const cauthy = cauthyNext(auth)\n")
	}
	return b.String()
}

// serverLines mounts the adapter.
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
