package probes

import (
	"context"
	"github.com/satoshi-redline/redline/internal/capabilities"
	"github.com/satoshi-redline/redline/internal/domain"
	"time"
)

type FilesystemCapacity struct{}

func (FilesystemCapacity) Descriptor() Descriptor {
	return Descriptor{ID: "filesystem.capacity/v1", Version: "1.0.0", ComponentKinds: []string{"bitcoin-node"}, Required: []capabilities.ID{capabilities.FilesystemStat}, Mode: Safe, Timeout: 5 * time.Second, SideEffects: []SideEffect{ReadOnly}, Sensitivity: SensitiveNone, ObservationKinds: []string{"filesystem.capacity"}}
}
func (FilesystemCapacity) Collect(ctx context.Context, c capabilities.Set, scanID, executionID string, now time.Time) Result {
	if c.Filesystem == nil {
		return Result{Reason: domain.RequiredObservationMissing}
	}
	v, reason := c.Filesystem.Stat(ctx)
	if reason != "" {
		return Result{Reason: reason}
	}
	o := domain.Observation{ID: executionID + "-capacity", ScanID: scanID, ProbeExecutionID: executionID, ComponentID: "bitcoin.primary", Kind: "filesystem.capacity", SchemaVersion: "v1", CapabilityID: string(capabilities.FilesystemStat), ProviderID: "configured.data-directory", Sensitivity: "none", ObservedAt: now, Value: domain.FilesystemCapacity{TotalBytes: v.Total, AvailableBytes: v.Available, ReadOnly: v.ReadOnly, MountPresent: v.MountPresent}}
	if e := o.Validate(); e != nil {
		return Result{Reason: domain.InvalidResponse}
	}
	return Result{Values: []domain.Observation{o}}
}
