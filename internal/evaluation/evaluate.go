package evaluation

import (
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
	"time"
)

func assessed(scan, code string, status domain.Status, severity domain.Severity, now time.Time, obs domain.Observation, thresholds domain.Thresholds) domain.Finding {
	return domain.Finding{ID: scan + "-" + code, ScanID: scan, Code: code, ComponentID: "bitcoin.primary", Status: status, Applicability: domain.Applicable, Severity: severity, Confidence: domain.ConfidenceHigh, EvaluatorID: "redline." + code, EvaluatorVersion: "1.0.0", EvaluatedAt: now, Role: domain.Required, Evidence: []domain.EvidenceRef{{ObservationID: obs.ID, ObservationKind: obs.Kind, Role: domain.EvidenceSupports}}, Thresholds: thresholds}
}
func unknown(scan, code, kind string, reason domain.UnknownReasonCode, now time.Time, thresholds domain.Thresholds) domain.Finding {
	return domain.Finding{ID: scan + "-" + code, ScanID: scan, Code: code, ComponentID: "bitcoin.primary", Status: domain.Unknown, Applicability: domain.Applicable, Severity: domain.SeverityNone, Confidence: domain.ConfidenceLow, EvaluatorID: "redline." + code, EvaluatorVersion: "1.0.0", EvaluatedAt: now, Role: domain.Required, Reasons: []domain.UnknownReason{domain.Reason(reason)}, MissingEvidence: []domain.MissingEvidence{{ObservationKind: kind, Problem: problem(reason), ReasonCode: reason}}, Thresholds: thresholds}
}
func problem(r domain.UnknownReasonCode) domain.EvidenceProblem {
	if r == domain.StaleEvidence {
		return domain.EvidenceStale
	}
	if r == domain.TemporallyInconsistent || r == domain.InvalidResponse {
		return domain.EvidenceUnusable
	}
	return domain.EvidenceMissing
}
func RPC(scan string, obs *domain.Observation, reason domain.UnknownReasonCode, now time.Time) domain.Finding {
	if obs == nil {
		return unknown(scan, "bitcoin.rpc.reachable", "bitcoin.rpc.transport", first(reason, domain.RequiredObservationMissing), now, domain.Thresholds{})
	}
	v, ok := obs.Value.(domain.RPCTransportResult)
	if !ok {
		return unknown(scan, "bitcoin.rpc.reachable", "bitcoin.rpc.transport", domain.InvalidResponse, now, domain.Thresholds{})
	}
	if v.Reached {
		return assessed(scan, "bitcoin.rpc.reachable", domain.Pass, domain.SeverityNone, now, *obs, domain.Thresholds{})
	}
	if reason == domain.ConnectionRefused || reason == domain.RouteUnreachable || reason == domain.ProtocolError {
		return assessed(scan, "bitcoin.rpc.reachable", domain.Failure, domain.SeverityHigh, now, *obs, domain.Thresholds{})
	}
	return unknown(scan, "bitcoin.rpc.reachable", "bitcoin.rpc.transport", first(reason, domain.RequiredObservationMissing), now, domain.Thresholds{})
}
func Chain(scan string, obs *domain.Observation, reason domain.UnknownReasonCode, c app.SyncThresholds, now time.Time) []domain.Finding {
	t := domain.Thresholds{MaxTipAgeSeconds: c.MaxTipAgeSeconds, MinVerificationProgress: c.MinVerificationProgress, MaxHeaderLag: c.MaxHeaderLag}
	u := func(r domain.UnknownReasonCode) []domain.Finding {
		return []domain.Finding{unknown(scan, "bitcoin.chain.synchronized", "bitcoin.chain.state", r, now, t), unknown(scan, "bitcoin.chain.tip-freshness", "bitcoin.chain.state", r, now, t)}
	}
	if obs == nil {
		return u(first(reason, domain.RequiredObservationMissing))
	}
	v, ok := obs.Value.(domain.BitcoinChainState)
	if !ok || v.Validate() != nil {
		return u(domain.InvalidResponse)
	}
	if v.TipTime.After(now.Add(time.Minute)) {
		return u(domain.TemporallyInconsistent)
	}
	if now.Sub(v.TipTime) > time.Duration(c.MaxTipAgeSeconds)*time.Second {
		return []domain.Finding{unknown(scan, "bitcoin.chain.synchronized", "bitcoin.chain.state", domain.StaleEvidence, now, t), assessed(scan, "bitcoin.chain.tip-freshness", domain.Warning, domain.SeverityMedium, now, *obs, t)}
	}
	syncStatus := domain.Pass
	sev := domain.SeverityNone
	if v.InitialBlockDownload || v.Headers-v.Blocks > c.MaxHeaderLag || v.VerificationProgress < c.MinVerificationProgress {
		syncStatus = domain.Failure
		sev = domain.SeverityHigh
	}
	return []domain.Finding{assessed(scan, "bitcoin.chain.synchronized", syncStatus, sev, now, *obs, t), assessed(scan, "bitcoin.chain.tip-freshness", domain.Pass, domain.SeverityNone, now, *obs, t)}
}
func Filesystem(scan string, obs *domain.Observation, reason domain.UnknownReasonCode, c app.FilesystemThresholds, now time.Time) domain.Finding {
	t := domain.Thresholds{WarningAvailablePercent: c.WarningAvailablePercent, FailureAvailablePercent: c.FailureAvailablePercent}
	if obs == nil {
		return unknown(scan, "filesystem.capacity-pressure", "filesystem.capacity", first(reason, domain.RequiredObservationMissing), now, t)
	}
	v, ok := obs.Value.(domain.FilesystemCapacity)
	if !ok || v.Validate() != nil {
		return unknown(scan, "filesystem.capacity-pressure", "filesystem.capacity", domain.InvalidResponse, now, t)
	}
	status, sev := domain.Pass, domain.SeverityNone
	if !v.MountPresent || v.ReadOnly {
		status, sev = domain.Failure, domain.SeverityHigh
	} else {
		pct := float64(v.AvailableBytes) * 100 / float64(v.TotalBytes)
		if pct <= c.FailureAvailablePercent {
			status, sev = domain.Failure, domain.SeverityCritical
		} else if pct <= c.WarningAvailablePercent {
			status, sev = domain.Warning, domain.SeverityMedium
		}
	}
	return assessed(scan, "filesystem.capacity-pressure", status, sev, now, *obs, t)
}
func first(a, b domain.UnknownReasonCode) domain.UnknownReasonCode {
	if a != "" {
		return a
	}
	return b
}
func Summary(fs []domain.Finding) domain.ComponentSummary {
	s := domain.ComponentSummary{ComponentID: "bitcoin.primary"}
	for _, f := range fs {
		if f.Role != domain.Required || f.Applicability != domain.Applicable {
			continue
		}
		s.Coverage.ApplicableRequired++
		if f.Status == domain.Unknown {
			s.Coverage.UnknownRequired++
		} else {
			s.Coverage.AssessedRequired++
		}
		if f.Status == domain.Failure {
			s.Health = domain.HealthFailure
		} else if f.Status == domain.Warning && s.Health != domain.HealthFailure {
			s.Health = domain.HealthWarning
		}
	}
	if s.Coverage.AssessedRequired == 0 {
		s.Coverage.State = domain.Unavailable
	} else if s.Coverage.UnknownRequired == 0 {
		s.Coverage.State = domain.Complete
	} else if s.Coverage.AssessedRequired == 1 && s.Coverage.ApplicableRequired > 2 {
		s.Coverage.State = domain.Minimal
	} else {
		s.Coverage.State = domain.Partial
	}
	if s.Health == "" {
		if s.Coverage.UnknownRequired > 0 {
			s.Health = domain.NoConclusion
		} else {
			s.Health = domain.Healthy
		}
	}
	return s
}
