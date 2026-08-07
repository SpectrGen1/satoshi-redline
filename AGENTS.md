# AGENTS.md

## Project

SATOSHI//REDLINE is a local-first diagnostic and monitoring tool for Bitcoin node, wallet-backend, and mining infrastructure.

Its primary purpose is to answer:

* What failed?
* At which layer did it fail?
* What evidence supports that conclusion?
* What remains unknown?

The project is currently in its bootstrap phase. The accepted architecture decisions are under `docs/ADRs/`.

The current pre-release slice supports one manually configured Bitcoin Core or Bitcoin Knots node and one optional manually configured Electrs or Fulcrum backend with an explicit `uses_chain_from` dependency. It does not discover or manage either service.

## Source of Truth

Before making architectural or cross-cutting changes, read:

```text
docs/ADRs/index.md
docs/ADRs/ADR-001.md through ADR-010.md
```

Accepted ADRs are binding unless superseded by a later accepted ADR.

Do not silently work around an ADR. When implementation evidence shows that an ADR should change, propose a new ADR or an explicit amendment.

## Core Architecture

The backend is a Go modular monolith that may produce:

```text
redlined       unprivileged daemon and local API
redline        CLI and one-shot scanner
redline-privd  optional constrained privileged helper
```

The browser UI will use TypeScript and Svelte and will be built into local static assets.

Expected high-level package boundaries include:

```text
cmd/
internal/
  domain/
  engine/
  inventory/
  capabilities/
  probes/
  evaluation/
  correlation/
  incidents/
  platform/
  redaction/
  reports/
  storage/
  api/
  privileged/
ui/
schemas/
packaging/
testlab/
docs/ADRs/
```

Create directories only when the implementation requires them. Do not scaffold empty abstractions speculatively.

## Non-Negotiable Design Rules

### Diagnostic semantics

Keep these layers distinct:

```text
capabilities → probes → observations → findings → diagnoses
```

* Platform adapters provide access and discovery.
* Probes collect normalized facts.
* Evaluators interpret facts into findings.
* Correlation rules explain relationships and causes.
* Platform code must not emit findings or diagnoses.

Observations contain facts, not health judgments.

Findings use:

```text
pass | warning | failure | unknown
```

`unknown` always requires a structured reason. Missing required evidence must never produce a pass.

Health and diagnostic coverage are separate concepts.

### Platform support

Generic Linux, Docker, Umbrel, and StartOS packages must wrap the same diagnostic engine.

Platform packages may provide bindings, storage, lifecycle integration, and capabilities. They may not fork diagnostic semantics, thresholds, schemas, privacy behavior, or the embedded UI’s health model.

### Privilege

`redlined` must run unprivileged.

Privileged host inspection belongs only in `redline-privd`, which must expose a closed catalog of typed, bounded, read-only operations over a local Unix socket.

Never add:

* arbitrary shell execution
* arbitrary file reads
* generic Docker or D-Bus proxies
* destructive service operations
* public network access to the helper

### Privacy

Do not persist or log:

* raw protocol responses
* raw logs
* command output
* unrestricted configuration
* credentials or authorization headers
* external IP addresses
* onion addresses
* payout or wallet addresses
* arbitrary provider payloads

Extract approved typed fields and discard raw input.

Persistence, API, and report boundaries must use explicit allowlisted types. Do not introduce generic payload, metadata, debug, blob, or raw JSON fields.

### Networking

Core execution is local.

Do not add:

* telemetry
* analytics
* crash uploads
* automatic update checks
* remote feature flags
* cloud diagnosis
* external frontend assets
* arbitrary URL-fetching capabilities

Network probes must use destination-bound capabilities tied to configured or platform-declared components.

### Extensibility

For v0.1, probes, evaluators, and correlation rules are first-party code compiled into official binaries.

Do not add runtime plugin loading, shell probes, user scripts, external executable probes, or user-supplied WebAssembly without a new ADR.

## Contracts

External contracts must remain separate from internal Go types.

Use:

* OpenAPI for the local HTTP API
* JSON Schema for reports and exported diagnostic objects
* stable codes for observations, findings, diagnoses, unknown reasons, and recommendations

Do not serialize internal domain structs directly from API handlers or report exporters.

Treat schema compatibility and privacy review as part of every external-field change.

## Implementation Practices

Prefer:

* small concrete types
* explicit constructors and validation
* typed errors
* deterministic evaluators
* context-aware operations
* explicit deadlines
* bounded reads and outputs
* dependency injection through narrow interfaces
* table-driven tests
* synthetic protocol fixtures
* architecture tests for forbidden imports

Avoid:

* service locators
* global mutable state
* speculative interfaces
* arbitrary maps
* hidden I/O
* package initialization side effects
* reflection-based registration
* platform conditionals inside probes or evaluators
* premature microservices

Use standard-library functionality where practical. Add dependencies deliberately and document why they are needed.

## Testing Expectations

Every probe should cover:

* success
* target failure
* missing capability
* permission denial
* timeout and cancellation
* malformed or oversized input
* unsupported versions
* partial evidence
* secret-canary leakage
* declared observation validation

Every evaluator should cover:

* pass
* warning
* failure
* unknown
* stale evidence
* conflicting evidence
* not-applicable behavior
* boundary thresholds

Every correlation rule should cover:

* confirmed root cause
* break point without known cause
* multiple alternatives
* material unknown evidence
* unrelated simultaneous failures

Tests must verify that sensitive canaries never reach logs, persistence, API responses, reports, temporary files, or crash output.

## Working Process

Before implementing a change:

1. Identify the ADRs and contracts affected.
2. Define the diagnostic question and evidence required.
3. Separate collection, interpretation, and correlation.
4. State privilege, privacy, and network effects.
5. Add tests for failure and uncertainty paths.

When finishing a change:

1. Run formatting, static analysis, and tests available in the repository.
2. Confirm no raw or sensitive data crosses a durable boundary.
3. Confirm unavailable evidence cannot produce a false pass.
4. Update schemas and generated clients when contracts change.
5. Update relevant documentation and ADR references.
6. Keep commits focused and explain architectural consequences.

Do not invent build, lint, or test commands before the project selects its tooling. Once commands exist, document the canonical commands in this file.

## Canonical commands

```text
go build ./cmd/redline ./cmd/redlined
go test ./...
go test -race ./...
go vet ./...
```

Opt-in lab (bitcoin-vm-lab already running; does not manage VMs):

```text
./scripts/lab/prepare.sh --vm umbrel
./scripts/lab/smoke.sh --runtime-dir .lab/runtime/umbrel
./scripts/lab/scan.sh --runtime-dir .lab/runtime/umbrel
./scripts/lab/smoke-all.sh
```

Optional live integration procedures are documented under `docs/lab-integration.md`, `docs/live-bitcoin-smoke.md`, and `docs/live-electrum-smoke.md`. Live paths must remain explicitly opt-in and must never start or stop user services or lab VMs from `go test`.

## Lab integration (`bitcoin-vm-lab`)

The sibling repository [bitcoin-vm-lab](https://github.com/SpectrGen1/bitcoin-vm-lab) (local path typically `~/projects/bitcoin-vm-lab`) is the preferred **real-host** fixture for live Bitcoin / Electrum smoke tests and manual `redline` scans. It is **not** a dependency of this Go module: product binaries and `go test` packages never import, build, or drive lab lifecycle through `bvml`. Operators bring a running lab stack with `bvml`, then use opt-in host tooling.

Architecture: [docs/ADRs/ADR-011.md](docs/ADRs/ADR-011.md) (lab is a packaging consumer, not a package fork). Operator guide: [docs/lab-integration.md](docs/lab-integration.md). Scripts: [scripts/lab/](scripts/lab/).

### Verification tiers

| Tier | Meaning | How |
| ---- | ------- | --- |
| 0 | Offline engine | `go test ./...` |
| 1 | Protocol against lab endpoints | `./scripts/lab/prepare.sh` + `smoke.sh` / `scan.sh` |
| 2 | Same artifact on the guest | `sideload-binary.sh` (dev) or `sideload-package.sh` (when packages exist) |
| 3 | User publish | Official packages only; no lab-only product |

There is no `packaging/lab` fork. Official packages (when present) live under `packaging/` and are what the lab installs for Tier 2.

### What the lab provides

- Persistent KVM/libvirt guests: **Ubuntu** (Knots producer/consumer), **UmbrelOS**, and **StartOS**
- Shared **signet** canonical checkpoint with disposable per-VM overlays
- Protected Electrs / Fulcrum index bases and per-VM index overlays
- Official appliance packages on Umbrel and StartOS (native lifecycle via `umbreld` / `start-cli`)
- Operator CLI: `./bin/bvml` (`status`, `info`, `resume` / `start`, `adapter-setup`, `index-adapter-setup`, `stop`)

Lab lifecycle, safety, and recovery rules live in that repository’s `AGENTS.md` and `docs/OPERATIONS.md`. Do not bypass them with ad-hoc `virsh` / docker / disk edits from this project.

### Boundary rules (agents)

1. **Redline does not manage the lab.** Do not start, stop, install, reconfigure, promote, discard, or destroy lab VMs, overlays, or packages from product Go code or from `go test` packages. Lab lifecycle is operator-driven via `bvml`. Opt-in `scripts/lab/*` may read `bvml info` and copy cookies/binaries only.
2. **Live tests stay opt-in.** Unset `REDLINE_LIVE_*` variables must keep `go test ./...` offline and skip `testlab/livebitcoin` and `testlab/liveelectrum`.
3. **Cookies and paths stay local credentials.** Copy cookies only into operator-controlled files under `.lab/runtime/` (gitignored) or temp paths; never commit cookies, lab credentials, or guest IPs. Do not log or assert cookie contents.
4. **Platform fidelity:** Prefer scanning the **guest’s published RPC and Electrum ports** (or an SSH tunnel to guest loopback) rather than substituting a host bitcoind that is not the lab chain under test.
5. **Filesystem probe is host-local** for Tier 1. `bitcoin.data_directory` is `statfs`’d on the machine running Redline. For protocol-only smoke, a host temp/runtime dir is valid; capacity findings then describe the host, not the guest datadir. In-guest sideload changes that scope.
6. **No lab-only diagnostic semantics.** Do not add lab build tags, lab thresholds, or lab privacy exceptions in product code.

### Recommended lab readiness (operator)

From `bitcoin-vm-lab` with storage already prepared and a promoted signet checkpoint:

```text
./bin/bvml status
./bin/bvml resume umbrel          # or ubuntu / startos
./bin/bvml index-adapter-setup umbrel
./bin/bvml adapter-setup umbrel   # when Bitcoin app is not yet native-started
./bin/bvml info umbrel
```

Prefer **Umbrel** for the first host-side pass (published RPC + Electrum). Ubuntu needs `./scripts/lab/tunnel-ubuntu.sh` or in-guest sideload. StartOS needs explicit path/endpoint overrides until packaging bindings land.

### Host-side smoke (Tier 1, preferred)

From this repository after the lab guests are ready:

```sh
./scripts/lab/prepare.sh --vm umbrel
./scripts/lab/smoke.sh --runtime-dir .lab/runtime/umbrel

./scripts/lab/prepare.sh --vm ubuntu
./scripts/lab/smoke.sh --runtime-dir .lab/runtime/ubuntu

./scripts/lab/prepare.sh --vm startos
./scripts/lab/smoke.sh --runtime-dir .lab/runtime/startos

# or all three:
./scripts/lab/smoke-all.sh
```

Access details (RPC tunnels, StartOS LXC discovery, cookie `sudo cat`) are in `docs/lab-integration.md`. Raw env-var interface remains in `docs/live-bitcoin-smoke.md` and `docs/live-electrum-smoke.md`.

### In-guest binary (Tier 2-style dev shortcut)

```sh
./scripts/lab/sideload-binary.sh --vm umbrel
```

This is not package-store proof. When `packaging/` formats exist, use `sideload-package.sh` with the **same** artifact users would receive.

### Endpoint map (signet lab)

Confirm with `./bin/bvml info <vm>` and guest listeners. Typical values:

| Platform | Bitcoin RPC (from host) | Cookie (inside guest) | Electrs | Fulcrum | Notes |
| -------- | ----------------------- | --------------------- | ------- | ------- | ----- |
| Ubuntu   | Host tunnel `38332` → guest loopback | `/srv/bitcoin/signet/.cookie` | Tunnel `50001` | Tunnel `50002` | SSH private key required |
| Umbrel   | Host tunnel `9332` → guest loopback | `…/app-data/bitcoin-knots/data/bitcoin/signet/.cookie` | Guest IP `50001` | Guest IP `50002` | LAN `:9332` is 403 (rpcallowip) |
| StartOS  | Host tunnel `18332` → bitcoind LXC `:8332` | package volume `…/main/signet/.cookie` via sudo | Tunnel `25001` | Tunnel `25002` → LXC `:50001` | LXC IPs discovered at prepare |

Signet cookie layout is `$datadir/signet/.cookie`, not `$datadir/.cookie`.

### Interpreting results

- Live smoke **success** means authenticated transport, persisted graph integrity, API/report contracts, and privacy canaries — not that every finding is `pass`.
- IBD, tip lag, or storage pressure may produce `warning` / `failure` findings while the integration is still correct.
- Cross-platform runs (Ubuntu vs Umbrel vs StartOS) are separate operator exercises; do not encode lab VM names into product packages without an ADR.

The current release-candidate scope is the manual Bitcoin RPC and configured
filesystem vertical slice documented in `README.md` and
`docs/mvp-implementation.md`. It is pre-release software, not a production
readiness claim.
