# MVP implementation note

`internal/capabilities` owns destination-bound RPC and configured-path filesystem access. The engine scopes declared capabilities to compiled probes, enforces probe deadlines, validates sealed typed observations, and passes them to deterministic evaluators. API and report packages map the complete evidence graph into independent v1 transport types.

The scan flow is configured node → scoped capability → compiled probe → typed observation → evaluator → finding → separate health and coverage summaries. One internal execution path performs collection and evaluation. A direct `Scan` emits `scan.completed` after evaluation; the shared `ScanAndPersist` service emits it only after SQLite commits. Every started workflow emits exactly one terminal event (`scan.completed` or `scan.failed`).

SQLite uses strictly numbered, numerically ordered embedded migrations and normalized typed child tables. Load and save validation enforce scan ownership, observation-to-execution references, finding evidence references and kinds, exactly one matching typed value per observation, structured unknown reasons, and coverage consistency. No raw protocol or configuration field exists in the schema.

`POST /api/v1/scans` is synchronous and returns only after persistence. The SSE stream is concurrency-safe but in-memory and non-replayable; subscribers see only events emitted while connected. Rolling per-write deadlines keep events and heartbeats usable beyond the HTTP server’s ordinary write timeout without removing global write protection.

RPC access uses a destination-bound HTTP transport that ignores ambient proxies, rejects redirects, restricts schemes, rejects endpoint userinfo, bounds response bodies, and accepts a cookie-file or environment-backed secret reference. Authentication evidence is `accepted`, `rejected`, or `unknown`. A reached HTTP endpoint can support a transport pass while malformed or rejected authenticated evidence leaves chain findings unknown.

Report v1 projection first validates the domain scan, verifies exported reference integrity and typed variants, then executes the embedded Draft 2020-12 schema before returning bytes. API responses use explicit v1 DTOs and are encoded into a bounded buffer before headers are committed.

Known limitations: one manually configured endpoint, no discovery or correlation diagnoses, Linux filesystem `statfs` only, synchronous scans, and an in-memory non-replayable event stream. Core execution and persistence are local; the configured diagnostic target may be a user-selected LAN or remote endpoint. The daemon defaults to loopback and refuses root on supported Unix systems. This pre-release MVP is not production-ready.

The slice follows accepted ADR-001 through ADR-010: a Go monolith, scoped capabilities, fact/finding separation, unprivileged operation, no raw durable data, versioned contracts, compiled probes, explicit unknown reasons, no outbound product telemetry, and no platform-specific engine fork. No platform package is included.

Contract tests use the pinned `kin-openapi` module to validate actual handler responses against `schemas/api/v1/openapi.yaml`. Report export uses the pinned `jsonschema/v6` module to execute the embedded Draft 2020-12 report schema.

Architecture tests use Go syntax trees and imports to enforce evaluator I/O boundaries, probe boundaries, transport/domain separation, executable-configuration restrictions, explicit DTOs, forbidden persistence fields, and the absence of runtime plugin loading. Privacy tests place distinct canaries on exercised cookie, endpoint, RPC error/body/header, filesystem, internal failure, storage failure, configuration, CLI/daemon, API, report, and SSE paths. Storage tests deliberately create and inspect the SQLite database, a live rollback journal, and live WAL and SHM files.
