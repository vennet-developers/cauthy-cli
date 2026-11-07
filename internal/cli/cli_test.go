package cli

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// testEnv is an Env with buffers, the given environment, a fixed clock and no
// programs to run.
func testEnv(t *testing.T, vars map[string]string) (Env, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errOut bytes.Buffer
	return Env{
		Out: &out, Err: &errOut,
		Getenv: func(k string) string { return vars[k] },
		Dir:    t.TempDir(),
		HTTP:   &http.Client{Timeout: 5 * time.Second},
		Now:    func() time.Time { return time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC) },
		Rand:   bytes.NewReader(bytes.Repeat([]byte{7}, 4096)),
		Exec: func(dir, name string, args ...string) ([]byte, error) {
			return nil, errors.New("no programs in tests")
		},
	}, &out, &errOut
}

func TestHelpAndUnknownCommand(t *testing.T) {
	env, out, _ := testEnv(t, nil)
	if code := Run(nil, env); code != ExitOK || !strings.Contains(out.String(), "cauthy init") {
		t.Fatalf("no args: exit %d\n%s", code, out)
	}
	env, _, errOut := testEnv(t, nil)
	if code := Run([]string{"launch"}, env); code != ExitUsage || !strings.Contains(errOut.String(), `unknown command "launch"`) {
		t.Fatalf("unknown: exit %d\n%s", code, errOut)
	}
}

func TestVersion(t *testing.T) {
	env, out, _ := testEnv(t, nil)
	Version, Commit, Date = "1.2.3", "0123456789abcdef0123", "2026-11-07T12:00:00Z"
	t.Cleanup(func() { Version, Commit, Date = "dev", "", "" })
	if code := Run([]string{"version"}, env); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"cauthy 1.2.3 (0123456789ab, 2026-11-07T12:00:00Z)", "protocol auth-bff 1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	env, _, errOut := testEnv(t, nil)
	if code := Run([]string{"version", "extra"}, env); code != ExitUsage || !strings.Contains(errOut.String(), `unexpected argument "extra"`) {
		t.Fatalf("extra arg: exit %d\n%s", code, errOut)
	}
}

func TestSystemExecGivesUp(t *testing.T) {
	was := execTimeout
	execTimeout = time.Nanosecond
	t.Cleanup(func() { execTimeout = was })
	_, err := System().Exec(t.TempDir(), "go", "version")
	if err == nil || !strings.Contains(err.Error(), "did not finish within") {
		t.Fatalf("got %v", err)
	}
}

func TestVersionWithoutADate(t *testing.T) {
	env, out, _ := testEnv(t, nil)
	Version, Commit, Date = "1.2.3", "0123456789abcdef0123", ""
	t.Cleanup(func() { Version, Commit, Date = "dev", "", "" })
	Run([]string{"version"}, env)
	if !strings.HasPrefix(out.String(), "cauthy 1.2.3 (0123456789ab)\n") {
		t.Fatalf("%q", out)
	}
}

func TestErrorsCarryNoControlCharacterAndNoPrefix(t *testing.T) {
	env, _, errOut := testEnv(t, nil)
	if code := fail(env, errors.New("bad\x1b]0;owned\x07 thing")); code != ExitFail || errOut.String() != "cauthy: bad]0;owned thing\n" {
		t.Fatalf("%d %q", code, errOut)
	}
	errOut.Reset()
	if code := fail(env, usagef("give --%s", "system")); code != ExitUsage || errOut.String() != "cauthy: give --system\n" {
		t.Fatalf("%d %q", code, errOut)
	}
	var u *usageError
	if !errors.As(fmt.Errorf("wrapped: %w", usagef("x")), &u) || u.msg != "x" {
		t.Fatal("a usage error is a *usageError, also wrapped")
	}
}
