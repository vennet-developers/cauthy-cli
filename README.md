# cauthy-cli

`cauthy` works with an application on [Cauthy](https://github.com/vennet-developers):
it says what the application's `CAUTHY_KEY` names, adds a Pact contract to an
application already running on that key, vendors the contract, checks it before and
after deploying, and certifies a backend against the `auth-bff` protocol.

An application connects in Cauthy's dashboard (Integrate → Applications), which hands
over one line, `CAUTHY_KEY=…`, for the secret manager and the local `.env`. Then:

```sh
npx @cauthyhq/cli key inspect --check
npx @cauthyhq/cli init
npx @cauthyhq/cli contracts pull --from ../core-contracts
npx @cauthyhq/cli pact check --system acme-crm --live https://crm.example.com
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
| `cauthy init [--key-env <name>]` | Adds a contract to an application already running on `CAUTHY_KEY`: asks Cauthy who the application is (system, workspace, addresses), detects the application (Node: Next.js 15.5 or newer, TanStack Start, Hono, Express, Fastify; Go: net/http, chi, Gin, Fiber, Echo), and prints the Pact connection request to email to Vennet at support@vennet.dev (or, with `--contracts <core-contracts checkout>`, drafts the link and the manifest there). Writes the contract wiring (no key store, no migration) and `CAUTHY_KEY=` in `.env.example` (never a value). Pulls the graph when the contracts are at hand. The application deploys with the same `CAUTHY_KEY`. Run it again any time: it never writes over a file without `--force` |
| `cauthy contracts pull --from <dir or https url> --to <dir>...` | Writes `graph.json` and `VERSION` into every destination and checks the copies are identical. Refuses a checkout with uncommitted changes unless `--allow-dirty` |
| `cauthy contracts check --to <dir>...` | Fails when vendored copies differ (for CI) |
| `cauthy pact check --system <id>` | What the contract says, whether the environment would make the app refuse to start (`CAUTHY_KEY` parses and names the contract's Cauthy; no variable it replaces is set beside it), and (with `CAUTHY_TOKEN`) how Cauthy sees it, with the keys registered against the ones published. `--live <url>` probes the deployed JWKS (the key's Ed25519 public half, by its key id), session and sign-in redirect |
| `cauthy keys new-root [--out <file>]` | Deprecated: `CAUTHY_KEY` replaces `PACT_ROOT_KEY`. Still makes one (to a 0600 file with `--out`) for applications not moved yet, and says so on standard error |
| `cauthy keys rotate`, `cauthy keys reseal` | Explain how to rotate a `PACT_ROOT_KEY` from the running application: the CLI never holds the root key |
| `cauthy conformance --origin <url> --email <account>` | Runs the `auth-bff` conformance suite (password in `CONFORMANCE_PASSWORD`); `--key-env <name>` adds the `key.*` checks for a backend on `CAUTHY_KEY` |
| `cauthy conformance … --level switches --cauthy <url> --tenant <slug>` | Also checks the backend follows the dashboard switches of a test workspace, and puts them back (admin token in `CAUTHY_ADMIN_TOKEN`) |
| `cauthy version` | Version, commit, and the protocol version it certifies |

For a Go application, `init` asks you to run `go get github.com/vennet-developers/cauthy-sdk-go@latest`.

Exit codes: `0` ok; `1` a check failed, or the application would refuse to start;
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
