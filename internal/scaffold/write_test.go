package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteNeverOverwritesWithoutForce(t *testing.T) {
	dir := t.TempDir()
	files := []File{{"lib/auth.ts", "one\n"}}
	done, err := Write(dir, files, false)
	if err != nil || done[0].Action != "wrote" {
		t.Fatalf("%v %+v", err, done)
	}
	if done, _ := Write(dir, files, false); done[0].Action != "unchanged" {
		t.Fatalf("%+v", done)
	}
	changed := []File{{"lib/auth.ts", "two\n"}}
	if done, _ := Write(dir, changed, false); done[0].Action != "kept" {
		t.Fatalf("%+v", done)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "lib", "auth.ts"))
	if string(raw) != "one\n" {
		t.Fatalf("overwritten without force: %q", raw)
	}
	if done, _ := Write(dir, changed, true); done[0].Action != "replaced" {
		t.Fatalf("%+v", done)
	}
}

func TestEnvExampleAddsTheNameNeverAValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ExampleFile)
	if err := os.WriteFile(path, []byte("DATABASE_URL="), 0o644); err != nil {
		t.Fatal(err)
	}
	if said, err := EnvExample(dir); err != nil || said != "added" {
		t.Fatalf("%q %v", said, err)
	}
	if said, _ := EnvExample(dir); said != "present" {
		t.Fatalf("second run: %q", said)
	}
	raw, _ := os.ReadFile(path)
	want := "DATABASE_URL=\n" + keyLines
	if string(raw) != want {
		t.Fatalf("%q\nwant %q", raw, want)
	}
	if err := os.WriteFile(path, []byte("CAUTHY_KEY=cauthy1.abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if said, _ := EnvExample(dir); !strings.HasPrefix(said, "has a value") {
		t.Fatalf("%q", said)
	}
}

const keyLines = "# The application's only setting: the key Cauthy's dashboard made for it (Integrate → Applications). Set it in the secret manager, never here.\nCAUTHY_KEY=\n"

// A file from contract mode before CAUTHY_KEY: PACT_ROOT_KEY is said to be
// deprecated, once.
func TestEnvExampleDeprecatesTheRootKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ExampleFile)
	if err := os.WriteFile(path, []byte("# The only secret of contract mode.\nPACT_ROOT_KEY=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := EnvExample(dir); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(path)
	want := "# The only secret of contract mode.\n# PACT_ROOT_KEY: deprecated, replaced by CAUTHY_KEY (remove after moving).\nPACT_ROOT_KEY=\n" + keyLines
	if string(raw) != want {
		t.Fatalf("%q\nwant %q", raw, want)
	}
}

func TestWritesNeverFollowALink(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(outside, []byte("theirs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "lib", "auth.ts")); err != nil {
		t.Skip("no symbolic links here:", err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ExampleFile)); err != nil {
		t.Fatal(err)
	}
	for _, force := range []bool{false, true} {
		if _, err := Write(dir, []File{{"lib/auth.ts", "mine\n"}}, force); err == nil || !strings.Contains(err.Error(), "symbolic link") {
			t.Fatalf("force %v: %v", force, err)
		}
	}
	if _, err := EnvExample(dir); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("%v", err)
	}
	if raw, _ := os.ReadFile(outside); string(raw) != "theirs\n" {
		t.Fatalf("a link was followed: %q", raw)
	}
}

func TestEnvExampleNewAndEmpty(t *testing.T) {
	dir := t.TempDir()
	if said, err := EnvExample(dir); err != nil || said != "added" {
		t.Fatalf("no file: %q %v", said, err)
	}
	empty := t.TempDir()
	if err := os.WriteFile(filepath.Join(empty, ExampleFile), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if said, err := EnvExample(empty); err != nil || said != "added" {
		t.Fatalf("an empty file: %q %v", said, err)
	}
}
