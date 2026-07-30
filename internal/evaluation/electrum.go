package evaluation

import (
	"time"

	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
)

type ElectrumEvidence struct {
	Transport   *domain.Observation
	Identity    *domain.Observation
	Tip         *domain.Observation
	Bitcoin     *domain.Observation
	BitcoinHash *domain.Observation
	Limitation  domain.UnknownReasonCode
}

func Electrum(scan string, evidence ElectrumEvidence, config app.ElectrumConfig, now time.Time) []domain.Finding {
	thresholds := domain.Thresholds{
		ElectrumTipWarningAgeSeconds: config.TipWarningAgeSeconds,
		ElectrumTipFailureAgeSeconds: config.TipFailureAgeSeconds,
		ElectrumLagWarningBlocks:     config.LagWarningBlocks,
		ElectrumLagFailureBlocks:     config.LagFailureBlocks,
		MaxObservationSkewSeconds:    config.MaxObservationSkewSeconds,
		MaxObservationAgeSeconds:     config.MaxObservationAgeSeconds,
		MaxFutureSkewSeconds:         config.MaxFutureSkewSeconds,
	}
	return []domain.Finding{
		electrumReachability(scan, evidence.Transport, evidence.Limitation, now, thresholds),
		electrumCompatibility(scan, evidence.Transport, evidence.Identity, evidence.Limitation, now, thresholds),
		electrumTipFreshness(scan, evidence.Tip, evidence.Limitation, config, now, thresholds),
		electrumSyncLag(scan, evidence.Tip, evidence.Bitcoin, evidence.Limitation, config, now, thresholds),
		electrumChainMatch(scan, evidence.Tip, evidence.BitcoinHash, evidence.Limitation, now, thresholds),
	}
}

func assessedElectrum(scan, code string, status domain.Status, severity domain.Severity, now time.Time, thresholds domain.Thresholds, observations ...domain.Observation) domain.Finding {
	finding := domain.Finding{
		ID: scan + "-" + code, ScanID: scan, Code: code, ComponentID: "electrum.primary",
		Status: status, Applicability: domain.Applicable, Severity: severity, Confidence: domain.ConfidenceHigh,
		EvaluatorID: "redline." + code, EvaluatorVersion: "1.0.0", EvaluatedAt: now,
		Role: domain.Required, Thresholds: thresholds,
	}
	for _, observation := range observations {
		finding.Evidence = append(finding.Evidence, domain.EvidenceRef{
			ObservationID: observation.ID, ObservationKind: observation.Kind, Role: domain.EvidenceSupports,
		})
	}
	return finding
}

func unknownElectrum(scan, code, kind string, reason domain.UnknownReasonCode, now time.Time, thresholds domain.Thresholds) domain.Finding {
	if reason == "" {
		reason = domain.RequiredObservationMissing
	}
	return domain.Finding{
		ID: scan + "-" + code, ScanID: scan, Code: code, ComponentID: "electrum.primary",
		Status: domain.Unknown, Applicability: domain.Applicable, Severity: domain.SeverityNone,
		Confidence: domain.ConfidenceLow, EvaluatorID: "redline." + code, EvaluatorVersion: "1.0.0",
		EvaluatedAt: now, Role: domain.Required, Thresholds: thresholds,
		Reasons: []domain.UnknownReason{domain.Reason(reason)},
		MissingEvidence: []domain.MissingEvidence{{
			ObservationKind: kind, Problem: problem(reason), ReasonCode: reason,
		}},
	}
}

func electrumReachability(scan string, observation *domain.Observation, limitation domain.UnknownReasonCode, now time.Time, thresholds domain.Thresholds) domain.Finding {
	if observation == nil {
		return unknownElectrum(scan, "electrum.protocol.reachable", "electrum.protocol.transport", limitation, now, thresholds)
	}
	value, ok := observation.Value.(domain.ElectrumProtocolTransport)
	if !ok || value.Validate() != nil {
		return unknownElectrum(scan, "electrum.protocol.reachable", "electrum.protocol.transport", domain.InvalidResponse, now, thresholds)
	}
	if value.Reached {
		return assessedElectrum(scan, "electrum.protocol.reachable", domain.Pass, domain.SeverityNone, now, thresholds, *observation)
	}
	switch domain.UnknownReasonCode(value.Category) {
	case domain.ConnectionRefused, domain.RouteUnreachable:
		return assessedElectrum(scan, "electrum.protocol.reachable", domain.Failure, domain.SeverityHigh, now, thresholds, *observation)
	default:
		return unknownElectrum(scan, "electrum.protocol.reachable", "electrum.protocol.transport", first(limitation, domain.UnknownReasonCode(value.Category)), now, thresholds)
	}
}

func electrumCompatibility(scan string, transportObservation, identityObservation *domain.Observation, limitation domain.UnknownReasonCode, now time.Time, thresholds domain.Thresholds) domain.Finding {
	if transportObservation == nil {
		return unknownElectrum(scan, "electrum.protocol.compatible", "electrum.server.identity", limitation, now, thresholds)
	}
	transport, ok := transportObservation.Value.(domain.ElectrumProtocolTransport)
	if !ok || transport.Validate() != nil {
		return unknownElectrum(scan, "electrum.protocol.compatible", "electrum.server.identity", domain.InvalidResponse, now, thresholds)
	}
	if transport.ProtocolNegotiation == domain.ProtocolUnsupported {
		return assessedElectrum(scan, "electrum.protocol.compatible", domain.Failure, domain.SeverityHigh, now, thresholds, *transportObservation)
	}
	if transport.ProtocolNegotiation != domain.ProtocolAccepted || identityObservation == nil {
		return unknownElectrum(scan, "electrum.protocol.compatible", "electrum.server.identity", first(limitation, domain.UnknownReasonCode(transport.Category)), now, thresholds)
	}
	identity, ok := identityObservation.Value.(domain.ElectrumServerIdentity)
	if !ok || identity.Validate() != nil {
		return unknownElectrum(scan, "electrum.protocol.compatible", "electrum.server.identity", domain.InvalidResponse, now, thresholds)
	}
	return assessedElectrum(scan, "electrum.protocol.compatible", domain.Pass, domain.SeverityNone, now, thresholds, *transportObservation, *identityObservation)
}

func electrumTipFreshness(scan string, observation *domain.Observation, limitation domain.UnknownReasonCode, config app.ElectrumConfig, now time.Time, thresholds domain.Thresholds) domain.Finding {
	if observation == nil {
		return unknownElectrum(scan, "electrum.chain.tip-freshness", "electrum.chain.tip", limitation, now, thresholds)
	}
	value, ok := observation.Value.(domain.ElectrumChainTip)
	if !ok || value.Validate() != nil {
		return unknownElectrum(scan, "electrum.chain.tip-freshness", "electrum.chain.tip", domain.InvalidResponse, now, thresholds)
	}
	if reason := temporalValidity(*observation, now, config); reason != "" {
		return unknownElectrum(scan, "electrum.chain.tip-freshness", "electrum.chain.tip", reason, now, thresholds)
	}
	if value.HeaderTime.After(now.Add(time.Duration(config.MaxFutureSkewSeconds) * time.Second)) {
		return unknownElectrum(scan, "electrum.chain.tip-freshness", "electrum.chain.tip", domain.TemporallyInconsistent, now, thresholds)
	}
	age := now.Sub(value.HeaderTime)
	if age > time.Duration(config.TipFailureAgeSeconds)*time.Second {
		return assessedElectrum(scan, "electrum.chain.tip-freshness", domain.Failure, domain.SeverityHigh, now, thresholds, *observation)
	}
	if age > time.Duration(config.TipWarningAgeSeconds)*time.Second {
		return assessedElectrum(scan, "electrum.chain.tip-freshness", domain.Warning, domain.SeverityMedium, now, thresholds, *observation)
	}
	return assessedElectrum(scan, "electrum.chain.tip-freshness", domain.Pass, domain.SeverityNone, now, thresholds, *observation)
}

func electrumSyncLag(scan string, tipObservation, bitcoinObservation *domain.Observation, limitation domain.UnknownReasonCode, config app.ElectrumConfig, now time.Time, thresholds domain.Thresholds) domain.Finding {
	if tipObservation == nil {
		return unknownElectrum(scan, "electrum.chain.sync-lag", "electrum.chain.tip", limitation, now, thresholds)
	}
	if bitcoinObservation == nil {
		return unknownElectrum(scan, "electrum.chain.sync-lag", "bitcoin.chain.state", first(limitation, domain.RequiredObservationMissing), now, thresholds)
	}
	tip, tipOK := tipObservation.Value.(domain.ElectrumChainTip)
	bitcoin, bitcoinOK := bitcoinObservation.Value.(domain.BitcoinChainState)
	if !tipOK || !bitcoinOK || tip.Validate() != nil || bitcoin.Validate() != nil {
		return unknownElectrum(scan, "electrum.chain.sync-lag", "bitcoin.chain.state", domain.InvalidResponse, now, thresholds)
	}
	if reason := temporalValidity(*tipObservation, now, config); reason != "" {
		return unknownElectrum(scan, "electrum.chain.sync-lag", "electrum.chain.tip", reason, now, thresholds)
	}
	if reason := temporalValidity(*bitcoinObservation, now, config); reason != "" {
		return unknownElectrum(scan, "electrum.chain.sync-lag", "bitcoin.chain.state", reason, now, thresholds)
	}
	skew := tipObservation.ObservedAt.Sub(bitcoinObservation.ObservedAt)
	if skew < 0 {
		skew = -skew
	}
	if skew > time.Duration(config.MaxObservationSkewSeconds)*time.Second || tip.Height > bitcoin.Blocks {
		return unknownElectrum(scan, "electrum.chain.sync-lag", "bitcoin.chain.state", domain.TemporallyInconsistent, now, thresholds)
	}
	lag := bitcoin.Blocks - tip.Height
	switch {
	case lag >= config.LagFailureBlocks:
		return assessedElectrum(scan, "electrum.chain.sync-lag", domain.Failure, domain.SeverityHigh, now, thresholds, *tipObservation, *bitcoinObservation)
	case lag >= config.LagWarningBlocks:
		return assessedElectrum(scan, "electrum.chain.sync-lag", domain.Warning, domain.SeverityMedium, now, thresholds, *tipObservation, *bitcoinObservation)
	default:
		return assessedElectrum(scan, "electrum.chain.sync-lag", domain.Pass, domain.SeverityNone, now, thresholds, *tipObservation, *bitcoinObservation)
	}
}

func electrumChainMatch(scan string, tipObservation, hashObservation *domain.Observation, limitation domain.UnknownReasonCode, now time.Time, thresholds domain.Thresholds) domain.Finding {
	if tipObservation == nil {
		return unknownElectrum(scan, "electrum.chain.matches-bitcoin", "electrum.chain.tip", limitation, now, thresholds)
	}
	if hashObservation == nil {
		return unknownElectrum(scan, "electrum.chain.matches-bitcoin", "bitcoin.chain.block-hash", first(limitation, domain.RequiredObservationMissing), now, thresholds)
	}
	tip, tipOK := tipObservation.Value.(domain.ElectrumChainTip)
	hash, hashOK := hashObservation.Value.(domain.BitcoinBlockHash)
	if !tipOK || !hashOK || tip.Validate() != nil || hash.Validate() != nil || tip.Height != hash.Height {
		return unknownElectrum(scan, "electrum.chain.matches-bitcoin", "bitcoin.chain.block-hash", domain.InvalidResponse, now, thresholds)
	}
	status, severity := domain.Pass, domain.SeverityNone
	if tip.BlockHash != hash.BlockHash {
		status, severity = domain.Failure, domain.SeverityCritical
	}
	return assessedElectrum(scan, "electrum.chain.matches-bitcoin", status, severity, now, thresholds, *tipObservation, *hashObservation)
}

func temporalValidity(observation domain.Observation, now time.Time, config app.ElectrumConfig) domain.UnknownReasonCode {
	if observation.ObservedAt.After(now.Add(time.Duration(config.MaxFutureSkewSeconds) * time.Second)) {
		return domain.TemporallyInconsistent
	}
	if now.Sub(observation.ObservedAt) > time.Duration(config.MaxObservationAgeSeconds)*time.Second {
		return domain.StaleEvidence
	}
	return ""
}
