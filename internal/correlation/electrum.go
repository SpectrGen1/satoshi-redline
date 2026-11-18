// Package correlation contains deterministic first-party cross-component rules.
package correlation

import (
	"time"

	"github.com/satoshi-redline/redline/internal/domain"
)

const (
	indexingRuleID = "correlation.wallet-backend.electrum-indexing-breakpoint"
	protocolRuleID = "correlation.wallet-backend.electrum-protocol-breakpoint"
	upstreamRuleID = "correlation.wallet-backend.upstream-bitcoin-limitation"
	unknownRuleID  = "correlation.wallet-backend.electrum-inconclusive"
	ruleVersion    = "1.0.0"
)

// Electrum correlates findings for every uses_chain_from electrum→bitcoin edge.
func Electrum(scanID string, edges []domain.DependencyEdge, findings []domain.Finding, diagnosedAt time.Time) []domain.Diagnosis {
	var out []domain.Diagnosis
	for _, edge := range edges {
		if edge.Relationship != domain.UsesChainFrom {
			continue
		}
		out = append(out, correlatePair(scanID, edge.SourceComponentID, edge.DestinationComponentID, findings, diagnosedAt)...)
	}
	return out
}

func correlatePair(scanID, electrumID, bitcoinID string, findings []domain.Finding, diagnosedAt time.Time) []domain.Diagnosis {
	byCode := map[string]domain.Finding{}
	for _, finding := range findings {
		if finding.ComponentID != electrumID && finding.ComponentID != bitcoinID {
			continue
		}
		// Index by component+code so multi-instance findings do not collide.
		byCode[finding.ComponentID+"\x00"+finding.Code] = finding
	}
	get := func(componentID, code string) domain.Finding {
		return byCode[componentID+"\x00"+code]
	}

	bitcoinSync := get(bitcoinID, "bitcoin.chain.synchronized")
	bitcoinTip := get(bitcoinID, "bitcoin.chain.tip-freshness")
	electrumReach := get(electrumID, "electrum.protocol.reachable")
	electrumLag := get(electrumID, "electrum.chain.sync-lag")
	electrumMatch := get(electrumID, "electrum.chain.matches-bitcoin")
	electrumTip := get(electrumID, "electrum.chain.tip-freshness")

	bitcoinHealthy := bitcoinSync.Status == domain.Pass && bitcoinTip.Status == domain.Pass
	if bitcoinHealthy && electrumReach.Status == domain.Pass && (electrumLag.Status == domain.Failure || electrumMatch.Status == domain.Failure) {
		return []domain.Diagnosis{newDiagnosis(
			scanID, electrumID, bitcoinID, "wallet-backend.electrum-indexing-breakpoint", domain.DiagnosisBreakPoint,
			domain.ConfidenceHigh, indexingRuleID, diagnosedAt,
			[]domain.Finding{bitcoinSync, bitcoinTip, electrumReach, failing(electrumLag, electrumMatch)},
		)}
	}
	if bitcoinHealthy && electrumReach.Status == domain.Failure {
		return []domain.Diagnosis{newDiagnosis(
			scanID, electrumID, bitcoinID, "wallet-backend.electrum-protocol-breakpoint", domain.DiagnosisBreakPoint,
			domain.ConfidenceHigh, protocolRuleID, diagnosedAt,
			[]domain.Finding{bitcoinSync, bitcoinTip, electrumReach},
		)}
	}
	bitcoinLimited := bitcoinSync.Status == domain.Failure || bitcoinTip.Status == domain.Failure
	electrumLimited := electrumLag.Status == domain.Warning || electrumLag.Status == domain.Failure ||
		electrumTip.Status == domain.Warning || electrumTip.Status == domain.Failure
	if bitcoinLimited && electrumLimited {
		return []domain.Diagnosis{newDiagnosis(
			scanID, electrumID, bitcoinID, "wallet-backend.upstream-bitcoin-limitation", domain.DiagnosisContributingFactor,
			domain.ConfidenceMedium, upstreamRuleID, diagnosedAt,
			[]domain.Finding{failing(bitcoinSync, bitcoinTip), failing(electrumLag, electrumTip)},
		)}
	}

	relevant := []domain.Finding{bitcoinSync, bitcoinTip, electrumReach, electrumLag, electrumMatch, electrumTip}
	var blockers []string
	var present []domain.Finding
	for _, finding := range relevant {
		if finding.ID == "" {
			continue
		}
		present = append(present, finding)
		if finding.Status == domain.Unknown {
			blockers = append(blockers, finding.ID)
		}
	}
	if len(blockers) == 0 || len(present) == 0 {
		return nil
	}
	diagnosis := newDiagnosis(
		scanID, electrumID, bitcoinID, "wallet-backend.electrum-diagnosis-inconclusive", domain.DiagnosisInconclusive,
		domain.ConfidenceLow, unknownRuleID, diagnosedAt, present,
	)
	diagnosis.BlockingUnknownFindingIDs = blockers
	diagnosis.PlausibleAlternativeCodes = []string{
		"wallet-backend.electrum-indexing-breakpoint",
		"wallet-backend.electrum-protocol-breakpoint",
		"wallet-backend.upstream-bitcoin-limitation",
	}
	return []domain.Diagnosis{diagnosis}
}

func failing(first, second domain.Finding) domain.Finding {
	if first.Status == domain.Failure || first.Status == domain.Warning {
		return first
	}
	return second
}

func newDiagnosis(scanID, electrumID, bitcoinID, code string, classification domain.DiagnosisClassification, confidence domain.Confidence, ruleID string, diagnosedAt time.Time, findings []domain.Finding) domain.Diagnosis {
	diagnosis := domain.Diagnosis{
		ID: domain.DiagnosisID(scanID, electrumID, bitcoinID, code), ScanID: scanID, Code: code, Classification: classification,
		Confidence: confidence, SummaryKey: "diagnosis." + code,
		ComponentIDs:      []string{bitcoinID, electrumID},
		CorrelationRuleID: ruleID, CorrelationRuleVersion: ruleVersion, DiagnosedAt: diagnosedAt,
	}
	seenEvidence := map[string]bool{}
	for _, finding := range findings {
		if finding.ID == "" {
			continue
		}
		diagnosis.FindingIDs = append(diagnosis.FindingIDs, finding.ID)
		for _, evidence := range finding.Evidence {
			key := evidence.ObservationID + "\x00" + evidence.ObservationKind + "\x00" + string(evidence.Role)
			if !seenEvidence[key] {
				diagnosis.Evidence = append(diagnosis.Evidence, evidence)
				seenEvidence[key] = true
			}
		}
	}
	return diagnosis
}
