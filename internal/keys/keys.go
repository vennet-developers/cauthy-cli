// Package keys makes a PACT_ROOT_KEY and says how an application rotates its
// own key. Rotation itself stays in the SDK: the CLI never holds the root key
// or the application's database.
package keys

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// RootKeyBytes is the root key's size: 32 random bytes, 43 characters in base64url.
const RootKeyBytes = 32

// NewRoot reads a fresh root key from random.
func NewRoot(random io.Reader) (string, error) {
	b := make([]byte, RootKeyBytes)
	if _, err := io.ReadFull(random, b); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// WriteRoot writes the key to path, readable by its owner only, making its
// folder (owner only) when missing. An existing file is refused unless force.
func WriteRoot(path, key string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s exists: a root key is never written over (pass --force if you mean it)", path)
	}
	if err != nil {
		return err
	}
	// O_TRUNC keeps an existing file's mode: set it.
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := fmt.Fprintln(f, key); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// RootWarning goes to standard error whenever a root key is printed.
const RootWarning = `This is a new PACT_ROOT_KEY. Put it in the deployment's secret manager now:
it is not stored anywhere else. Whoever holds it can open the application's
key and its people's sessions. Never commit it, log it or paste it into a
chat or a ticket.
`

// RotateHelp is what `cauthy keys rotate` prints.
const RotateHelp = `cauthy keys rotate does not touch the application's key, by design: the key
is sealed with PACT_ROOT_KEY in the application's own key store, and the CLI
never holds the root key or the application's database. Rotate it from the
running application (a one-off script, or an admin-only endpoint):

  Node  const kid = await auth.pact.keys.rotate()          // @cauthyhq/node
  Go    kid, err := auth.Pact().Keys().Rotate(ctx)        // cauthy-sdk-go

The new key is published at once and signs after six minutes; the old one
stays published for 24 hours. Then: cauthy pact check --live <app url>.
`

// ResealHelp is what `cauthy keys reseal` prints.
const ResealHelp = `cauthy keys reseal does not touch the application's key, by design (see
cauthy keys rotate). To rotate PACT_ROOT_KEY itself:

  1. cauthy keys new-root --out ~/.config/cauthy/new-root.key (outside the
     repository), and put the value in the secret manager as PACT_ROOT_KEY;
     the old value becomes PACT_ROOT_KEY_PREVIOUS. Deploy.
  2. From the running application:
       Node  await auth.pact.keys.reseal()                  // @cauthyhq/node
       Go    n, err := auth.Pact().Keys().Reseal(ctx)      // cauthy-sdk-go
  3. Remove PACT_ROOT_KEY_PREVIOUS and deploy again. Delete the key file.
`
