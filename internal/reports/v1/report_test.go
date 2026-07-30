package v1

import (
	"bytes"
	"testing"
	"time"

	"github.com/satoshi-redline/redline/internal/domain"
)

func reportScan() domain.Scan {
	n := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	p := domain.ProbeExecution{ID: "p", ScanID: "s", ProbeID: "bitcoin.chain-state/v1", ProbeVersion: "1.0.0", Outcome: domain.ExecutionCompleted, StartedAt: n, FinishedAt: n}
	o := domain.Observation{ID: "o", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "bitcoin.rpc.transport", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "configured.bitcoin-rpc", Sensitivity: "none", Value: domain.RPCTransportResult{Reached: true, Authentication: domain.AuthenticationAccepted, Category: "valid_json_rpc_response"}}
	f := domain.Finding{ID: "f", ScanID: "s", Code: "bitcoin.rpc.reachable", ComponentID: "bitcoin.primary", Status: domain.Pass, Applicability: domain.Applicable, Severity: domain.SeverityNone, Confidence: domain.ConfidenceHigh, EvaluatorID: "redline.bitcoin.rpc.reachable", EvaluatorVersion: "1.0.0", EvaluatedAt: n, Role: domain.Required, Evidence: []domain.EvidenceRef{{ObservationID: "o", ObservationKind: "bitcoin.rpc.transport", Role: domain.EvidenceSupports}}}
	return domain.Scan{ID: "s", StartedAt: n, FinishedAt: n, Status: "completed", Executions: []domain.ProbeExecution{p}, Observations: []domain.Observation{o}, Findings: []domain.Finding{f}, Summary: domain.ComponentSummary{ComponentID: "bitcoin.primary", Health: domain.Healthy, Coverage: domain.CoverageSummary{State: domain.Complete, ApplicableRequired: 1, AssessedRequired: 1}}}
}

func TestReportRunsEmbeddedSchemaValidation(t *testing.T) {
	b, e := Marshal(reportScan())
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(b, []byte("CANARY_SECRET")) {
		t.Fatal("canary leaked")
	}
	r := FromDomain(reportScan())
	r.Format = "wrong"
	if Validate(r) == nil {
		t.Fatal("invalid report accepted")
	}
}
func TestReportRejectsBrokenReference(t *testing.T) {
	r := FromDomain(reportScan())
	r.Findings[0].Evidence[0].ObservationID = "missing"
	if Validate(r) == nil {
		t.Fatal("broken evidence accepted")
	}
}
