package correlation

import (
	"testing"
	"time"

	"github.com/satoshi-redline/redline/internal/domain"
)

func finding(code string, status domain.Status) domain.Finding {
	return domain.Finding{ID: "scan-" + code, ScanID: "scan", Code: code, Status: status}
}

func binding(now time.Time) []domain.DependencyEdge {
	return []domain.DependencyEdge{{
		ID: "edge", SourceComponentID: "electrum.primary", DestinationComponentID: "bitcoin.primary",
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
			diagnoses := Electrum("scan", domain.BaseComponents(true), binding(now), findings, now)
			if len(diagnoses) != 1 || diagnoses[0].Code != test.code || diagnoses[0].Classification != test.classification {
				t.Fatalf("unexpected diagnoses: %+v", diagnoses)
			}
			if diagnoses[0].Classification == domain.DiagnosisRootCause {
				t.Fatal("rule overstated root cause")
			}
		})
	}
}

func TestMaterialUnknownProducesInconclusiveDiagnosis(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	findings := []domain.Finding{
		finding("bitcoin.chain.synchronized", domain.Pass),
		finding("bitcoin.chain.tip-freshness", domain.Pass),
		finding("electrum.protocol.reachable", domain.Pass),
		finding("electrum.chain.sync-lag", domain.Unknown),
		finding("electrum.chain.matches-bitcoin", domain.Unknown),
		finding("electrum.chain.tip-freshness", domain.Pass),
	}
	diagnoses := Electrum("scan", domain.BaseComponents(true), binding(now), findings, now)
	if len(diagnoses) != 1 || diagnoses[0].Classification != domain.DiagnosisInconclusive ||
		len(diagnoses[0].BlockingUnknownFindingIDs) != 2 || len(diagnoses[0].PlausibleAlternativeCodes) == 0 {
		t.Fatalf("material unknown was ignored: %+v", diagnoses)
	}
	if err := diagnoses[0].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestHealthyOrUnboundGraphProducesNoDiagnosis(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	healthy := []domain.Finding{
		finding("bitcoin.chain.synchronized", domain.Pass),
		finding("bitcoin.chain.tip-freshness", domain.Pass),
		finding("electrum.protocol.reachable", domain.Pass),
		finding("electrum.chain.sync-lag", domain.Pass),
		finding("electrum.chain.matches-bitcoin", domain.Pass),
		finding("electrum.chain.tip-freshness", domain.Pass),
	}
	if diagnoses := Electrum("scan", domain.BaseComponents(true), binding(now), healthy, now); len(diagnoses) != 0 {
		t.Fatalf("healthy graph diagnosed: %+v", diagnoses)
	}
	healthy[3].Status = domain.Failure
	if diagnoses := Electrum("scan", domain.BaseComponents(true), nil, healthy, now); len(diagnoses) != 0 {
		t.Fatalf("unbound graph diagnosed: %+v", diagnoses)
	}
}
