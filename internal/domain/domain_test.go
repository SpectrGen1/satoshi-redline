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
}
