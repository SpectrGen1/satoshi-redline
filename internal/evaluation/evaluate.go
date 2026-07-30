package evaluation

import (
	"fmt"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
	"time"
)

func finding(scan, code string, status domain.Status, reason domain.UnknownReason, provenance string, required bool) domain.Finding {
	f := domain.Finding{ID: scan + "-" + code, ScanID: scan, Code: code, ComponentID: "bitcoin.primary", Status: status, Applicability: domain.Applicable, Evaluator: "redline.mvp/v1", Provenance: provenance, EvaluatedAt: time.Now().UTC(), Required: required}
	if reason != "" {
		f.Reasons = []domain.UnknownReason{reason}
	}
	return f
}
func RPC(scan string, reachable *bool, reason domain.UnknownReason) domain.Finding {
	if reason != "" {
		return finding(scan, "bitcoin.rpc.reachable", domain.Unknown, reason, "", true)
	}
	if reachable == nil {
		return finding(scan, "bitcoin.rpc.reachable", domain.Unknown, domain.RequiredObservationMissing, "", true)
	}
	if !*reachable {
		return finding(scan, "bitcoin.rpc.reachable", domain.Failure, "", "rpc endpoint returned no usable response", true)
	}
	return finding(scan, "bitcoin.rpc.reachable", domain.Pass, "", "approved getblockchaininfo response", true)
}
func Chain(scan string, obs *domain.Observation, reason domain.UnknownReason, c app.SyncThresholds, now time.Time) []domain.Finding {
	p := fmt.Sprintf("max_tip_age_seconds=%d,min_verification_progress=%.6f,max_header_lag=%d", c.MaxTipAgeSeconds, c.MinVerificationProgress, c.MaxHeaderLag)
	unknown := func(r domain.UnknownReason) []domain.Finding {
		return []domain.Finding{finding(scan, "bitcoin.chain.synchronized", domain.Unknown, r, p, true), finding(scan, "bitcoin.chain.tip-freshness", domain.Unknown, r, p, true)}
	}
	if reason != "" || obs == nil || obs.Blocks == nil || obs.Headers == nil || obs.VerificationProgress == nil || obs.InitialBlockDownload == nil || obs.TipTime == nil {
		return unknown(first(reason, domain.RequiredObservationMissing))
	}
	if now.Sub(*obs.TipTime) > time.Duration(c.MaxTipAgeSeconds)*time.Second {
		return []domain.Finding{finding(scan, "bitcoin.chain.synchronized", domain.Unknown, domain.StaleEvidence, p, true), finding(scan, "bitcoin.chain.tip-freshness", domain.Warning, "", p, true)}
	}
	if *obs.InitialBlockDownload || *obs.Headers-*obs.Blocks > c.MaxHeaderLag || *obs.VerificationProgress < c.MinVerificationProgress {
		return []domain.Finding{finding(scan, "bitcoin.chain.synchronized", domain.Failure, "", p, true), finding(scan, "bitcoin.chain.tip-freshness", domain.Pass, "", p, true)}
	}
	return []domain.Finding{finding(scan, "bitcoin.chain.synchronized", domain.Pass, "", p, true), finding(scan, "bitcoin.chain.tip-freshness", domain.Pass, "", p, true)}
}
func Filesystem(scan string, obs *domain.Observation, reason domain.UnknownReason, c app.FilesystemThresholds) domain.Finding {
	p := fmt.Sprintf("warning_available_percent=%.2f,failure_available_percent=%.2f", c.WarningAvailablePercent, c.FailureAvailablePercent)
	if reason != "" || obs == nil || obs.TotalBytes == nil || obs.AvailableBytes == nil || obs.ReadOnly == nil || obs.MountPresent == nil {
		return finding(scan, "filesystem.capacity-pressure", domain.Unknown, first(reason, domain.RequiredObservationMissing), p, true)
	}
	if *obs.ReadOnly {
		return finding(scan, "filesystem.capacity-pressure", domain.Failure, "", p, true)
	}
	percent := float64(*obs.AvailableBytes) * 100 / float64(*obs.TotalBytes)
	if percent <= c.FailureAvailablePercent {
		return finding(scan, "filesystem.capacity-pressure", domain.Failure, "", p, true)
	}
	if percent <= c.WarningAvailablePercent {
		return finding(scan, "filesystem.capacity-pressure", domain.Warning, "", p, true)
	}
	return finding(scan, "filesystem.capacity-pressure", domain.Pass, "", p, true)
}
func first(a, b domain.UnknownReason) domain.UnknownReason {
	if a != "" {
		return a
	}
	return b
}
func Summary(findings []domain.Finding) domain.ComponentSummary {
	s := domain.ComponentSummary{ComponentID: "bitcoin.primary"}
	for _, f := range findings {
		if !f.Required || f.Applicability != domain.Applicable {
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
