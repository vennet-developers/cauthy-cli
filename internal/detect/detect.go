// Package detect reads which language and framework an application is made
// with, from its package.json or go.mod and the paths it has.
package detect

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// App is what init learns of an application.
type App struct {
	Lang      string // "node" or "go"
	Framework string // node: next, tanstack-start, hono, fastify, express; go: net/http, chi, gin, fiber, echo; "" unknown
	DevURL    string

	// Node
	Root           string // "" or "src": where the code lives
	TypeScript     bool
	ESM            bool
	NextMajor      int // 0 when the version cannot be read ("latest", a tag)
	NextMinor      int
	PackageManager string // npm, pnpm, yarn, bun
	Deps           map[string]bool

	// Go
	Module string
}

// FrameworkNames are the names a person reads.
var FrameworkNames = map[string]string{
	"next": "Next.js", "tanstack-start": "TanStack Start", "hono": "Hono", "fastify": "Fastify", "express": "Express",
	"net/http": "net/http", "chi": "chi", "gin": "Gin", "fiber": "Fiber", "echo": "Echo",
}

// Detect reads the application in dir. lang forces "node" or "go" when the
// folder has both; "" decides by what is there.
func Detect(dir, lang string) (*App, error) {
	hasPkg := exists(filepath.Join(dir, "package.json"))
	hasMod := exists(filepath.Join(dir, "go.mod"))
	switch {
	case lang != "" && lang != "node" && lang != "go":
		return nil, fmt.Errorf("--lang is node or go, not %q", lang)
	case lang == "node" || (lang == "" && hasPkg && !hasMod):
		if !hasPkg {
			return nil, errors.New("no package.json here")
		}
		return detectNode(dir)
	case lang == "go" || (lang == "" && hasMod && !hasPkg):
		if !hasMod {
			return nil, errors.New("no go.mod here")
		}
		return detectGo(dir)
	case hasPkg && hasMod:
		return nil, errors.New("this folder has both package.json and go.mod: pass --lang node or --lang go")
	}
	return nil, errors.New("no package.json or go.mod here: run cauthy init in the application's folder")
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

type packageJSON struct {
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func detectNode(dir string) (*App, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil, err
	}
	var pkg packageJSON
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil, fmt.Errorf("package.json: %w", err)
	}
	versions := map[string]string{}
	for k, v := range pkg.DevDependencies {
		versions[k] = v
	}
	for k, v := range pkg.Dependencies {
		versions[k] = v
	}
	deps := map[string]bool{}
	for k := range versions {
		deps[k] = true
	}
	a := &App{Lang: "node", DevURL: "http://localhost:3000", Deps: deps, ESM: pkg.Type == "module"}
	switch {
	case deps["next"]:
		a.Framework = "next"
	case deps["@tanstack/react-start"]:
		a.Framework = "tanstack-start"
	case deps["hono"]:
		a.Framework = "hono"
	case deps["fastify"]:
		a.Framework = "fastify"
	case deps["express"]:
		a.Framework = "express"
	}
	has := func(p string) bool { return exists(filepath.Join(dir, p)) }
	if has("src") {
		a.Root = "src"
	}
	switch a.Framework {
	case "next":
		a.Root = ""
		if has("src/app") {
			a.Root = "src"
		}
		if m := regexp.MustCompile(`(\d+)(?:\.(\d+))?`).FindStringSubmatch(versions["next"]); m != nil {
			a.NextMajor, _ = strconv.Atoi(m[1])
			a.NextMinor, _ = strconv.Atoi(m[2])
		}
	case "tanstack-start":
		a.Root = "src"
	}
	a.TypeScript = has("tsconfig.json") || deps["typescript"]
	switch {
	case has("pnpm-lock.yaml"):
		a.PackageManager = "pnpm"
	case has("yarn.lock"):
		a.PackageManager = "yarn"
	case has("bun.lockb") || has("bun.lock"):
		a.PackageManager = "bun"
	default:
		a.PackageManager = "npm"
	}
	return a, nil
}

func detectGo(dir string) (*App, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return nil, err
	}
	a := &App{Lang: "go", DevURL: "http://localhost:8080", Framework: "net/http"}
	requires := map[string]bool{}
	inBlock := false
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		switch {
		case len(f) == 0:
		case f[0] == "module" && len(f) > 1:
			a.Module = strings.Trim(f[1], `"`)
		case f[0] == "require" && len(f) > 1 && f[1] == "(":
			inBlock = true
		case f[0] == "require" && len(f) > 1:
			requires[f[1]] = true
		case inBlock && f[0] == ")":
			inBlock = false
		case inBlock:
			requires[f[0]] = true
		}
	}
	if a.Module == "" {
		return nil, errors.New("go.mod names no module")
	}
	for mod := range requires {
		switch {
		case strings.HasPrefix(mod, "github.com/go-chi/chi"):
			a.Framework = "chi"
		case strings.HasPrefix(mod, "github.com/gin-gonic/gin"):
			a.Framework = "gin"
		case strings.HasPrefix(mod, "github.com/gofiber/fiber"):
			a.Framework = "fiber"
		case strings.HasPrefix(mod, "github.com/labstack/echo"):
			a.Framework = "echo"
		}
	}
	return a, nil
}
