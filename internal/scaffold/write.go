// Package scaffold writes into the application what contract mode needs: the
// auth wiring for its framework, the key table's migration and the
// .env.example line. It never writes over a file without force.
package scaffold

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
)

// File is one file to lay down, its path relative to the application.
type File struct {
	Path    string
	Content string
}

// Plan is what init writes and what is left to a person.
type Plan struct {
	Files   []File
	Install string   // the command that adds the packages; "" when nothing is missing
	Steps   []string // what a person still does, in order
}

// Written is what happened to one file.
type Written struct {
	Path   string
	Action string // wrote, unchanged, kept, replaced
}

// Write lays the files down in dir. A file with the same content is
// unchanged; a different one is kept unless force.
func Write(dir string, files []File, force bool) ([]Written, error) {
	var done []Written
	for _, f := range files {
		full := filepath.Join(dir, filepath.FromSlash(f.Path))
		had, err := os.ReadFile(full)
		switch {
		case err == nil && bytes.Equal(had, []byte(f.Content)):
			done = append(done, Written{f.Path, "unchanged"})
			continue
		case err == nil && !force:
			done = append(done, Written{f.Path, "kept"})
			continue
		case err != nil && !os.IsNotExist(err):
			return done, err
		}
		action := "wrote"
		if err == nil {
			action = "replaced"
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return done, err
		}
		if err := os.WriteFile(full, []byte(f.Content), 0o644); err != nil {
			return done, err
		}
		done = append(done, Written{f.Path, action})
	}
	return done, nil
}

// ExampleFile is the committed file that lists the variables an application reads.
const ExampleFile = ".env.example"

// EnvExample adds the line PACT_ROOT_KEY= to ExampleFile, never a value. It
// answers what it did: added, present, or a warning when the file carries a value.
func EnvExample(dir string) (string, error) {
	path := filepath.Join(dir, ExampleFile)
	had, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	for _, line := range strings.Split(string(had), "\n") {
		line = strings.TrimSpace(line)
		if line == "PACT_ROOT_KEY=" {
			return "present", nil
		}
		if strings.HasPrefix(line, "PACT_ROOT_KEY=") {
			return "has a value: a root key never goes in a committed file; remove it there and rotate it (cauthy keys reseal --help)", nil
		}
	}
	text := string(had)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "# The only secret of contract mode: 32 random bytes (cauthy keys new-root). Set it in the secret manager, never here.\nPACT_ROOT_KEY=\n"
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return "added", nil
}
