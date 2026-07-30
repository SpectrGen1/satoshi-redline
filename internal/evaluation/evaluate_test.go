package evaluation

import (
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/domain"
	"testing"
	"time"
)

func TestChainFutureTipIsUnknown(t *testing.T) {
	n := time.Now().UTC()
	b, h := int64(1), int64(1)
	p := 1.0
	ibd := false
	tip := n.Add(2 * time.Minute)
	o := &domain.Observation{Blocks: &b, Headers: &h, VerificationProgress: &p, InitialBlockDownload: &ibd, TipTime: &tip}
	f := Chain("s", o, "", app.DefaultConfig().Sync, n)
	if f[0].Status != domain.Unknown || f[0].Reasons[0] != domain.TemporallyInconsistent {
		t.Fatalf("%+v", f[0])
	}
}

func TestSummaryPreservesFailureWithUnknown(t *testing.T) {
	s := Summary([]domain.Finding{{Status: domain.Failure, Applicability: domain.Applicable, Required: true}, {Status: domain.Unknown, Applicability: domain.Applicable, Required: true, Reasons: []domain.UnknownReason{domain.Timeout}}})
	if s.Health != domain.HealthFailure || s.Coverage.State != domain.Partial {
		t.Fatalf("got %+v", s)
	}
}
