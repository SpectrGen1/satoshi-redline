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
	o := &domain.Observation{ID: "o", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "bitcoin.chain.state", SchemaVersion: "v1", CapabilityID: "bitcoin.rpc.read/v1", ProviderID: "test", Sensitivity: "none", ObservedAt: n, Value: domain.BitcoinChainState{Blocks: b, Headers: h, VerificationProgress: p, InitialBlockDownload: ibd, TipTime: tip}}
	f := Chain("s", o, "", app.DefaultConfig().Sync, n)
	if f[0].Status != domain.Unknown || f[0].Reasons[0].Code != domain.TemporallyInconsistent {
		t.Fatalf("%+v", f[0])
	}
}

func TestSummaryPreservesFailureWithUnknown(t *testing.T) {
	s := Summary([]domain.Finding{{Status: domain.Failure, Applicability: domain.Applicable, Role: domain.Required}, {Status: domain.Unknown, Applicability: domain.Applicable, Role: domain.Required, Reasons: []domain.UnknownReason{domain.Reason(domain.Timeout)}}})
	if s.Health != domain.HealthFailure || s.Coverage.State != domain.Partial {
		t.Fatalf("got %+v", s)
	}
}

func TestFilesystemBoundariesAndInvalidValues(t *testing.T) {
	n := time.Now().UTC()
	cfg := app.DefaultConfig().Filesystem
	makeObs := func(v domain.FilesystemCapacity) *domain.Observation {
		return &domain.Observation{ID: "o", ScanID: "s", ProbeExecutionID: "p", ComponentID: "bitcoin.primary", Kind: "filesystem.capacity", SchemaVersion: "v1", ObservedAt: n, CapabilityID: "filesystem.stat/v1", ProviderID: "test", Sensitivity: "none", Value: v}
	}
	tests := []struct {
		name string
		v    domain.FilesystemCapacity
		want domain.Status
	}{{"healthy", domain.FilesystemCapacity{TotalBytes: 100, AvailableBytes: 50, MountPresent: true}, domain.Pass}, {"warning", domain.FilesystemCapacity{TotalBytes: 100, AvailableBytes: 15, MountPresent: true}, domain.Warning}, {"failure", domain.FilesystemCapacity{TotalBytes: 100, AvailableBytes: 5, MountPresent: true}, domain.Failure}, {"read only", domain.FilesystemCapacity{TotalBytes: 100, AvailableBytes: 50, MountPresent: true, ReadOnly: true}, domain.Failure}, {"zero total", domain.FilesystemCapacity{MountPresent: true}, domain.Unknown}, {"available exceeds total", domain.FilesystemCapacity{TotalBytes: 100, AvailableBytes: 101, MountPresent: true}, domain.Unknown}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Filesystem("s", makeObs(tt.v), "", cfg, n); got.Status != tt.want {
				t.Fatalf("got %s", got.Status)
			}
		})
	}
}
