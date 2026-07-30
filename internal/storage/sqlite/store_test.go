package sqlite

import (
	"bytes"
	"context"
	"github.com/satoshi-redline/redline/internal/domain"
	"os"
	"path/filepath"
	"testing"
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
	o := domain.Observation{ID: "o", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "bitcoin.rpc.transport", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.RPCTransportResult{Reached: true, Authenticated: true}}
	f := domain.Finding{ID: "f", ScanID: "s", Code: "bitcoin.rpc.reachable", ComponentID: "bitcoin.primary", Status: domain.Pass, Applicability: domain.Applicable, Severity: domain.SeverityNone, Confidence: domain.ConfidenceHigh, EvaluatorID: "test", EvaluatorVersion: "1", EvaluatedAt: n, Role: domain.Required, Evidence: []domain.EvidenceRef{{ObservationID: "o", ObservationKind: "bitcoin.rpc.transport", Role: domain.EvidenceSupports}}}
	x := domain.Scan{ID: "s", StartedAt: n, FinishedAt: n, Status: "completed", Summary: domain.ComponentSummary{Health: domain.Healthy, Coverage: domain.CoverageSummary{State: domain.Complete}}, Executions: []domain.ProbeExecution{ex}, Observations: []domain.Observation{o}, Findings: []domain.Finding{f}}
	if e = s.Save(context.Background(), x); e != nil {
		t.Fatal(e)
	}
	y, e := s.Get(context.Background(), "s")
	if e != nil || len(y.Findings) != 1 {
		t.Fatalf("%v %+v", e, y)
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
		{ID: "rpc", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "bitcoin.rpc.transport", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.RPCTransportResult{Reached: true, Category: "access.authentication_rejected"}},
		{ID: "chain", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "bitcoin.chain.state", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.BitcoinChainState{Blocks: 10, Headers: 10, VerificationProgress: 1, TipTime: n}},
		{ID: "fs", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "filesystem.capacity", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "filesystem.stat/v1", ProviderID: "test", Sensitivity: "none", Value: domain.FilesystemCapacity{TotalBytes: 100, AvailableBytes: 50, MountPresent: true}},
		{ID: "clock", ScanID: "s", ProbeExecutionID: "p", ComponentID: "system.local", Kind: "system.clock.current", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "clock.read/v1", ProviderID: "test", Sensitivity: "none", Value: domain.ClockState{Current: n}},
	}
	reason := domain.Reason(domain.AuthenticationRejected)
	reason.CapabilityID = "bitcoin.rpc.read/v1"
	reason.ProviderID = "test"
	f := domain.Finding{ID: "f", ScanID: "s", Code: "bitcoin.chain.synchronized", ComponentID: "bitcoin.primary", Status: domain.Unknown, Applicability: domain.Applicable, Severity: domain.SeverityNone, Confidence: domain.ConfidenceLow, EvaluatorID: "test", EvaluatorVersion: "1", EvaluatedAt: n, Role: domain.Required, Reasons: []domain.UnknownReason{reason}, MissingEvidence: []domain.MissingEvidence{{ObservationKind: "bitcoin.chain.state", Problem: domain.EvidenceMissing, ReasonCode: domain.AuthenticationRejected}}, Thresholds: domain.Thresholds{MaxTipAgeSeconds: 60}}
	x := domain.Scan{ID: "s", StartedAt: n, FinishedAt: n, Status: "completed", Executions: []domain.ProbeExecution{ex}, Observations: obs, Findings: []domain.Finding{f}, Summary: domain.ComponentSummary{ComponentID: "bitcoin.primary", Health: domain.NoConclusion, Coverage: domain.CoverageSummary{State: domain.Partial, ApplicableRequired: 1, UnknownRequired: 1}}}
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
