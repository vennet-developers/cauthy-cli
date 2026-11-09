package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeysNewRoot(t *testing.T) {
	env, out, errOut := testEnv(t, nil)
	if code := Run([]string{"keys", "new-root"}, env); code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	key := strings.TrimSpace(out.String())
	if len(key) != 43 || !strings.Contains(errOut.String(), "This is a new PACT_ROOT_KEY") || strings.Contains(errOut.String(), key) {
		t.Fatalf("stdout %q stderr %q", out, errOut)
	}
	if !strings.Contains(errOut.String(), deprecatedNewRoot) {
		t.Fatalf("no deprecation: %q", errOut)
	}

	env, out, errOut = testEnv(t, nil)
	if code := Run([]string{"keys", "new-root", "--out", "root.key"}, env); code != ExitOK {
		t.Fatalf("exit %d %s", code, errOut)
	}
	if !strings.Contains(errOut.String(), deprecatedNewRoot) {
		t.Fatalf("no deprecation: %q", errOut)
	}
	written, _ := os.ReadFile(filepath.Join(env.Dir, "root.key"))
	if len(strings.TrimSpace(string(written))) != 43 || strings.Contains(out.String()+errOut.String(), strings.TrimSpace(string(written))) {
		t.Fatalf("the written key was printed, or is wrong: %q / %q", out, errOut)
	}
	if code := Run([]string{"keys", "new-root", "--out", "root.key"}, env); code != ExitFail || !strings.Contains(errOut.String(), "never written over") {
		t.Fatalf("exit %d %s", code, errOut)
	}
}

func TestKeysRotateAndResealOnlyExplain(t *testing.T) {
	for _, sub := range []string{"rotate", "reseal"} {
		env, out, _ := testEnv(t, nil)
		if code := Run([]string{"keys", sub}, env); code != ExitOK || !strings.Contains(out.String(), "auth.pact") || !strings.Contains(out.String(), "by design") {
			t.Fatalf("%s: exit %d\n%s", sub, code, out)
		}
	}
	env, _, _ := testEnv(t, nil)
	for _, args := range [][]string{{"keys"}, {"keys", "burn"}, {"keys", "rotate", "now"}} {
		if code := Run(args, env); code != ExitUsage {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}

const deprecatedNewRoot = "cauthy: keys new-root is deprecated: CAUTHY_KEY replaces PACT_ROOT_KEY. Make the key on the application's card in Cauthy's dashboard (Integrate → Applications).\n"
