package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	if !strings.HasPrefix(string(raw), "DATABASE_URL=\n# ") || strings.Count(string(raw), "PACT_ROOT_KEY=") != 1 || !strings.HasSuffix(string(raw), "PACT_ROOT_KEY=\n") {
		t.Fatalf("%q", raw)
	}
	if err := os.WriteFile(path, []byte("PACT_ROOT_KEY=abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if said, _ := EnvExample(dir); !strings.HasPrefix(said, "has a value") {
		t.Fatalf("%q", said)
	}
}

func TestMigrationFollowsTheFolder(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 30, 0, 0, time.UTC)
	mk := func(files ...string) string {
		dir := t.TempDir()
		for _, f := range files {
			p := filepath.Join(dir, filepath.FromSlash(f))
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("CREATE TABLE things (id int);\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	paths := func(fs []File) string {
		var out []string
		for _, f := range fs {
			out = append(out, f.Path)
		}
		return strings.Join(out, " ")
	}
	cases := map[string]struct {
		dir  string
		want string
	}{
		"no folder":       {mk(), "migrations/20261107123000_cauthy_pact_keys.sql"},
		"numbered pairs":  {mk("db/migrations/000011_things.up.sql", "db/migrations/000011_things.down.sql"), "db/migrations/000012_cauthy_pact_keys.up.sql db/migrations/000012_cauthy_pact_keys.down.sql"},
		"timestamped":     {mk("migrations/20260101000000_things.sql"), "migrations/20261107123000_cauthy_pact_keys.sql"},
		"numbered single": {mk("sql/migrations/7_things.sql"), "sql/migrations/8_cauthy_pact_keys.sql"},
	}
	for name, tc := range cases {
		fs, err := Migration(tc.dir, now)
		if err != nil || paths(fs) != tc.want {
			t.Errorf("%s: %v %q, want %q", name, err, paths(fs), tc.want)
		}
	}
	fs, _ := Migration(mk(), now)
	if !strings.Contains(fs[0].Content, "CREATE TABLE IF NOT EXISTS cauthy_pact_keys") || !strings.Contains(fs[0].Content, "public_jwk JSONB") {
		t.Fatalf("%s", fs[0].Content)
	}
	// A migration that already names the table: nothing to add.
	done := mk()
	if err := os.MkdirAll(filepath.Join(done, "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(done, "migrations", "001_keys.sql"), []byte("CREATE TABLE cauthy_pact_keys ();"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fs, err := Migration(done, now); err != nil || len(fs) != 0 {
		t.Fatalf("%v %+v", err, fs)
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
