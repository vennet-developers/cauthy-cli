// Package scaffold writes into the application what CAUTHY_KEY needs: the
// auth wiring for its framework and the .env.example line. It never writes
// over a file without force.
package scaffold

import (
	"bytes"
	"fmt"
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
// unchanged; a different one is kept unless force. A symbolic link is never
// followed, and a new file is created exclusively, so nothing is written
// outside the application or over a file made meanwhile.
func Write(dir string, files []File, force bool) ([]Written, error) {
	var done []Written
	for _, f := range files {
		full := filepath.Join(dir, filepath.FromSlash(f.Path))
		st, err := os.Lstat(full)
		switch {
		case err == nil && st.Mode()&os.ModeSymlink != 0:
			return done, fmt.Errorf("%s is a symbolic link: init never writes through one; replace it with a file", f.Path)
		case err == nil && !st.Mode().IsRegular():
			return done, fmt.Errorf("%s is not a regular file", f.Path)
		case err != nil && !os.IsNotExist(err):
			return done, err
		}
		if err == nil {
			had, err := os.ReadFile(full)
			if err != nil {
				return done, err
			}
			if bytes.Equal(had, []byte(f.Content)) {
				done = append(done, Written{f.Path, "unchanged"})
				continue
			}
			if !force {
				done = append(done, Written{f.Path, "kept"})
				continue
			}
			if err := os.WriteFile(full, []byte(f.Content), 0o644); err != nil {
				return done, err
			}
			done = append(done, Written{f.Path, "replaced"})
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return done, err
		}
		if err := CreateNew(full, []byte(f.Content)); err != nil {
			return done, err
		}
		done = append(done, Written{f.Path, "wrote"})
	}
	return done, nil
}

// CreateNew makes a new file, refusing one that already exists (or a link in its place).
func CreateNew(path string, data []byte) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := out.Write(data); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ExampleFile is the committed file that lists the variables an application reads.
const ExampleFile = ".env.example"

// keyComment and keyLine are what EnvExample adds.
const (
	keyComment = "# The application's only setting: the key Cauthy's dashboard made for it (Integrate → Applications). Set it in the secret manager, never here."
	keyLine    = "CAUTHY_KEY="
)

// EnvExample adds the line CAUTHY_KEY= to ExampleFile, never a value. It
// answers what it did: added, present, or a warning when the file carries a
// key.
func EnvExample(dir string) (string, error) {
	path := filepath.Join(dir, ExampleFile)
	if st, err := os.Lstat(path); err == nil && st.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%s is a symbolic link: init never writes through one; replace it with a file", ExampleFile)
	}
	had, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	missing := err != nil
	text := string(had)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	var lines []string
	present, changed := false, false
	for _, line := range strings.Split(strings.TrimSuffix(text, "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == keyLine:
			present = true
		case strings.HasPrefix(trimmed, keyLine):
			return "has a value: a CAUTHY_KEY never goes in a committed file; remove it there and make a new key on the application's card", nil
		}
		if text != "" {
			lines = append(lines, line)
		}
	}
	said := "present"
	if !present {
		lines = append(lines, keyComment, keyLine)
		said, changed = "added", true
	}
	if !changed {
		return said, nil
	}
	out := []byte(strings.Join(lines, "\n") + "\n")
	if missing {
		if err := CreateNew(path, out); err != nil {
			return "", err
		}
		return said, nil
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", err
	}
	return said, nil
}
