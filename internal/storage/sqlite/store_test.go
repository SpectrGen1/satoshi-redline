package sqlite

import (
	"bytes"
	"context"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/engine"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestPersistenceContainsNormalizedFields(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "x.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	n := time.Now().UTC()
	ex := domain.ProbeExecution{ID: "p", ScanID: "s", ProbeID: "bitcoin.chain-state/v1", ProbeVersion: "1.0.0", Outcome: domain.ExecutionCompleted, StartedAt: n, FinishedAt: n}
	o := domain.Observation{ID: "o", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "bitcoin.rpc.transport", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.RPCTransportResult{Reached: true, Authentication: domain.AuthenticationAccepted, Category: "valid_json_rpc_response"}}
	f := domain.Finding{ID: "f", ScanID: "s", Code: "bitcoin.rpc.reachable", ComponentID: "bitcoin.primary", Status: domain.Pass, Applicability: domain.Applicable, Severity: domain.SeverityNone, Confidence: domain.ConfidenceHigh, EvaluatorID: "test", EvaluatorVersion: "1", EvaluatedAt: n, Role: domain.Required, Evidence: []domain.EvidenceRef{{ObservationID: "o", ObservationKind: "bitcoin.rpc.transport", Role: domain.EvidenceSupports}}}
	x := domain.Scan{ID: "s", StartedAt: n, FinishedAt: n, Status: "completed", Components: domain.BaseComponents(false), Summary: domain.ComponentSummary{ComponentID: "bitcoin.primary", Health: domain.Healthy, Coverage: domain.CoverageSummary{State: domain.Complete, ApplicableRequired: 1, AssessedRequired: 1}}, Executions: []domain.ProbeExecution{ex}, Observations: []domain.Observation{o}, Findings: []domain.Finding{f}}
	if e = s.Save(context.Background(), x); e != nil {
		t.Fatal(e)
	}
	y, e := s.Get(context.Background(), "s")
	if e != nil || len(y.Findings) != 1 {
		t.Fatalf("%v %+v", e, y)
	}
}

func TestComponentGraphRoundTripAndCascade(t *testing.T) {
	store, scan := corruptionFixture(t)
	scan.Components = domain.BaseComponents(true)
	scan.DependencyEdges = []domain.DependencyEdge{{
		ID: "electrum.primary-uses-chain-from-bitcoin.primary", SourceComponentID: "electrum.primary",
		DestinationComponentID: "bitcoin.primary", Relationship: domain.UsesChainFrom,
		Provenance: domain.ProvenanceConfiguration, Confidence: domain.ConfidenceHigh, RecordedAt: scan.StartedAt,
	}}
	if _, err := store.db.Exec(`DELETE FROM scans WHERE id='scan'`); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), scan); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Get(context.Background(), scan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Components) != 4 || len(loaded.DependencyEdges) != 1 {
		t.Fatalf("component graph not restored: %+v", loaded)
	}
	if _, err = store.db.Exec(`DELETE FROM scans WHERE id='scan'`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"components", "dependency_edges"} {
		var count int
		if err = store.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("orphan in %s: %d %v", table, count, err)
		}
	}
}

func TestComponentForeignKeysRejectCrossScanReferences(t *testing.T) {
	store, scan := corruptionFixture(t)
	if _, err := store.db.Exec(`INSERT INTO scans(id,started_at,finished_at,status,health,coverage_state,applicable_required,assessed_required,unknown_required) VALUES(?,?,?,?,?,?,?,?,?)`,
		"other", ts(scan.StartedAt), ts(scan.FinishedAt), "completed", domain.Healthy, domain.Complete, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO components(scan_id,id,kind,implementation_family,implementation_version,display_alias,provenance,inventory_state) VALUES(?,?,?,?,?,?,?,?)`,
		"other", "electrum.primary", domain.ComponentElectrumBackend, domain.ImplementationElectrum, "", "Electrum backend", domain.ProvenanceConfiguration, domain.InventoryConfigured); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO dependency_edges(scan_id,id,source_component_id,destination_component_id,relationship_kind,provenance,confidence,recorded_at) VALUES(?,?,?,?,?,?,?,?)`,
		"scan", "bad", "electrum.primary", "bitcoin.primary", domain.UsesChainFrom, domain.ProvenanceConfiguration, domain.ConfidenceHigh, ts(scan.StartedAt)); err == nil {
		t.Fatal("cross-scan component reference accepted")
	}
}

func TestMigrationDiscoveryUsesNumericStrictOrdering(t *testing.T) {
	source := fstest.MapFS{
		"migrations/010_tenth.sql":  {Data: []byte("ten")},
		"migrations/002_second.sql": {Data: []byte("two")},
	}
	migrations, err := discoverMigrations(source, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].version != 2 || migrations[1].version != 10 {
		t.Fatalf("wrong order: %+v", migrations)
	}
	for name, source := range map[string]fstest.MapFS{
		"duplicate": {
			"migrations/001_first.sql": {Data: []byte("one")},
			"migrations/001_other.sql": {Data: []byte("other")},
		},
		"non sql":        {"migrations/001_note.txt": {Data: []byte("no")}},
		"missing prefix": {"migrations/initial.sql": {Data: []byte("no")}},
		"zero":           {"migrations/000_zero.sql": {Data: []byte("no")}},
		"ambiguous":      {"migrations/01_short.sql": {Data: []byte("no")}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := discoverMigrations(source, "migrations"); err == nil {
				t.Fatal("invalid migration set accepted")
			}
		})
	}
}

func corruptionFixture(t *testing.T) (*Store, domain.Scan) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "corrupt.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	execution := domain.ProbeExecution{ID: "execution", ScanID: "scan", ProbeID: "bitcoin/v1", ProbeVersion: "1.0.0", Outcome: domain.ExecutionCompleted, StartedAt: now, FinishedAt: now}
	rpc := domain.Observation{ID: "rpc", ScanID: "scan", ProbeExecutionID: execution.ID, ComponentID: "bitcoin.primary", Kind: "bitcoin.rpc.transport", SchemaVersion: "v1", ObservedAt: now, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.RPCTransportResult{Reached: true, Authentication: domain.AuthenticationAccepted, Category: "valid_json_rpc_response"}}
	chain := domain.Observation{ID: "chain", ScanID: "scan", ProbeExecutionID: execution.ID, ComponentID: "bitcoin.primary", Kind: "bitcoin.chain.state", SchemaVersion: "v1", ObservedAt: now, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.BitcoinChainState{Blocks: 1, Headers: 1, VerificationProgress: 1, TipTime: now}}
	finding := domain.Finding{ID: "finding", ScanID: "scan", Code: "bitcoin.rpc.reachable", ComponentID: "bitcoin.primary", Status: domain.Pass, Applicability: domain.Applicable, Severity: domain.SeverityNone, Confidence: domain.ConfidenceHigh, EvaluatorID: "test", EvaluatorVersion: "1", EvaluatedAt: now, Role: domain.Required, Evidence: []domain.EvidenceRef{{ObservationID: rpc.ID, ObservationKind: rpc.Kind, Role: domain.EvidenceSupports}}}
	scan := domain.Scan{ID: "scan", StartedAt: now, FinishedAt: now, Status: "completed", Components: domain.BaseComponents(false), Executions: []domain.ProbeExecution{execution}, Observations: []domain.Observation{rpc, chain}, Findings: []domain.Finding{finding}, Summary: domain.ComponentSummary{ComponentID: "bitcoin.primary", Health: domain.Healthy, Coverage: domain.CoverageSummary{State: domain.Complete, ApplicableRequired: 1, AssessedRequired: 1}}}
	if err = store.Save(context.Background(), scan); err != nil {
		t.Fatal(err)
	}
	return store, scan
}

func TestGetRejectsCorruptedEvidenceGraphs(t *testing.T) {
	tests := []struct {
		name string
		sql  string
	}{
		{"invalid timestamp", `UPDATE observation_bitcoin_chain_state SET tip_time='not-a-time' WHERE observation_id='chain'`},
		{"unknown observation kind", `UPDATE observations SET kind='unknown.kind' WHERE id='rpc'`},
		{"missing typed value", `DELETE FROM observation_bitcoin_rpc_transport WHERE observation_id='rpc'`},
		{"more than one typed value", `INSERT INTO observation_filesystem_capacity(observation_id,total_bytes,available_bytes,read_only,mount_present) VALUES('rpc',10,5,0,1)`},
		{"evidence kind mismatch", `UPDATE finding_evidence SET observation_kind='bitcoin.chain.state' WHERE finding_id='finding'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, _ := corruptionFixture(t)
			if _, err := store.db.Exec(tt.sql); err != nil {
				t.Fatal(err)
			}
			if _, err := store.Get(context.Background(), "scan"); err == nil {
				t.Fatal("corrupted graph loaded")
			}
		})
	}
}

func TestGetRejectsMismatchedScanReference(t *testing.T) {
	store, scan := corruptionFixture(t)
	_, err := store.db.Exec(`INSERT INTO scans(id,started_at,finished_at,status,health,coverage_state,applicable_required,assessed_required,unknown_required) VALUES(?,?,?,?,?,?,?,?,?)`,
		"other", ts(scan.StartedAt), ts(scan.FinishedAt), "completed", domain.Healthy, domain.Complete, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec(`UPDATE probe_executions SET scan_id='other' WHERE id='execution'`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Get(context.Background(), "scan"); err == nil {
		t.Fatal("mismatched scan graph loaded")
	}
}

func TestCompleteEvidenceRoundTripAndCascade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "roundtrip.db")
	s, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	n := time.Now().UTC().Truncate(time.Second)
	ex := domain.ProbeExecution{ID: "p", ScanID: "s", ProbeID: "all/v1", ProbeVersion: "1.0.0", Outcome: domain.ExecutionPartial, StartedAt: n, FinishedAt: n, Limitation: domain.AuthenticationRejected}
	obs := []domain.Observation{
		{ID: "rpc", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "bitcoin.rpc.transport", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.RPCTransportResult{Reached: true, Authentication: domain.AuthenticationRejectedState, Category: "access.authentication_rejected"}},
		{ID: "chain", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "bitcoin.chain.state", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.BitcoinChainState{Blocks: 10, Headers: 10, VerificationProgress: 1, TipTime: n}},
		{ID: "fs", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "filesystem.capacity", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "filesystem.stat/v1", ProviderID: "test", Sensitivity: "none", Value: domain.FilesystemCapacity{TotalBytes: 100, AvailableBytes: 50, MountPresent: true}},
		{ID: "clock", ScanID: "s", ProbeExecutionID: "p", ComponentID: "system.local", Kind: "system.clock.current", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "clock.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.ClockState{Current: n}},
	}
	reason := domain.Reason(domain.AuthenticationRejected)
	reason.CapabilityID = "bitcoin.rpc.read/v1"
	reason.ProviderID = "test"
	f := domain.Finding{ID: "f", ScanID: "s", Code: "bitcoin.chain.synchronized", ComponentID: "bitcoin.primary", Status: domain.Unknown, Applicability: domain.Applicable, Severity: domain.SeverityNone, Confidence: domain.ConfidenceLow, EvaluatorID: "test", EvaluatorVersion: "1", EvaluatedAt: n, Role: domain.Required, Reasons: []domain.UnknownReason{reason}, MissingEvidence: []domain.MissingEvidence{{ObservationKind: "bitcoin.chain.state", Problem: domain.EvidenceMissing, ReasonCode: domain.AuthenticationRejected}}, Thresholds: domain.Thresholds{MaxTipAgeSeconds: 60}}
	x := domain.Scan{ID: "s", StartedAt: n, FinishedAt: n, Status: "completed", Components: domain.BaseComponents(false), Executions: []domain.ProbeExecution{ex}, Observations: obs, Findings: []domain.Finding{f}, Summary: domain.ComponentSummary{ComponentID: "bitcoin.primary", Health: domain.NoConclusion, Coverage: domain.CoverageSummary{State: domain.Unavailable, ApplicableRequired: 1, UnknownRequired: 1}}}
	if e = s.Save(context.Background(), x); e != nil {
		t.Fatal(e)
	}
	y, e := s.Get(context.Background(), "s")
	if e != nil {
		t.Fatal(e)
	}
	if len(y.Observations) != 4 || len(y.Executions) != 1 || len(y.Findings) != 1 || len(y.Findings[0].Reasons) != 1 || len(y.Findings[0].MissingEvidence) != 1 {
		t.Fatalf("incomplete round trip: %+v", y)
	}
	if _, e = s.db.Exec(`DELETE FROM scans WHERE id='s'`); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"probe_executions", "observations", "findings", "finding_unknown_reasons"} {
		var count int
		if e = s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); e != nil || count != 0 {
			t.Fatalf("orphan in %s: %d %v", table, count, e)
		}
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(data, []byte("CANARY_SECRET")) {
		t.Fatal("canary persisted")
	}
}

func TestCanaryAbsentFromActualWALSHMAndRollbackJournal(t *testing.T) {
	const canary = "CANARY_FILESYSTEM_PATH_d720"
	root := t.TempDir()
	dataDirectory := filepath.Join(root, canary)
	if err := os.Mkdir(dataDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	databasePath := filepath.Join(root, "privacy.db")
	store, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.db.SetMaxOpenConns(1)
	if _, err = store.db.Exec(`PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatal(err)
	}
	config := app.DefaultConfig()
	config.Bitcoin.DataDirectory = dataDirectory
	if _, err = (engine.Runner{Config: config}).ScanAndPersist(context.Background(), store); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		path := databasePath + suffix
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("%s was not created: %v", suffix, readErr)
		}
		if bytes.Contains(body, []byte(canary)) {
			t.Fatalf("canary entered %s", suffix)
		}
	}
	if _, err = store.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec(`PRAGMA journal_mode=DELETE`); err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE scans SET status='running'`); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	journalPath := databasePath + "-journal"
	journal, err := os.ReadFile(journalPath)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("rollback journal was not created: %v", err)
	}
	if bytes.Contains(journal, []byte(canary)) {
		_ = tx.Rollback()
		t.Fatal("canary entered rollback journal")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}
