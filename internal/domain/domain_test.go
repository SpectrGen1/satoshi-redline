package domain

import (
	"testing"
	"time"
)

func TestFindingInvariants(t *testing.T) {
	n := time.Now().UTC()
	base := Finding{ID: "f", ScanID: "s", Code: "c", ComponentID: "x", Status: Unknown, Applicability: Applicable, Severity: SeverityNone, Confidence: ConfidenceLow, EvaluatorID: "e", EvaluatorVersion: "1", EvaluatedAt: n, Role: Required}
	if base.Validate() == nil {
		t.Fatal("unknown without reasons accepted")
	}
	base.Reasons = []UnknownReason{Reason(Timeout)}
	base.MissingEvidence = []MissingEvidence{{ObservationKind: "bitcoin.chain.state", Problem: EvidenceMissing, ReasonCode: Timeout}}
	if e := base.Validate(); e != nil {
		t.Fatal(e)
	}
	base.Status = Pass
	base.Reasons = nil
	base.MissingEvidence = nil
	if base.Validate() == nil {
		t.Fatal("pass without evidence accepted")
	}
}
func TestObservationValueValidation(t *testing.T) {
	if (BitcoinChainState{Blocks: 2, Headers: 1, VerificationProgress: 1, TipTime: time.Now()}).Validate() == nil {
		t.Fatal("invalid chain accepted")
	}
	if (FilesystemCapacity{TotalBytes: 1, AvailableBytes: 2}).Validate() == nil {
		t.Fatal("invalid filesystem accepted")
	}
	for _, value := range []RPCTransportResult{
		{Reached: false, Authentication: AuthenticationAccepted, Category: "valid_json_rpc_response"},
		{Reached: false, Authentication: AuthenticationRejectedState, Category: string(AuthenticationRejected)},
		{Reached: true, Authentication: AuthenticationUnknown, Category: ""},
		{Reached: true, Authentication: "maybe", Category: string(InvalidResponse)},
	} {
		if value.Validate() == nil {
			t.Fatalf("invalid transport accepted: %+v", value)
		}
	}
}

func validScan() Scan {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	execution := ProbeExecution{ID: "execution", ScanID: "scan", ProbeID: "probe/v1", ProbeVersion: "1.0.0", Outcome: ExecutionCompleted, StartedAt: now, FinishedAt: now}
	observation := Observation{ID: "observation", ScanID: "scan", ProbeExecutionID: execution.ID, ComponentID: "bitcoin.knots", Kind: "bitcoin.rpc.transport", SchemaVersion: "v1", ObservedAt: now, CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", Value: RPCTransportResult{Reached: true, Authentication: AuthenticationAccepted, Category: "valid_json_rpc_response"}}
	finding := Finding{ID: "finding", ScanID: "scan", Code: "bitcoin.rpc.reachable", ComponentID: "bitcoin.knots", Status: Pass, Applicability: Applicable, Severity: SeverityNone, Confidence: ConfidenceHigh, EvaluatorID: "test", EvaluatorVersion: "1", EvaluatedAt: now, Role: Required, Evidence: []EvidenceRef{{ObservationID: observation.ID, ObservationKind: observation.Kind, Role: EvidenceSupports}}}
	return Scan{ID: "scan", StartedAt: now, FinishedAt: now, Status: "completed", Components: FixtureComponents(false), Executions: []ProbeExecution{execution}, Observations: []Observation{observation}, Findings: []Finding{finding}, Summary: ComponentSummary{ComponentID: ScanAggregateComponentID, Health: Healthy, Coverage: CoverageSummary{State: Complete, ApplicableRequired: 1, AssessedRequired: 1}}}
}

func TestScanReferentialInvariants(t *testing.T) {
	if err := validScan().Validate(); err != nil {
		t.Fatalf("valid fixture rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Scan)
	}{
		{"execution scan", func(s *Scan) { s.Executions[0].ScanID = "other" }},
		{"execution outcome", func(s *Scan) { s.Executions[0].Outcome = "" }},
		{"duplicate component", func(s *Scan) { s.Components = append(s.Components, s.Components[0]) }},
		{"observation component", func(s *Scan) { s.Observations[0].ComponentID = "missing" }},
		{"finding component", func(s *Scan) { s.Findings[0].ComponentID = "missing" }},
		{"observation scan", func(s *Scan) { s.Observations[0].ScanID = "other" }},
		{"observation execution", func(s *Scan) { s.Observations[0].ProbeExecutionID = "missing" }},
		{"observation kind", func(s *Scan) { s.Observations[0].Kind = "filesystem.capacity" }},
		{"finding scan", func(s *Scan) { s.Findings[0].ScanID = "other" }},
		{"evidence missing", func(s *Scan) { s.Findings[0].Evidence[0].ObservationID = "missing" }},
		{"evidence kind", func(s *Scan) { s.Findings[0].Evidence[0].ObservationKind = "bitcoin.chain.state" }},
		{"negative coverage", func(s *Scan) { s.Summary.Coverage.UnknownRequired = -1 }},
		{"coverage total", func(s *Scan) { s.Summary.Coverage.ApplicableRequired = 2 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scan := validScan()
			tt.mutate(&scan)
			if err := scan.Validate(); err == nil {
				t.Fatal("invalid scan accepted")
			}
		})
	}
}

func TestDependencyGraphInvariants(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	scan := validScan()
	scan.Components = FixtureComponents(true)
	scan.DependencyEdges = []DependencyEdge{{
		ID: "electrum.electrs-uses-chain-from-bitcoin.knots", SourceComponentID: "electrum.electrs",
		DestinationComponentID: "bitcoin.knots", Relationship: UsesChainFrom,
		Provenance: ProvenanceConfiguration, Confidence: ConfidenceHigh, RecordedAt: now,
	}}
	if err := scan.Validate(); err != nil {
		t.Fatalf("valid dependency graph rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Scan)
	}{
		{"missing source", func(s *Scan) { s.DependencyEdges[0].SourceComponentID = "missing" }},
		{"missing destination", func(s *Scan) { s.DependencyEdges[0].DestinationComponentID = "missing" }},
		{"invalid relationship", func(s *Scan) { s.DependencyEdges[0].Relationship = "unknown" }},
		{"invalid source kind", func(s *Scan) { s.DependencyEdges[0].SourceComponentID = "system.local" }},
		{"duplicate edge", func(s *Scan) { s.DependencyEdges = append(s.DependencyEdges, s.DependencyEdges[0]) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := scan
			candidate.Components = append([]Component(nil), scan.Components...)
			candidate.DependencyEdges = append([]DependencyEdge(nil), scan.DependencyEdges...)
			tt.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid dependency graph accepted")
			}
		})
	}
}

func TestFindingRejectsIncompleteUnknownAndAssessedUnknownData(t *testing.T) {
	scan := validScan()
	finding := scan.Findings[0]
	finding.Status = Unknown
	finding.Evidence = nil
	finding.Reasons = []UnknownReason{{Code: Timeout}}
	finding.MissingEvidence = []MissingEvidence{{ObservationKind: "required", Problem: EvidenceMissing, ReasonCode: Timeout}}
	if err := finding.Validate(); err == nil {
		t.Fatal("incomplete unknown reason accepted")
	}
	finding = scan.Findings[0]
	finding.Reasons = []UnknownReason{Reason(Timeout)}
	finding.MissingEvidence = []MissingEvidence{{ObservationKind: "bitcoin.chain.state", Problem: EvidenceMissing, ReasonCode: Timeout}}
	if err := finding.Validate(); err == nil {
		t.Fatal("assessed finding with unknown data accepted")
	}
}

func TestDiagnosisGraphInvariants(t *testing.T) {
	scan := validScan()
	scan.Diagnoses = []Diagnosis{{
		ID: "diagnosis", ScanID: scan.ID, Code: "wallet-backend.electrum-protocol-breakpoint",
		Classification: DiagnosisBreakPoint, Confidence: ConfidenceHigh,
		SummaryKey:   "diagnosis.wallet-backend.electrum-protocol-breakpoint",
		ComponentIDs: []string{"bitcoin.knots"}, FindingIDs: []string{"finding"},
		Evidence:          []EvidenceRef{{ObservationID: "observation", ObservationKind: "bitcoin.rpc.transport", Role: EvidenceSupports}},
		CorrelationRuleID: "rule", CorrelationRuleVersion: "1.0.0", DiagnosedAt: scan.FinishedAt,
	}}
	if err := scan.Validate(); err != nil {
		t.Fatalf("valid diagnosis rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Diagnosis)
	}{
		{"missing component", func(d *Diagnosis) { d.ComponentIDs[0] = "missing" }},
		{"missing finding", func(d *Diagnosis) { d.FindingIDs[0] = "missing" }},
		{"missing observation", func(d *Diagnosis) { d.Evidence[0].ObservationID = "missing" }},
		{"evidence kind mismatch", func(d *Diagnosis) { d.Evidence[0].ObservationKind = "bitcoin.chain.state" }},
		{"wrong scan", func(d *Diagnosis) { d.ScanID = "other" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := scan
			candidate.Diagnoses = append([]Diagnosis(nil), scan.Diagnoses...)
			diagnosis := candidate.Diagnoses[0]
			diagnosis.ComponentIDs = append([]string(nil), diagnosis.ComponentIDs...)
			diagnosis.FindingIDs = append([]string(nil), diagnosis.FindingIDs...)
			diagnosis.Evidence = append([]EvidenceRef(nil), diagnosis.Evidence...)
			test.mutate(&diagnosis)
			candidate.Diagnoses[0] = diagnosis
			if err := candidate.Validate(); err == nil {
				t.Fatal("invalid diagnosis graph accepted")
			}
		})
	}
	inconclusive := scan.Diagnoses[0]
	inconclusive.Classification = DiagnosisInconclusive
	if err := inconclusive.Validate(); err == nil {
		t.Fatal("inconclusive diagnosis without blocking unknown accepted")
	}
}
