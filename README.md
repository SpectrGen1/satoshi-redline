# SATOSHI//REDLINE MVP

This is a local-only, unprivileged Go vertical slice for one manually configured Bitcoin Core/Knots RPC endpoint and its data directory. It collects approved chain-state and filesystem facts, evaluates four findings, stores normalized records in SQLite, and exposes the same engine through `redline` and `redlined`.

Build and test:

```sh
go build ./cmd/redline ./cmd/redlined
go test ./...
go vet ./...
```

Copy `redline.example.json` to `redline.json`, set the local endpoint, cookie-file reference, data directory, and database path. Environment overrides are `REDLINE_RPC_ENDPOINT`, `REDLINE_DATA_DIRECTORY`, `REDLINE_API_LISTEN_ADDRESS`, `REDLINE_DATABASE_PATH`, and `REDLINE_RPC_TIMEOUT_MS`.

```sh
go run ./cmd/redline --config redline.json scan --json
go run ./cmd/redlined --config redline.json
curl -X POST http://127.0.0.1:8383/api/v1/scans
go run ./cmd/redline --config redline.json report SCAN_ID
```

The RPC cookie is consumed only by the destination-bound provider. Credentials, headers, raw requests, raw responses, peer/local/proxy addresses, and directory contents are neither logged nor persisted/exported. This MVP has no UI, privileged helper, discovery, packaging, monitoring, incidents, wallet RPC, external services, telemetry, updates, or remediation. SQLite stores only approved normalized fields.
