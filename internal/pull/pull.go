// Package pull vendors the Pact graph into an application: graph.json and
// VERSION, the same bytes in every destination.
package pull

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/vennet-developers/cauthy-cli/internal/address"
	"github.com/vennet-developers/cauthy-cli/internal/graph"
)

// MaxGraph is the largest graph.json accepted from an address.
const MaxGraph = 16 << 20

// Deps is what pulling touches outside itself.
type Deps struct {
	Dir  string // relative paths are read against it
	HTTP *http.Client
	Exec func(dir, name string, args ...string) ([]byte, error)
}

// Source is a graph read from core-contracts.
type Source struct {
	Raw      []byte // graph.json, ending in a newline
	Version  string // the commit, or sha256:<hex> for a graph read from an address
	Systems  int
	Links    int
	Warnings []string // what a person should know, such as why it is unversioned
}

// InputError is an input that cannot be read: a source or a vendored copy
// that is missing or is not a graph. The CLI answers it with exit code 2.
type InputError struct{ Err error }

func (e *InputError) Error() string { return e.Err.Error() }
func (e *InputError) Unwrap() error { return e.Err }

func inputf(format string, a ...any) error { return &InputError{fmt.Errorf(format, a...)} }

func (d Deps) abs(p string) string {
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Join(d.Dir, p)
}

// IsAddress says whether from is an address rather than a folder.
func IsAddress(from string) bool { return strings.Contains(from, "://") }

// Fetch reads the graph from a core-contracts checkout or an https address.
// A checkout with uncommitted changes is refused unless allowDirty: the graph
// an application vendors is the one of a commit.
func Fetch(d Deps, from string, allowDirty bool) (*Source, error) {
	if IsAddress(from) {
		return fetchURL(d, from)
	}
	return fetchDir(d, d.abs(from), allowDirty)
}

// sameFolder says whether a and b name one folder, through links and the
// file system's case rules.
func sameFolder(a, b string) bool {
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(sa, sb)
}

// version reads the commit of the checkout at dir. A folder that is not a
// checkout's root (not one, or a folder inside another), or a checkout with no
// commit, is unversioned, with the reason as a warning.
func version(d Deps, dir string, allowDirty bool) (string, []string, error) {
	top, err := d.Exec(dir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "unversioned", []string{dir + " is not a git checkout: VERSION says unversioned"}, nil
	}
	if root := strings.TrimSpace(string(top)); !sameFolder(filepath.FromSlash(root), dir) {
		return "unversioned", []string{fmt.Sprintf("%s is inside the git checkout %s, not its root: VERSION says unversioned", dir, root)}, nil
	}
	head, err := d.Exec(dir, "git", "rev-parse", "HEAD")
	if err != nil {
		return "unversioned", []string{dir + " has no commit yet: VERSION says unversioned"}, nil
	}
	v := strings.TrimSpace(string(head))
	status, err := d.Exec(dir, "git", "status", "--porcelain")
	if err != nil {
		return "", nil, err
	}
	if len(bytes.TrimSpace(status)) > 0 {
		if !allowDirty {
			return "", nil, fmt.Errorf("%s has uncommitted changes: the graph must come from a commit (commit them, or pass --allow-dirty to try a draft)", dir)
		}
		v += "-dirty"
	}
	return v, nil, nil
}

func fetchDir(d Deps, dir string, allowDirty bool) (*Source, error) {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil, inputf("no contracts repository at %s (give --from, or set CONTRACTS_DIR)", dir)
	}
	v, warnings, err := version(d, dir, allowDirty)
	if err != nil {
		return nil, err
	}
	var raw []byte
	if _, err := os.Stat(filepath.Join(dir, "scripts", "graph.mjs")); err == nil {
		// The graph is generated from the contracts as they are, into a
		// temporary file: the checkout is never written to.
		tmp, err := os.MkdirTemp("", "cauthy-graph-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmp)
		out := filepath.Join(tmp, "graph.json")
		if _, err := d.Exec(dir, "node", "scripts/graph.mjs", out); err != nil {
			return nil, fmt.Errorf("generating the graph in %s: %w", dir, err)
		}
		if raw, err = os.ReadFile(out); err != nil {
			return nil, err
		}
	} else {
		var err error
		if raw, err = os.ReadFile(filepath.Join(dir, "graph.json")); err != nil {
			return nil, inputf("%s has neither scripts/graph.mjs nor graph.json", dir)
		}
	}
	s, err := source(raw, v)
	if err != nil {
		return nil, err
	}
	s.Warnings = warnings
	return s, nil
}

func fetchURL(d Deps, from string) (*Source, error) {
	u, err := graph.HTTPSAddress(from, "graph address")
	if err != nil {
		return nil, &InputError{err}
	}
	shown := address.Redact(from) // a query may carry a token: never shown
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &InputError{err}
	}
	req.Header.Set("Accept", "application/json")
	res, err := address.Client(d.HTTP).Do(req)
	if err != nil {
		return nil, inputf("%s: %v", shown, errors.Unwrap(err))
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, inputf("%s answered %d", shown, res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, MaxGraph+1))
	if err != nil {
		return nil, inputf("%s: %v", shown, err)
	}
	if len(raw) > MaxGraph {
		return nil, inputf("%s answered more than %d bytes: not a graph", shown, MaxGraph)
	}
	sum := sha256.Sum256(raw)
	return source(raw, "sha256:"+hex.EncodeToString(sum[:]))
}

func source(raw []byte, version string) (*Source, error) {
	g, err := graph.Parse(raw)
	if err != nil {
		return nil, &InputError{err}
	}
	if !bytes.HasSuffix(raw, []byte("\n")) {
		raw = append(raw, '\n')
	}
	s, l := g.Counts()
	return &Source{Raw: raw, Version: version, Systems: s, Links: l}, nil
}

// Write puts graph.json and VERSION in every destination, then checks every
// copy is identical. It answers the line to print. Every file is first
// written and synced beside its place, and only then are all of them moved
// into place, so a destination that cannot be written leaves every folder as
// it was.
func Write(d Deps, src *Source, dests []string) (string, error) {
	if len(dests) == 0 {
		return "", errors.New("give at least one --to")
	}
	var folders []string
	for i, dest := range dests {
		p := d.abs(dest)
		if err := os.MkdirAll(p, 0o755); err != nil {
			return "", err
		}
		for j, other := range folders {
			if sameFolder(p, other) {
				return "", fmt.Errorf("--to %s is given twice: it is the same folder as --to %s", dests[i], dests[j])
			}
		}
		folders = append(folders, p)
	}

	type move struct{ tmp, final string }
	var moves []move
	cleanup := func() {
		for _, m := range moves {
			os.Remove(m.tmp)
		}
	}
	for _, p := range folders {
		for name, data := range map[string][]byte{"graph.json": src.Raw, "VERSION": []byte(src.Version + "\n")} {
			tmp, err := syncedTemp(p, data)
			if err != nil {
				cleanup()
				return "", err
			}
			moves = append(moves, move{tmp, filepath.Join(p, name)})
		}
	}
	for i, m := range moves {
		if err := os.Rename(m.tmp, m.final); err != nil {
			for _, rest := range moves[i:] {
				os.Remove(rest.tmp)
			}
			return "", err
		}
	}
	if err := Check(d, dests); err != nil {
		return "", err
	}
	return fmt.Sprintf("graph %s → %s (%d systems, %d links); every copy identical", Short(src.Version), strings.Join(dests, ", "), src.Systems, src.Links), nil
}

// syncedTemp writes data to a new file in dir, synced to disk, and answers its path.
func syncedTemp(dir string, data []byte) (string, error) {
	tmp, err := os.CreateTemp(dir, ".cauthy-*")
	if err != nil {
		return "", err
	}
	fail := func(err error) (string, error) {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

// Short is a version as a person reads it: a commit's first 8 characters.
func Short(version string) string {
	if strings.HasPrefix(version, "sha256:") && len(version) > 19 {
		return version[:19]
	}
	head, suffix, dirty := strings.Cut(version, "-")
	if len(head) == 40 {
		head = head[:8]
	}
	if dirty {
		return head + "-" + suffix
	}
	return head
}

// Check says whether every destination holds the same graph.json and VERSION.
// A copy that is missing or is not a graph is an *InputError.
func Check(d Deps, dests []string) error {
	if len(dests) == 0 {
		return errors.New("give at least one --to")
	}
	var first, firstDest string
	for _, dest := range dests {
		p := d.abs(dest)
		raw, err := os.ReadFile(filepath.Join(p, "graph.json"))
		if err != nil {
			return inputf("%s has no graph.json: run cauthy contracts pull", dest)
		}
		if _, err := graph.Parse(raw); err != nil {
			return inputf("%s/graph.json: %v", dest, err)
		}
		version, err := os.ReadFile(filepath.Join(p, "VERSION"))
		if err != nil {
			return inputf("%s has no VERSION: run cauthy contracts pull", dest)
		}
		sum := sha256.Sum256(append(append(raw, 0), version...))
		h := hex.EncodeToString(sum[:])
		if first == "" {
			first, firstDest = h, dest
			continue
		}
		if h != first {
			return fmt.Errorf("%s and %s hold different graphs: pull again into both", firstDest, dest)
		}
	}
	return nil
}
