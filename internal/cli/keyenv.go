package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
)

// keyFiles are the project files a key is looked for in, in this order, when
// the environment has none.
var keyFiles = []string{".env", ".env.local"}

// keyError is a refused key: the reason and where the key came from, never
// the key.
type keyError struct {
	from string
	err  *cauthykey.Error
}

func (e *keyError) Error() string {
	return fmt.Sprintf("%s (%s, from %s)", e.err.Msg, e.err.Reason, e.from)
}
func (e *keyError) Unwrap() error { return e.err }

// readKey reads the application's CAUTHY_KEY (or the variable name) and
// answers it with where it came from: "environment", ".env" or
// ".env.local". The environment wins; then the first of keyFiles in dir that
// sets it. Only that one line of a file is ever kept: no other value of the
// project's .env reaches the CLI's output.
func readKey(env *Env, dir, name string) (*cauthykey.Key, string, error) {
	value, from := "", ""
	if v := env.Getenv(name); v != "" {
		value, from = v, "environment"
	} else {
		for _, f := range keyFiles {
			v, ok, err := keyLine(filepath.Join(dir, f), name)
			if err != nil {
				return nil, "", err
			}
			if ok {
				value, from = v, f
				break
			}
		}
	}
	if from == "" {
		return nil, "", usagef("No %s: connect the application in Cauthy's dashboard (Integrate → Applications) and put the line in .env", name)
	}
	k, err := cauthykey.Parse(value)
	if err != nil {
		var ke *cauthykey.Error
		if errors.As(err, &ke) {
			return nil, from, &keyError{from: from, err: ke}
		}
		return nil, from, fmt.Errorf("%s from %s cannot be read", name, from)
	}
	return k, from, nil
}

// keyLine answers the value of name in a dotenv file: `NAME=value`,
// `export NAME=value`, with one pair of matching quotes stripped. A missing
// file has none.
func keyLine(path, name string) (string, bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("cannot read %s", filepath.Base(path))
	}
	defer f.Close()
	line := regexp.MustCompile(`^(?:export\s+)?` + regexp.QuoteMeta(name) + `\s*=\s*(.*)$`)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		m := line.FindStringSubmatch(strings.TrimSpace(sc.Text()))
		if m == nil {
			continue
		}
		return unquote(strings.TrimSpace(m[1])), true, nil
	}
	if sc.Err() != nil {
		return "", false, fmt.Errorf("cannot read %s", filepath.Base(path))
	}
	return "", false, nil
}

// unquote strips one pair of matching quotes.
func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}
