package correlation

import (
	"testing"
	"time"

	"github.com/satoshi-redline/redline/internal/domain"
)

func finding(code string, status domain.Status) domain.Finding {
	componentID := "bitcoin.knots"
	if len(code) >= 8 && code[:8] == "electrum" {
		componentID = "electrum.electrs"
	}
	return domain.Finding{
		ID: domain.FindingID("scan", componentID, code), ScanID: "scan", Code: code,
		ComponentID: componentID, Status: status,
	}
}

func binding(now time.Time) []domain.DependencyEdge {
	return []domain.DependencyEdge{{
		ID: "edge", SourceComponentID: "electrum.electrs", DestinationComponentID: "bitcoin.knots",
		Relationship: domain.UsesChainFrom, Provenance: domain.ProvenanceConfiguration,
		Confidence: domain.ConfidenceHigh, RecordedAt: now,
	}}
}

func TestElectrumCorrelationRules(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	base := []domain.Finding{
		finding("bitcoin.chain.synchronized", domain.Pass),
		finding("bitcoin.chain.tip-freshness", domain.Pass),
		finding("electrum.protocol.reachable", domain.Pass),
		finding("electrum.chain.sync-lag", domain.Pass),
		finding("electrum.chain.matches-bitcoin", domain.Pass),
		finding("electrum.chain.tip-freshness", domain.Pass),
	}
	tests := []struct {
		name           string
		mutate         func([]domain.Finding)
		code           string
		classification domain.DiagnosisClassification
	}{
		{"indexing lag", func(findings []domain.Finding) { findings[3].Status = domain.Failure }, "wallet-backend.electrum-indexing-breakpoint", domain.DiagnosisBreakPoint},
		{"chain mismatch", func(findings []domain.Finding) { findings[4].Status = domain.Failure }, "wallet-backend.electrum-indexing-breakpoint", domain.DiagnosisBreakPoint},
		{"protocol", func(findings []domain.Finding) { findings[2].Status = domain.Failure }, "wallet-backend.electrum-protocol-breakpoint", domain.DiagnosisBreakPoint},
		{"upstream", func(findings []domain.Finding) {
			findings[0].Status = domain.Failure
			findings[3].Status = domain.Failure
		}, "wallet-backend.upstream-bitcoin-limitation", domain.DiagnosisContributingFactor},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			findings := append([]domain.Finding(nil), base...)
			test.mutate(findings)
			diagnoses := Electrum("scan", binding(now), findings, now)
			if len(diagnoses) != 1 || diagnoses[0].Code != test.code || diagnoses[0].Classification != test.classification {
				t.Fatalf("unexpected diagnoses: %+v", diagnoses)
			}
			if diagnoses[0].ComponentIDs[0] != "bitcoin.knots" || diagnoses[0].ComponentIDs[1] != "electrum.electrs" {
				t.Fatalf("unexpected component ids: %v", diagnoses[0].ComponentIDs)
			}
		})
	}
}

func TestElectrumInconclusiveWhenUnknownBlocksDiagnosis(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	findings := []domain.Finding{
		finding("bitcoin.chain.synchronized", domain.Pass),
		finding("bitcoin.chain.tip-freshness", domain.Pass),
		finding("electrum.protocol.reachable", domain.Pass),
		finding("electrum.chain.sync-lag", domain.Unknown),
		finding("electrum.chain.matches-bitcoin", domain.Unknown),
		finding("electrum.chain.tip-freshness", domain.Pass),
	}
	diagnoses := Electrum("scan", binding(now), findings, now)
	if len(diagnoses) != 1 || diagnoses[0].Classification != domain.DiagnosisInconclusive {
		t.Fatalf("expected inconclusive diagnosis, got %+v", diagnoses)
	}
	if len(diagnoses[0].BlockingUnknownFindingIDs) == 0 {
		t.Fatal("expected blocking unknown finding ids")
	}
}

func TestElectrumSkipsWithoutBindingOrHealthyPath(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	findings := []domain.Finding{
		finding("bitcoin.chain.synchronized", domain.Pass),
		finding("bitcoin.chain.tip-freshness", domain.Pass),
		finding("electrum.protocol.reachable", domain.Pass),
		finding("electrum.chain.sync-lag", domain.Pass),
		finding("electrum.chain.matches-bitcoin", domain.Pass),
		finding("electrum.chain.tip-freshness", domain.Pass),
	}
	if diagnoses := Electrum("scan", nil, findings, now); len(diagnoses) != 0 {
		t.Fatalf("expected no diagnoses without edge, got %+v", diagnoses)
	}
	if diagnoses := Electrum("scan", binding(now), findings, now); len(diagnoses) != 0 {
		t.Fatalf("expected no diagnoses on healthy path, got %+v", diagnoses)
	}
}
