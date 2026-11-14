# Changelog

## 1.3.0

- Contracts are retired: an application runs on `CAUTHY_KEY` alone, and the same key keeps its sessions (`@cauthyhq/node` 1.3.0 and `cauthy-sdk-go` v1.3.0 sign people in through the client Cauthy names for the key).
- **`init` wires `CAUTHY_KEY` alone**, as the five-minute guide does: it says what the key names and what Cauthy says the application is (including the client people sign in through, when it is not the key's), and writes the SDK in key mode (`createCauthy()` and the `/api/auth` route; `cauthy.New(cauthy.Options{})` and its mount) and `CAUTHY_KEY=` in `.env.example`. It no longer drafts or requests a Pact link, writes no contract wiring and no JWKS route, and does not run `contracts pull`. The contract flags (`--app-url`, `--system`, `--tenant`, `--callback-path`, `--no-service`, `--persistent-sessions`, `--from`, `--to`, `--no-pull`) are a usage error without `--contracts`.
- **Deprecated until 2.0**: `init --contracts` (drafts the link into a core-contracts checkout and writes the contract wiring, as before), `contracts pull`, `contracts check` and `pact check`. Each keeps working and prints a deprecation warning on stderr pointing to the "Protect sign-in" switches on the application's card in Cauthy's dashboard.

## 1.2.0

- `init` adds a contract to an application already running on `CAUTHY_KEY`: it asks Cauthy who the application is (system, workspace, addresses), writes the contract wiring with no key store and no migration, and `CAUTHY_KEY=` in `.env.example`; the application deploys with the same key.
- `key inspect` says what a `CAUTHY_KEY` names (Cauthy, client, key id, checksum) and, with `--check`, whether Cauthy knows it; it never prints the key's secret.
- `pact check` checks the key: it parses and names the contract's Cauthy, no variable it replaces is set beside it, `--live` finds its Ed25519 public half in the JWKS, and Cauthy's status shows the keys registered against the ones published. `conformance --key-env` runs the `key.*` checks.
- `keys new-root` is deprecated, and so is `PACT_ROOT_KEY`: `CAUTHY_KEY`, made on the application's card in Cauthy's dashboard, replaces it.
