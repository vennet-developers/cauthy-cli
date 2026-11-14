# cauthy-cli

`cauthy` works with an application on [Cauthy](https://github.com/vennet-developers):
it says what the application's `CAUTHY_KEY` names, wires the application to Cauthy on
that key alone, and certifies a backend against the `auth-bff` protocol.

An application connects in Cauthy's dashboard (Integrate → Applications), which hands
over one line, `CAUTHY_KEY=…`, for the secret manager and the local `.env`. Then:

```sh
npx @cauthyhq/cli key inspect --check
npx @cauthyhq/cli init
```

## Install

| How | Command |
| --- | --- |
| npm (no install script; the binary comes as a per-platform package and its SHA-256 is checked on every run) | `npx @cauthyhq/cli …`, or `npm install -D @cauthyhq/cli` and then `npx cauthy …` (the command is `cauthy`) |
| Homebrew (macOS, Linux) | `brew install vennet-developers/tap/cauthy` |
| Go | `go install github.com/vennet-developers/cauthy-cli/cmd/cauthy@latest` |
| Binary | the archive for your platform from the [releases](https://github.com/vennet-developers/cauthy-cli/releases), after verifying it (below) |

## Commands

| Command | What it does |
| --- | --- |
| `cauthy key inspect [--check] [--key-env <name>]` | What the application's `CAUTHY_KEY` names: its Cauthy, its client, its key id and whether its checksum is intact; `--check` also asks Cauthy whether it knows the key. Reads the environment, else `.env` or `.env.local` (only that one line), and never prints the key's secret |
| `cauthy init [--key-env <name>]` | Wires an application to Cauthy on `CAUTHY_KEY` alone, as the five-minute guide does: says what the key names and what Cauthy says the application is (workspace, system, addresses, and the client people sign in through when it is not the key's), detects the application (Node: Next.js 15.5 or newer, TanStack Start, Hono, Express, Fastify; Go: net/http, chi, Gin, Fiber, Echo), and writes the SDK wiring (`createCauthy()` and the `/api/auth` route; `cauthy.New(cauthy.Options{})` and its mount) and `CAUTHY_KEY=` in `.env.example` (never a value). Run it again any time: it never writes over a file without `--force` |
| `cauthy conformance --origin <url> --email <account>` | Runs the `auth-bff` conformance suite (password in `CONFORMANCE_PASSWORD`); `--key-env <name>` adds the `key.*` checks for a backend on `CAUTHY_KEY` |
| `cauthy conformance … --level switches --cauthy <url> --tenant <slug>` | Also checks the backend follows the dashboard switches of a test workspace, and puts them back (admin token in `CAUTHY_ADMIN_TOKEN`) |
| `cauthy version` | Version, commit, and the protocol version it certifies |

For a Go application, `init` asks you to run `go get github.com/vennet-developers/cauthy-sdk-go@latest`.

Exit codes: `0` ok; `1` a check failed;
`2` usage or an unreadable input; `3` nothing failed, but a check that was asked for
did not run.

`cauthy` never prints a secret: variables are named, never shown.

## Verify a release

Every release has `checksums.txt`, signed keyless with Sigstore by this repository's
release workflow, and a build provenance attestation.

```sh
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/vennet-developers/cauthy-cli/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c checksums.txt
gh attestation verify cauthy_<version>_linux_amd64.tar.gz --repo vennet-developers/cauthy-cli
```

The npm packages are published with provenance: `npm audit signatures` checks them.

## Develop

```sh
go vet ./... && go test -race ./...
node --test npm/cli/test/platform.test.js npm/scripts/stage.test.mjs
goreleaser release --snapshot --clean --skip=sign
```

## License

Apache License 2.0: see [LICENSE](LICENSE) and [NOTICE](NOTICE).
