# SATOSHI//REDLINE MVP

This pre-release MVP is a locally executed, unprivileged Go diagnostic slice for one manually configured Bitcoin Core or Bitcoin Knots node, its data directory, and an optional manually configured Electrs or Fulcrum backend. It records an explicit Electrum→Bitcoin dependency, collects bounded protocol evidence, evaluates component findings, and correlates the first cross-layer breakpoint diagnoses. It is not production-ready.

Build and test:

```sh
go build ./cmd/redline ./cmd/redlined
go test ./...
go test -race ./...
go vet ./...
```

Copy `redline.example.json` to `redline.json`, set the local Bitcoin endpoint, cookie-file reference, data directory, and database path. Electrum remains disabled unless `electrum.enabled` is true and an explicit `tcp://` or `ssl://` endpoint and `bitcoin.primary` binding are supplied. Environment overrides include `REDLINE_RPC_ENDPOINT`, `REDLINE_DATA_DIRECTORY`, `REDLINE_API_LISTEN_ADDRESS`, `REDLINE_DATABASE_PATH`, `REDLINE_RPC_TIMEOUT_MS`, `REDLINE_ELECTRUM_ENABLED`, `REDLINE_ELECTRUM_ENDPOINT`, `REDLINE_ELECTRUM_TIMEOUT_MS`, and `REDLINE_ELECTRUM_BITCOIN_COMPONENT_ID`.

RPC authentication supports an existing Bitcoin cookie file or an environment-backed cookie value for local development. Transport evidence records authentication as `accepted`, `rejected`, or `unknown`; lack of evidence is never represented as rejection.

```sh
go run ./cmd/redline --config redline.json scan --json
go run ./cmd/redlined --config redline.json
curl -X POST http://127.0.0.1:8383/api/v1/scans
curl -N http://127.0.0.1:8383/api/v1/events
go run ./cmd/redline --config redline.json report SCAN_ID
```

`POST /api/v1/scans` runs synchronously: success is returned only after the normalized scan is persisted. Each scan workflow emits exactly one terminal event, `scan.completed` after all required workflow steps succeed or `scan.failed` after a workflow failure. SSE events are in-memory, non-replayable, and available only to connected subscribers; heartbeats keep a live stream writable without disabling the daemon’s global write timeout.

Scan-owned API resources include `/api/v1/scans/{scan_id}/components`, `/graph`, and `/diagnoses` in addition to observations and findings. Reports archive the same normalized component and evidence graph.

The daemon binds to loopback by default. Diagnostic network traffic goes only to the configured Bitcoin RPC and Electrum targets; there is no discovery, telemetry, update check, remote service, or external asset. Electrum TCP/TLS connections use explicit deadlines and normal TLS certificate verification. A configured target can be outside loopback, so the operator remains responsible for routing and trust.

The RPC cookie is consumed only by the destination-bound provider. Credentials, headers, raw requests, raw responses, peer/local/proxy addresses, configuration paths, and directory contents are neither logged nor persisted or exported. SQLite stores only approved normalized fields. Report v1 exports use explicit transport types, pass reference-integrity checks, and are validated at runtime against the embedded Draft 2020-12 JSON Schema.

Current limitations are deliberate: one manual Bitcoin node and one optional manual Electrum backend; no wallet-client checks; synchronous scans only; Linux `statfs` filesystem collection; in-memory non-replayable SSE; no automatic discovery, UI, platform package, privileged helper, continuous monitoring, incidents, DATUM, Stratum, Tor routing, external services, telemetry, updates, plugins, or remediation. Correlation identifies breakpoints and contributing factors but does not claim a root cause in this slice.

The opt-in live procedures are documented in `docs/live-bitcoin-smoke.md` and `docs/live-electrum-smoke.md`. Neither test installs, starts, or stops an Electrum backend.

## Testing against bitcoin-vm-lab

For real Knots + Electrs/Fulcrum targets, use the sibling **bitcoin-vm-lab** project (typically `~/projects/bitcoin-vm-lab`). That repository runs Ubuntu, UmbrelOS, and StartOS under libvirt with a shared **signet** checkpoint and disposable overlays.

Redline **consumes** lab endpoints and (later) the same packages users install. It does not fork a lab-only product and does not drive lab lifecycle from `go test`. See [docs/ADRs/ADR-011.md](docs/ADRs/ADR-011.md) and [docs/lab-integration.md](docs/lab-integration.md).

**1. Bring the lab stack up** (lab repo; see its `README.md` / `docs/OPERATIONS.md`):

```sh
cd ~/projects/bitcoin-vm-lab
./bin/bvml status
./bin/bvml resume umbrel          # or ubuntu / startos
./bin/bvml index-adapter-setup umbrel
./bin/bvml info umbrel
```

**2. Tier 1 host-side smoke** (this repo):

```sh
./scripts/lab/prepare.sh --vm umbrel    # also: ubuntu | startos
./scripts/lab/smoke.sh --runtime-dir .lab/runtime/umbrel
./scripts/lab/scan.sh --runtime-dir .lab/runtime/umbrel

# all three lab consumers:
./scripts/lab/smoke-all.sh
```

Runtime state (cookie path, config, env, tunnels) lands in `.lab/runtime/<vm>/` and is gitignored. Cookie **contents** are never logged. Ubuntu/Umbrel/StartOS each use SSH tunnels where host LAN RPC is unusable; StartOS discovers bitcoind/electrs/fulcrum LXC addresses automatically.

**3. Optional in-guest binary** (dev shortcut, not package proof):

```sh
./scripts/lab/sideload-binary.sh --vm umbrel
```

Notes:

- `go test ./...` stays offline unless `REDLINE_LIVE_*` is set (or you use `scripts/lab/smoke.sh`).
- Unhealthy findings can still be a successful integration: the node may lag or report storage pressure while Redline correctly collected evidence.
- Official packages will live under `packaging/` with no lab-specific fork; agent detail in `AGENTS.md`.
