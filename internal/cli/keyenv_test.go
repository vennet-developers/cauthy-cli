package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vennet-developers/cauthy-sdk-spec/conformance/cauthykey"
)

// The spec's valid/basic vector. Its secret is the test value 00 01 … 1f,
// never a real key.
const (
	testKey       = "cauthy1.aHR0cHM6Ly9jYXV0aHkuYXBpLmV4YW1wbGUuY29t.Y2xpZW50XzAxMjM0NTY3ODlhYmNkZWYwMTIzNDU2Nzg5YWJjZGVm.AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8.AAvYhJZh"
	testKeySecret = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
	testKeyClient = "client_0123456789abcdef0123456789abcdef"
	testKeyIssuer = "https://cauthy.api.example.com"
	testKeyKID    = "dVpPrRtlQgOGn_9Q6DE2cgFRjUetrT2WPZCQyM48luc"
)

// damagedKey is testKey with its checksum changed.
const damagedKey = "cauthy1.aHR0cHM6Ly9jYXV0aHkuYXBpLmV4YW1wbGUuY29t.Y2xpZW50XzAxMjM0NTY3ODlhYmNkZWYwMTIzNDU2Nzg5YWJjZGVm.AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8.AAvYhJZi"

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadKeyFromTheEnvironment(t *testing.T) {
	env, _, _ := testEnv(t, map[string]string{"CAUTHY_KEY": testKey})
	writeFile(t, env.Dir, ".env", "CAUTHY_KEY="+damagedKey+"\n")
	k, from, err := readKey(&env, env.Dir, "CAUTHY_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if from != "environment" || k.Issuer != testKeyIssuer || k.ClientID != testKeyClient {
		t.Fatalf("from %q: %+v", from, k)
	}
}

func TestReadKeyFromDotEnv(t *testing.T) {
	for name, line := range map[string]string{
		"plain":         "CAUTHY_KEY=" + testKey,
		"double quoted": `CAUTHY_KEY="` + testKey + `"`,
		"single quoted": `CAUTHY_KEY='` + testKey + `'`,
		"export":        "export CAUTHY_KEY=" + testKey,
		"spaced":        "CAUTHY_KEY = " + testKey,
	} {
		t.Run(name, func(t *testing.T) {
			env, _, _ := testEnv(t, nil)
			writeFile(t, env.Dir, ".env", "# Acme CRM\nAPP_URL=http://localhost:3000\n"+line+"\n")
			k, from, err := readKey(&env, env.Dir, "CAUTHY_KEY")
			if err != nil {
				t.Fatal(err)
			}
			if from != ".env" || k.ClientID != testKeyClient {
				t.Fatalf("from %q: %+v", from, k)
			}
		})
	}
}

func TestReadKeyFromDotEnvLocal(t *testing.T) {
	env, _, _ := testEnv(t, nil)
	writeFile(t, env.Dir, ".env", "APP_URL=http://localhost:3000\n")
	writeFile(t, env.Dir, ".env.local", "CAUTHY_KEY="+testKey+"\n")
	k, from, err := readKey(&env, env.Dir, "CAUTHY_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if from != ".env.local" || k.ClientID != testKeyClient {
		t.Fatalf("from %q: %+v", from, k)
	}
}

func TestReadKeyDotEnvBeforeDotEnvLocal(t *testing.T) {
	env, _, _ := testEnv(t, nil)
	writeFile(t, env.Dir, ".env", "CAUTHY_KEY="+testKey+"\n")
	writeFile(t, env.Dir, ".env.local", "CAUTHY_KEY="+damagedKey+"\n")
	if _, from, err := readKey(&env, env.Dir, "CAUTHY_KEY"); err != nil || from != ".env" {
		t.Fatalf("from %q: %v", from, err)
	}
}

func TestReadKeyOtherName(t *testing.T) {
	env, _, _ := testEnv(t, map[string]string{"CAUTHY_KEY": damagedKey, "ACME_CAUTHY_KEY": testKey})
	k, from, err := readKey(&env, env.Dir, "ACME_CAUTHY_KEY")
	if err != nil || from != "environment" || k.ClientID != testKeyClient {
		t.Fatalf("from %q: %+v %v", from, k, err)
	}

	env, _, _ = testEnv(t, nil)
	writeFile(t, env.Dir, ".env", "CAUTHY_KEY="+damagedKey+"\nACME_CAUTHY_KEY="+testKey+"\n")
	if _, from, err := readKey(&env, env.Dir, "ACME_CAUTHY_KEY"); err != nil || from != ".env" {
		t.Fatalf("from %q: %v", from, err)
	}
}

func TestReadKeyNone(t *testing.T) {
	env, _, errOut := testEnv(t, nil)
	writeFile(t, env.Dir, ".env", "APP_URL=http://localhost:3000\n")
	_, _, err := readKey(&env, env.Dir, "CAUTHY_KEY")
	want := "No CAUTHY_KEY: connect the application in Cauthy's dashboard (Integrate → Applications) and put the line in .env"
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v", err)
	}
	if code := fail(env, err); code != ExitUsage {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(errOut.String(), want) {
		t.Fatalf("stderr: %s", errOut)
	}
}

func TestReadKeyDamaged(t *testing.T) {
	for name, setup := range map[string]func(t *testing.T) Env{
		"environment": func(t *testing.T) Env {
			env, _, _ := testEnv(t, map[string]string{"CAUTHY_KEY": damagedKey})
			return env
		},
		".env": func(t *testing.T) Env {
			env, _, _ := testEnv(t, nil)
			writeFile(t, env.Dir, ".env", "CAUTHY_KEY="+damagedKey+"\n")
			return env
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := setup(t)
			_, _, err := readKey(&env, env.Dir, "CAUTHY_KEY")
			var ke *cauthykey.Error
			if !errors.As(err, &ke) || ke.Reason != cauthykey.ReasonChecksum {
				t.Fatalf("err = %v", err)
			}
			if !strings.Contains(err.Error(), "key_checksum") {
				t.Fatalf("no reason in %q", err)
			}
			if strings.Contains(err.Error(), testKeySecret) || strings.Contains(err.Error(), damagedKey) {
				t.Fatalf("the error names the key: %q", err)
			}
		})
	}
}

// Only the key's line is read: nothing else in .env reaches any output.
func TestReadKeyKeepsOtherLinesOut(t *testing.T) {
	for _, key := range []string{testKey, damagedKey, ""} {
		env, out, errOut := testEnv(t, nil)
		body := "OTHER_SECRET=acme-correct-horse-battery\nDATABASE_URL=postgres://acme:hunter2@db.acme.test/crm\n"
		if key != "" {
			body += "CAUTHY_KEY=" + key + "\n"
		}
		writeFile(t, env.Dir, ".env", body)
		_, _, err := readKey(&env, env.Dir, "CAUTHY_KEY")
		if err != nil {
			fail(env, err)
		}
		all := out.String() + errOut.String()
		if err != nil {
			all += err.Error()
		}
		for _, leak := range []string{"acme-correct-horse-battery", "hunter2", "OTHER_SECRET", testKeySecret} {
			if strings.Contains(all, leak) {
				t.Fatalf("%q in the output:\n%s", leak, all)
			}
		}
	}
}
