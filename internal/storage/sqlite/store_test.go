package sqlite

import (
	"context"
	"github.com/satoshi-redline/redline/internal/domain"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistenceContainsNormalizedFields(t *testing.T) {
	s, e := Open(filepath.Join(t.TempDir(), "x.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	n := time.Now().UTC()
	x := domain.Scan{ID: "s", StartedAt: n, FinishedAt: n, Status: "completed", Summary: domain.ComponentSummary{Health: domain.Healthy, Coverage: domain.CoverageSummary{State: domain.Complete}}, Findings: []domain.Finding{{ID: "f", ScanID: "s", Code: "bitcoin.rpc.reachable", Status: domain.Pass, Applicability: domain.Applicable, Required: true}}}
	if e = s.Save(context.Background(), x); e != nil {
		t.Fatal(e)
	}
	y, e := s.Get(context.Background(), "s")
	if e != nil || len(y.Findings) != 1 {
		t.Fatalf("%v %+v", e, y)
	}
}
