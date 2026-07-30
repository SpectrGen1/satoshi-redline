# MVP implementation note

`internal/capabilities` owns destination-bound RPC and configured-path filesystem access. The engine scopes declared capabilities to compiled probes, enforces probe deadlines, validates sealed typed observations, and passes them to deterministic evaluators. API and report packages map the complete evidence graph into independent v1 transport types.

The scan flow is configured node → scoped capability → compiled probe → typed observation → evaluator → finding → separate health and coverage summaries. SQLite uses ordered embedded migrations and normalized typed child tables; no raw protocol/configuration fields exist in the schema.

Known limitations: one manually configured endpoint, no discovery or correlation/diagnoses, Linux filesystem stat only, synchronous API scans, and a minimal one-event SSE progress stream. The slice follows accepted ADR-001 through ADR-010: a Go monolith, scoped capabilities, fact/finding separation, unprivileged operation, no raw durable data, versioned contracts, compiled probes, explicit unknown reasons, local-only networking, and no platform-specific engine fork. No platform package is included.
