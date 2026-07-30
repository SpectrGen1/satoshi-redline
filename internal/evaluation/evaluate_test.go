package evaluation

import (
	"github.com/satoshi-redline/redline/internal/domain"
	"testing"
)

func TestSummaryPreservesFailureWithUnknown(t *testing.T) {
	s := Summary([]domain.Finding{{Status: domain.Failure, Applicability: domain.Applicable, Required: true}, {Status: domain.Unknown, Applicability: domain.Applicable, Required: true, Reasons: []domain.UnknownReason{domain.Timeout}}})
	if s.Health != domain.HealthFailure || s.Coverage.State != domain.Partial {
		t.Fatalf("got %+v", s)
	}
}
