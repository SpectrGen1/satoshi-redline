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
	o := domain.Observation{ID: "o", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.knots", Kind: "bitcoin.rpc.transport", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "configured.bitcoin-rpc", Sensitivity: "none", Value: domain.RPCTransportResult{Reached: true, Authentication: domain.AuthenticationAccepted, Category: "valid_json_rpc_response"}}
	chain := domain.Observation{ID: "chain", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.knots", Kind: "bitcoin.chain.state", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "configured.bitcoin-rpc", Sensitivity: "none", Value: domain.BitcoinChainState{Blocks: 1, Headers: 1, VerificationProgress: 1, TipTime: n}}
	filesystem := domain.Observation{ID: "filesystem", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.knots", Kind: "filesystem.capacity", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "filesystem.stat/v1", ProviderID: "configured.data-directory", Sensitivity: "none", Value: domain.FilesystemCapacity{TotalBytes: 10, AvailableBytes: 5, MountPresent: true}}
	clock := domain.Observation{ID: "clock", ScanID: "s", ProbeExecutionID: "p", ComponentID: "system.local", Kind: "system.clock.current", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "clock.read/v1", ProviderID: "system.clock", Sensitivity: "none", Value: domain.ClockState{Current: n}}
	hash := domain.Observation{ID: "hash", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.knots", Kind: "bitcoin.chain.block-hash", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "configured.bitcoin-rpc", Sensitivity: "none", Value: domain.BitcoinBlockHash{Height: 1, BlockHash: "0011111111111111111111111111111111111111111111111111111111111111"}}
	electrumTransport := domain.Observation{ID: "electrum-transport", ScanID: "s", ProbeExecutionID: "p", ComponentID: "electrum.electrs", Kind: "electrum.protocol.transport", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "electrum.protocol.read/v1", ProviderID: "configured.electrum-protocol", Sensitivity: "local_sensitive", Value: domain.ElectrumProtocolTransport{Reached: true, Transport: domain.ElectrumTransportTCP, TLSVerification: domain.TLSNotApplicable, ProtocolNegotiation: domain.ProtocolAccepted, Category: "valid_protocol_response"}}
	electrumIdentity := domain.Observation{ID: "electrum-identity", ScanID: "s", ProbeExecutionID: "p", ComponentID: "electrum.electrs", Kind: "electrum.server.identity", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "electrum.protocol.read/v1", ProviderID: "configured.electrum-protocol", Sensitivity: "local_sensitive", Value: domain.ElectrumServerIdentity{ImplementationFamily: domain.ElectrumElectrs, ProtocolVersion: "1.4.3"}}
	electrumTip := domain.Observation{ID: "electrum-tip", ScanID: "s", ProbeExecutionID: "p", ComponentID: "electrum.electrs", Kind: "electrum.chain.tip", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "electrum.protocol.read/v1", ProviderID: "configured.electrum-protocol", Sensitivity: "local_sensitive", Value: domain.ElectrumChainTip{Height: 1, BlockHash: "0011111111111111111111111111111111111111111111111111111111111111", HeaderTime: n}}
	f := domain.Finding{ID: "f", ScanID: "s", Code: "bitcoin.rpc.reachable", ComponentID: "bitcoin.knots", Status: domain.Pass, Applicability: domain.Applicable, Severity: domain.SeverityNone, Confidence: domain.ConfidenceHigh, EvaluatorID: "redline.bitcoin.rpc.reachable", EvaluatorVersion: "1.0.0", EvaluatedAt: n, Role: domain.Required, Evidence: []domain.EvidenceRef{{ObservationID: "o", ObservationKind: "bitcoin.rpc.transport", Role: domain.EvidenceSupports}}}
	diagnosis := domain.Diagnosis{ID: "diagnosis", ScanID: "s", Code: "wallet-backend.electrum-protocol-breakpoint", Classification: domain.DiagnosisBreakPoint, Confidence: domain.ConfidenceHigh, SummaryKey: "diagnosis.wallet-backend.electrum-protocol-breakpoint", ComponentIDs: []string{"bitcoin.knots", "electrum.electrs"}, FindingIDs: []string{"f"}, Evidence: []domain.EvidenceRef{{ObservationID: "o", ObservationKind: "bitcoin.rpc.transport", Role: domain.EvidenceContextualizes}}, CorrelationRuleID: "correlation.test", CorrelationRuleVersion: "1.0.0", DiagnosedAt: n}
	return domain.Scan{ID: "s", StartedAt: n, FinishedAt: n, Status: "completed", Components: domain.FixtureComponents(true), DependencyEdges: []domain.DependencyEdge{{ID: "edge", SourceComponentID: "electrum.electrs", DestinationComponentID: "bitcoin.knots", Relationship: domain.UsesChainFrom, Provenance: domain.ProvenanceConfiguration, Confidence: domain.ConfidenceHigh, RecordedAt: n}}, Executions: []domain.ProbeExecution{p}, Observations: []domain.Observation{o, chain, hash, filesystem, clock, electrumTransport, electrumIdentity, electrumTip}, Findings: []domain.Finding{f}, Diagnoses: []domain.Diagnosis{diagnosis}, Summary: domain.ComponentSummary{ComponentID: "bitcoin.knots", Health: domain.Healthy, Coverage: domain.CoverageSummary{State: domain.Complete, ApplicableRequired: 1, AssessedRequired: 1}}}
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
		{"edge component", func(r *Report) { r.DependencyEdges[0].SourceComponentID = "missing" }},
		{"diagnosis component", func(r *Report) { r.Diagnoses[0].ComponentIDs[0] = "missing" }},
		{"diagnosis finding", func(r *Report) { r.Diagnoses[0].FindingIDs[0] = "missing" }},
		{"diagnosis evidence", func(r *Report) { r.Diagnoses[0].Evidence[0].ObservationID = "missing" }},
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
