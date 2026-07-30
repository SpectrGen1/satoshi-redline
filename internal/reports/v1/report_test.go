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
	chain := domain.Observation{ID: "chain", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "bitcoin.chain.state", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "configured.bitcoin-rpc", Sensitivity: "none", Value: domain.BitcoinChainState{Blocks: 1, Headers: 1, VerificationProgress: 1, TipTime: n}}
	filesystem := domain.Observation{ID: "filesystem", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "filesystem.capacity", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "filesystem.stat/v1", ProviderID: "configured.data-directory", Sensitivity: "none", Value: domain.FilesystemCapacity{TotalBytes: 10, AvailableBytes: 5, MountPresent: true}}
	clock := domain.Observation{ID: "clock", ScanID: "s", ProbeExecutionID: "p", ComponentID: "system.local", Kind: "system.clock.current", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "clock.read/v1", ProviderID: "system.clock", Sensitivity: "none", Value: domain.ClockState{Current: n}}
	f := domain.Finding{ID: "f", ScanID: "s", Code: "bitcoin.rpc.reachable", ComponentID: "bitcoin.primary", Status: domain.Pass, Applicability: domain.Applicable, Severity: domain.SeverityNone, Confidence: domain.ConfidenceHigh, EvaluatorID: "redline.bitcoin.rpc.reachable", EvaluatorVersion: "1.0.0", EvaluatedAt: n, Role: domain.Required, Evidence: []domain.EvidenceRef{{ObservationID: "o", ObservationKind: "bitcoin.rpc.transport", Role: domain.EvidenceSupports}}}
	return domain.Scan{ID: "s", StartedAt: n, FinishedAt: n, Status: "completed", Executions: []domain.ProbeExecution{p}, Observations: []domain.Observation{o, chain, filesystem, clock}, Findings: []domain.Finding{f}, Summary: domain.ComponentSummary{ComponentID: "bitcoin.primary", Health: domain.Healthy, Coverage: domain.CoverageSummary{State: domain.Complete, ApplicableRequired: 1, AssessedRequired: 1}}}
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

func TestReportRejectsBrokenIntegrityAndVariants(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Report)
	}{
		{"report identity", func(r *Report) { r.ReportID = "other" }},
		{"duplicate execution", func(r *Report) { r.ProbeExecutions = append(r.ProbeExecutions, r.ProbeExecutions[0]) }},
		{"observation execution", func(r *Report) { r.Observations[0].ProbeExecutionID = "missing" }},
		{"duplicate observation", func(r *Report) { r.Observations = append(r.Observations, r.Observations[0]) }},
		{"missing value", func(r *Report) { r.Observations[0].RPCTransport = nil }},
		{"two values", func(r *Report) { r.Observations[0].Filesystem = &FilesystemDTO{10, 5, false, true} }},
		{"mismatched value", func(r *Report) { r.Observations[0].Kind = "filesystem.capacity" }},
		{"duplicate finding", func(r *Report) { r.Findings = append(r.Findings, r.Findings[0]) }},
		{"missing evidence", func(r *Report) { r.Findings[0].Evidence[0].ObservationID = "missing" }},
		{"evidence kind", func(r *Report) { r.Findings[0].Evidence[0].ObservationKind = "bitcoin.chain.state" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := FromDomain(reportScan())
			tt.mutate(&report)
			if err := Validate(report); err == nil {
				t.Fatal("invalid report accepted")
			}
		})
	}
}

func TestReportSchemaEnforcesFindingConditions(t *testing.T) {
	report := FromDomain(reportScan())
	report.Findings[0].Status = "unknown"
	report.Findings[0].Evidence = nil
	if err := Validate(report); err == nil {
		t.Fatal("unknown finding without reason and missing evidence accepted")
	}
	report = FromDomain(reportScan())
	report.Findings[0].Reasons = []ReasonDTO{{Code: "collection.timeout", Stage: "collection", Retryability: "retryable", ResolutionClass: "retry_scan"}}
	if err := Validate(report); err == nil {
		t.Fatal("assessed finding with unknown reason accepted")
	}
}

func TestMarshalRejectsInvalidDomainScan(t *testing.T) {
	scan := reportScan()
	scan.Observations[0].ProbeExecutionID = "missing"
	if _, err := Marshal(scan); err == nil {
		t.Fatal("invalid domain scan exported")
	}
}
