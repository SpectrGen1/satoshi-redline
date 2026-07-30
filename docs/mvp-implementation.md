# MVP implementation note

`internal/capabilities` owns destination-bound RPC and configured-path filesystem access. The engine scopes declared capabilities to compiled probes, enforces probe deadlines, validates sealed typed observations, and passes them to deterministic evaluators. API and report packages map the complete evidence graph into independent v1 transport types.

The scan flow is configured node → scoped capability → compiled probe → typed observation → evaluator → finding → separate health and coverage summaries. SQLite uses ordered embedded migrations and normalized typed child tables; no raw protocol/configuration fields exist in the schema.

Known limitations: one manually configured endpoint, no discovery or correlation/diagnoses, Linux filesystem stat only, synchronous API scans, and an in-memory non-replayable SSE lifecycle stream. The slice follows accepted ADR-001 through ADR-010: a Go monolith, scoped capabilities, fact/finding separation, unprivileged operation, no raw durable data, versioned contracts, compiled probes, explicit unknown reasons, local-only networking, and no platform-specific engine fork. No platform package is included.

Contract tests use the pinned `kin-openapi` module to validate actual handler responses against `schemas/api/v1/openapi.yaml`. Report export uses the pinned `jsonschema/v6` module to execute the embedded Draft 2020-12 report schema.

Architecture tests enforce evaluator I/O boundaries, probe import boundaries, forbidden persistence fields, and the absence of runtime plugin loading. Privacy integration tests inject credential, response, header, and path canaries and inspect API/report output plus SQLite database, journal, WAL, and SHM artifacts.
