# cauthy-cli

`cauthy` connects an application to [Cauthy](https://github.com/vennet-developers) by
contract, in any language: it writes what the application needs, vendors its Pact
contract, checks it before and after deploying, and certifies a backend against the
`auth-bff` protocol.

```sh
npx @cauthyhq/cli init --app-url https://crm.example.com --app-url http://localhost:3000
npx @cauthyhq/cli contracts pull --from ../core-contracts
npx @cauthyhq/cli pact check --system acme-crm --live https://crm.example.com
```

## Install

| How | Command |
| --- | --- |
| npm (no install script; the binary comes as a per-platform package and its SHA-256 is checked on every run) | `npx @cauthyhq/cli …`, or `npm install -D @cauthyhq/cli` and then `npx cauthy …` (the command is `cauthy`) |
| Go | `go install github.com/vennet-developers/cauthy-cli/cmd/cauthy@latest` |
| Binary | the archive for your platform from the [releases](https://github.com/vennet-developers/cauthy-cli/releases), after verifying it (below) |

## Commands

| Command | What it does |
| --- | --- |
| `cauthy init --app-url <url>...` | Detects the application (Node: Next.js 15.5 or newer, TanStack Start, Hono, Express, Fastify; Go: net/http, chi, Gin, Fiber, Echo). Prints the Pact connection request to email to Vennet at support@vennet.dev (or, with `--contracts <core-contracts checkout>`, drafts the link and the manifest there). Writes the auth wiring, the `cauthy_pact_keys` migration and `PACT_ROOT_KEY=` in `.env.example` (never a value). Pulls the graph when the contracts are at hand. Run it again any time: it never writes over a file without `--force` |
| `cauthy contracts pull --from <dir or https url> --to <dir>...` | Writes `graph.json` and `VERSION` into every destination and checks the copies are identical. Refuses a checkout with uncommitted changes unless `--allow-dirty` |
| `cauthy contracts check --to <dir>...` | Fails when vendored copies differ (for CI) |
| `cauthy pact check --system <id>` | What the contract says, whether the environment would make the app refuse to start, and (with `CAUTHY_TOKEN`) how Cauthy sees it. `--live <url>` probes the deployed JWKS, session and sign-in redirect |
| `cauthy keys new-root [--out <file>]` | Makes a `PACT_ROOT_KEY` (to a 0600 file with `--out`; keep it outside the repository, such as `~/.config/cauthy/pact-root.key`) |
| `cauthy keys rotate`, `cauthy keys reseal` | Explain how to rotate from the running application: the CLI never holds the root key |
| `cauthy conformance --origin <url> --email <account>` | Runs the `auth-bff` conformance suite (password in `CONFORMANCE_PASSWORD`) |
| `cauthy conformance … --level switches --cauthy <url> --tenant <slug>` | Also checks the backend follows the dashboard switches of a test workspace, and puts them back (admin token in `CAUTHY_ADMIN_TOKEN`) |
| `cauthy version` | Version, commit, and the protocol version it certifies |

For a Go application, `init` asks you to run `go get github.com/vennet-developers/cauthy-sdk-go@latest`:
that works once cauthy-sdk-go 1.0 is published. Until then, the Node wiring is the one
to use.

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
