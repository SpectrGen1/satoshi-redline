package sqlite

import (
	"context"
	"github.com/satoshi-redline/redline/internal/domain"
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
