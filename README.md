# sandstorm-utils

Go-based Sandstorm utilities.

## Scope

These utilities are meant to sit between app-specific capability
implementations and a fully generic Cap'n Proto RPC client.

The goal is to cover common Sandstorm app workflows through narrowly
defined command-line tools, so apps can reuse standard operations without
having to define their own custom capabilities for every routine task.

A utility belongs in this repo when it:

- maps to a common Sandstorm operation or workflow that multiple apps are
  likely to need
- has stable semantics that can be expressed with a small, intentional CLI
  surface
- preserves meaningful type information instead of collapsing everything
  into opaque blobs
- can return structured output that remains useful without app-specific
  schema knowledge

This repo should stay focused on curated adapters for common
tasks.

Current layout:

- `cmd/get-public-id`: `HackSessionContext.getPublicId()`
- `cmd/get-user-address`: `HackSessionContext.getUserAddress()`
- `cmd/close-session`: `SessionContext.close()`
- `cmd/open-view`: `SessionContext.openView()`
- `cmd/post-activity`: `SessionContext.activity()`
- `cmd/get-session-request`: `SandstormHttpBridge.getSessionRequest()`
- `cmd/get-session-offer`: `SandstormHttpBridge.getSessionOffer()`
- `cmd/send-email`: `EmailSendPort.send()` via `HackSessionContext`
- `cmd/stay-awake`: helper that holds `SandstormApi.stayAwake()` for the life of the process
- `cmd/enter-grain`: helper that joins a grain process's Linux namespaces and launches a shell
- `testapp`: Sandstorm integration harness that shells out to the utilities
- `internal/sandstorm`: shared bridge/session client logic
- `schemas/sandstorm`: vendored annotated Cap'n Proto schemas
- `internal/generated/*`: generated Go bindings by schema package
- `scripts/generate-capnp.sh`: regenerates Go bindings using the C++ `capnp` compiler plus a locally built `capnpc-go` plugin

Generate bindings:

```bash
./scripts/generate-capnp.sh
```

Build all commands:

```bash
go build ./cmd/...
```

Build local development binaries into `dist/bin`:

```bash
make build
```

Generate the utility manifest consumed by downstream tooling:

```bash
make manifest
```

The release workflow also publishes `manifest/utils.json` as a release asset so
downstream tooling can discover the available utilities and their summaries.

Build the Linux `amd64` release tarball used for Sandstorm integration:

```bash
make package VERSION=v0.1.0 GOOS=linux GOARCH=amd64
```

The published GitHub release currently only ships this `linux/amd64` archive,
since that is the deployment target that matters for Sandstorm.

Build the Sandstorm integration harness payload locally:

```bash
rm -rf testapp/bin
make build BINDIR=$(pwd)/testapp/bin
go build -o testapp/bin/testapp-server ./testapp/cmd/testapp-server
```

Run the full test suite:

```bash
make test
```

Install the commands into a prefix:

```bash
make install PREFIX=$HOME/.local
```

Command examples:

```bash
./get-public-id <session-id>
./get-public-id --json <session-id>

./get-user-address <session-id>
./get-user-address --json <session-id>

./close-session <session-id>

./open-view --path /docs/123 <session-id>
./open-view --path /docs/123 --new-tab <session-id>

./post-activity \
  --json-input event.json \
  --path /issues/1#comment-2 \
  --type 3 \
  --thread-path /issues/1 \
  --thread-title "Issue 1" \
  --caption "New comment" \
  <session-id>

./get-session-request <session-id>
./get-session-offer <session-id>

./send-email --to user@example.com --subject "Hello" --text "Hi there" <session-id>
./send-email --json-input message.json <session-id>

./stay-awake --title "Transcoding video" --caption "Encoding in the background"
./stay-awake --for 30s --title "Transcoding video" --caption "Encoding in the background"

./enter-grain <pid>
```

Behavior notes:

- Every command accepts `--timeout`, defaulting to `10s`.
- `get-public-id` and `get-user-address` support both plain text and `--json`.
- Their JSON output uses stable lower-camel field names such as `publicId`,
  `autoUrl`, `address`, and `name`.
- `open-view` and `post-activity` normalize a leading `/` out of Sandstorm
  grain-relative paths before making RPC calls.
- `post-activity --json-input FILE` reads a richer JSON payload, with `-`
  supported for stdin; simple flags still override JSON values.
- Localized text inside `post-activity --json-input` may be either a bare
  string or an object with `defaultText` and `localizations`; unknown JSON
  fields are rejected.
- `post-activity --json-input` also accepts `users[]` entries keyed by
  saved `identityId`, which are resolved through the bridge's
  `getSavedIdentity()` helper.
- `get-session-request` and `get-session-offer` emit JSON and decode common
  Powerbox tag payloads when possible; unknown tag payloads now get typed
  summaries like text/data/struct/list/interface instead of a generic opaque
  marker.
- `send-email` defaults the sender to `get-user-address()` when `--from` is
  not provided, and accepts either direct flags or `--json-input FILE`.
- `stay-awake` acquires a Sandstorm wake lock and keeps it alive for the life
  of the helper process; close stdin or send a termination signal to release it.
- `enter-grain` is Linux-only; it joins the target process's namespaces and
  launches `/bin/bash` with that process's environment.
- `testapp/` contains a minimal Sandstorm app plus Lima/SPK workflows for
  packaging an integration harness. Its package ID should match the signing key
  stored in `SANDSTORM_TESTAPP_KEYRING_B64`.
- For local Sandstorm development of the integration harness, run `lima-spk`
  from the repo root so the VM sees the full repository at `/opt/app`.
