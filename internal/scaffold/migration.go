package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// KeyTable is the table the SDKs' Postgres key store reads.
const KeyTable = "cauthy_pact_keys"

const tableSQL = `-- The application's own key for Pact, sealed with PACT_ROOT_KEY: the SDK's
-- Postgres key store (@cauthyhq/node/keys, cauthy-sdk-go/keys) reads it.
-- Written by cauthy init.
CREATE TABLE IF NOT EXISTS cauthy_pact_keys (
  kid        TEXT        PRIMARY KEY,
  public_jwk JSONB       NOT NULL,
  sealed     TEXT        NOT NULL,
  created_at TIMESTAMPTZ NOT NULL
);
`

const dropSQL = "DROP TABLE IF EXISTS cauthy_pact_keys;\n"

var (
	timestamp = regexp.MustCompile(`^(\d{14})_`)
	numbered  = regexp.MustCompile(`^(\d+)_`)
)

// Migration answers the key table's migration files for the application in
// dir, or none when a migration there already names the table. It writes into
// migrations/, db/migrations/ or sql/migrations/, whichever exists
// (migrations/ when none does), in the style the folder already uses.
func Migration(dir string, now time.Time) ([]File, error) {
	folder := "migrations"
	for _, candidate := range []string{"migrations", "db/migrations", "sql/migrations"} {
		if st, err := os.Stat(filepath.Join(dir, filepath.FromSlash(candidate))); err == nil && st.IsDir() {
			folder = candidate
			break
		}
	}
	entries, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(folder)))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	var names []string
	pairs := false
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(folder), e.Name()))
		if err != nil {
			return nil, err
		}
		if strings.Contains(string(raw), KeyTable) {
			return nil, nil
		}
		names = append(names, e.Name())
		pairs = pairs || strings.HasSuffix(e.Name(), ".up.sql")
	}
	base := folder + "/" + prefix(names, now) + "_" + KeyTable
	if pairs {
		return []File{{base + ".up.sql", tableSQL}, {base + ".down.sql", dropSQL}}, nil
	}
	return []File{{base + ".sql", tableSQL}}, nil
}

// prefix follows the folder's numbering: the next number, zero-padded like
// the others, or a UTC timestamp.
func prefix(names []string, now time.Time) string {
	width, last := 0, -1
	for _, n := range names {
		if timestamp.MatchString(n) {
			return now.UTC().Format("20060102150405")
		}
		if m := numbered.FindStringSubmatch(n); m != nil {
			v, _ := strconv.Atoi(m[1])
			if v > last {
				last, width = v, len(m[1])
			}
		}
	}
	if last < 0 {
		return now.UTC().Format("20060102150405")
	}
	return fmt.Sprintf("%0*d", width, last+1)
}
