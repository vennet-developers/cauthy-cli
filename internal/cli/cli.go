// Package cli is the cauthy command line. Every command reads and writes
// through Env, so tests drive it without a shell, a network or a clock.
package cli

import (
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vennet-developers/cauthy-cli/internal/pact"
	"github.com/vennet-developers/cauthy-cli/internal/pull"
)

// Exit codes, the same as the conformance runner's.
const (
	ExitOK         = 0 // done; every check passed
	ExitFail       = 1 // a check failed, or the application would refuse to start
	ExitUsage      = 2 // bad flags, or a file the command needs cannot be read
	ExitIncomplete = 3 // nothing failed, but a check that was asked for did not run
)

// Env is everything a command touches outside itself.
type Env struct {
	Ctx      context.Context // nil means context.Background()
	Out, Err io.Writer
	Getenv   func(string) string
	Dir      string // relative paths are read against it
	HTTP     *http.Client
	Now      func() time.Time
	Rand     io.Reader
	// Exec runs a program in dir and answers its standard output.
	Exec func(dir, name string, args ...string) ([]byte, error)
}

// execTimeout bounds every program a command runs (git, node).
var execTimeout = 2 * time.Minute

// System is the Env of a real run.
func System() Env {
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	return Env{
		Out: os.Stdout, Err: os.Stderr, Getenv: os.Getenv, Dir: dir,
		HTTP: &http.Client{Timeout: 15 * time.Second},
		Now:  time.Now, Rand: rand.Reader,
		Exec: func(dir, name string, args ...string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Dir = dir
			cmd.WaitDelay = 5 * time.Second
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if ctx.Err() != nil {
				return out, fmt.Errorf("%s %s did not finish within %s", name, strings.Join(args, " "), execTimeout)
			}
			if err != nil {
				return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
			}
			return out, nil
		},
	}
}

const help = `cauthy connects an application to Cauthy.

  cauthy init                 detect the application and write what it needs
  cauthy contracts pull       vendor the Pact graph into one or more folders
  cauthy contracts check      say whether vendored copies are identical
  cauthy pact check           what the contract says, and whether the app would start
  cauthy key inspect          what the application's CAUTHY_KEY names (--check asks Cauthy)
  cauthy keys new-root        make a PACT_ROOT_KEY
  cauthy keys rotate|reseal   how to rotate the application's key (from the app)
  cauthy conformance          certify a backend against the auth-bff protocol
  cauthy version              this build, and the protocol it checks

Run cauthy <command> --help for its flags.

Exit codes: 0 ok; 1 a check failed; 2 usage or configuration error;
3 nothing failed, but a check that was asked for did not run.
`

type command func(args []string, env Env) int

// commands is filled by each command's file (register in its init).
var commands = map[string]command{}

func register(name string, c command) { commands[name] = c }

// Run runs one command line and answers its exit code.
func Run(args []string, env Env) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(env.Out, help)
		return ExitOK
	}
	cmd, ok := commands[args[0]]
	if !ok {
		fmt.Fprintf(env.Err, "cauthy: unknown command %q\n\n%s", args[0], help)
		return ExitUsage
	}
	return cmd(args[1:], env)
}

// usageError is an error the person fixes by changing the command line.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return &usageError{fmt.Sprintf(format, a...)} }

// fail prints an error and answers its exit code: 2 for a usage error or an
// input that cannot be read, else 1.
func fail(env Env, err error) int {
	fmt.Fprintf(env.Err, "cauthy: %s\n", pact.Clean(err.Error()))
	var usage *usageError
	var input *pull.InputError
	if errors.As(err, &usage) || errors.As(err, &input) {
		return ExitUsage
	}
	return ExitFail
}

// parse parses a command's flags. It answers done=true with the exit code when
// the command must stop: --help (0) or a bad flag or argument (2).
func parse(fs *flag.FlagSet, args []string, env Env, usage string) (code int, done bool) {
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(env.Out, usage)
			return ExitOK, true
		}
		fmt.Fprintf(env.Err, "cauthy: %v\n\n%s", err, usage)
		return ExitUsage, true
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(env.Err, "cauthy: unexpected argument %q\n\n%s", fs.Arg(0), usage)
		return ExitUsage, true
	}
	return 0, false
}

// multi is a flag that may be repeated: --to a --to b.
type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

// context is the run's context: done when the person interrupts the run.
func (env Env) context() context.Context {
	if env.Ctx == nil {
		return context.Background()
	}
	return env.Ctx
}

// abs reads a path against the Env's folder.
func (env Env) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(env.Dir, p)
}
