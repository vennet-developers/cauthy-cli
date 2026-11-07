package keys

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNewRoot(t *testing.T) {
	a, err := NewRoot(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewRoot(rand.Reader)
	if len(a) != 43 || a == b || strings.ContainsAny(a, "+/=") {
		t.Fatalf("%q %q", a, b)
	}
	if _, err := NewRoot(bytes.NewReader([]byte{1, 2, 3})); err == nil {
		t.Fatal("a short random source was accepted")
	}
}

func TestWriteRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "root.key")
	if err := WriteRoot(path, "first", false); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
	if err := WriteRoot(path, "second", false); err == nil || !strings.Contains(err.Error(), "never written over") {
		t.Fatalf("got %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteRoot(path, "second", true); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	st, _ = os.Stat(path)
	if string(raw) != "second\n" || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o600) {
		t.Fatalf("%q %v", raw, st.Mode().Perm())
	}
}
