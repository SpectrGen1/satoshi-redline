# SATOSHI//REDLINE MVP

This pre-release MVP is a locally executed, unprivileged Go vertical slice for one manually configured Bitcoin Core or Bitcoin Knots RPC endpoint and its data directory. It collects approved chain-state and filesystem facts, evaluates four findings, stores normalized records in SQLite, and exposes the same scan-and-persist service through `redline` and `redlined`. It is not production-ready.

Build and test:

```sh
go build ./cmd/redline ./cmd/redlined
go test ./...
go test -race ./...
go vet ./...
```

Copy `redline.example.json` to `redline.json`, set the local endpoint, cookie-file reference, data directory, and database path. Environment overrides are `REDLINE_RPC_ENDPOINT`, `REDLINE_DATA_DIRECTORY`, `REDLINE_API_LISTEN_ADDRESS`, `REDLINE_DATABASE_PATH`, and `REDLINE_RPC_TIMEOUT_MS`.

RPC authentication supports an existing Bitcoin cookie file or an environment-backed cookie value for local development. Transport evidence records authentication as `accepted`, `rejected`, or `unknown`; lack of evidence is never represented as rejection.

```sh
go run ./cmd/redline --config redline.json scan --json
go run ./cmd/redlined --config redline.json
curl -X POST http://127.0.0.1:8383/api/v1/scans
curl -N http://127.0.0.1:8383/api/v1/events
go run ./cmd/redline --config redline.json report SCAN_ID
```

`POST /api/v1/scans` runs synchronously: success is returned only after the normalized scan is persisted. Each scan workflow emits exactly one terminal event, `scan.completed` after all required workflow steps succeed or `scan.failed` after a workflow failure. SSE events are in-memory, non-replayable, and available only to connected subscribers; heartbeats keep a live stream writable without disabling the daemon’s global write timeout.

The daemon binds to loopback by default. Diagnostic network traffic goes only to the configured Bitcoin RPC target; there is no telemetry, update check, remote service, or external asset. A configured target can be outside loopback, so the operator remains responsible for endpoint routing and TLS trust.

The RPC cookie is consumed only by the destination-bound provider. Credentials, headers, raw requests, raw responses, peer/local/proxy addresses, configuration paths, and directory contents are neither logged nor persisted or exported. SQLite stores only approved normalized fields. Report v1 exports use explicit transport types, pass reference-integrity checks, and are validated at runtime against the embedded Draft 2020-12 JSON Schema.

Current limitations are deliberate: one manual endpoint; synchronous scans only; Linux `statfs` filesystem collection; no automatic discovery; no UI, platform package, privileged helper, continuous monitoring, incidents, correlation diagnoses, wallet RPC, external services, telemetry, updates, plugins, or remediation. Contract and privacy tests validate actual HTTP responses, report exports, SQLite database/journal/WAL/SHM artifacts, CLI/daemon errors, and SSE failure events.
