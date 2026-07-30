# MVP implementation note

`internal/capabilities` owns destination-bound RPC and configured-path filesystem access. `internal/engine` invokes those providers and emits fact-only observations; `internal/evaluation` converts them to findings and distinct health/coverage summaries. API and report packages map the domain into explicit v1 transport types.

The scan flow is configured node → `bitcoin.rpc.read/v1` equivalent provider → chain-state observation → evaluators → component summary. The filesystem capability reads `statfs` only and never enumerates the data directory. SQLite persists scans, probe executions, approved observations, findings, and summaries; no raw protocol/configuration fields exist in the schema.

Known limitations: one manually configured endpoint, no discovery or correlation/diagnoses, Linux filesystem stat only, synchronous API scans, and a minimal one-event SSE progress stream. The slice follows ADR-001 through ADR-009: a Go monolith, scoped capabilities, fact/finding separation, unprivileged operation, no raw durable data, versioned contracts, compiled probes, explicit unknown reasons, and local-only networking. ADR-010 remains proposed and no platform package is included.
