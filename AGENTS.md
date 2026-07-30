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

Optional live integration procedures are documented under `docs/live-bitcoin-smoke.md` and `docs/live-electrum-smoke.md`. They must remain explicitly opt-in and must never start or stop user services.

The current release-candidate scope is the manual Bitcoin RPC and configured
filesystem vertical slice documented in `README.md` and
`docs/mvp-implementation.md`. It is pre-release software, not a production
readiness claim.
