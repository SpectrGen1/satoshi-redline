package probes

import (
	"context"
	"time"

	"github.com/satoshi-redline/redline/internal/capabilities"
	"github.com/satoshi-redline/redline/internal/domain"
)

type ElectrumProtocolState struct {
	SideEffect SideEffect
	Timeout    time.Duration
}

func (p ElectrumProtocolState) Descriptor() Descriptor {
	sideEffect := p.SideEffect
	if sideEffect == "" {
		sideEffect = LANConnect
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return Descriptor{
		ID: "electrum.protocol-state/v1", Version: "1.0.0",
		ComponentKinds: []string{string(domain.ComponentElectrumBackend)},
		Required:       []capabilities.ID{capabilities.ElectrumRead},
		Mode:           Safe, Timeout: timeout, SideEffects: []SideEffect{sideEffect},
		Sensitivity: "local_sensitive",
		ObservationKinds: []string{
			"electrum.protocol.transport", "electrum.server.identity", "electrum.chain.tip",
		},
	}
}

func (ElectrumProtocolState) Collect(ctx context.Context, capabilitiesSet capabilities.Set, scanID, executionID string, now time.Time) Result {
	if capabilitiesSet.Electrum == nil {
		return Result{Reason: domain.RequiredObservationMissing}
	}
	collected, reason := capabilitiesSet.Electrum.Collect(ctx)
	transport := domain.Observation{
		ID: executionID + "-transport", ScanID: scanID, ProbeExecutionID: executionID,
		ComponentID: "electrum.primary", Kind: "electrum.protocol.transport", SchemaVersion: "v1",
		ObservedAt: now, CapabilityID: string(capabilities.ElectrumRead),
		ProviderID: "configured.electrum-protocol", Sensitivity: "local_sensitive",
		Value: domain.ElectrumProtocolTransport{
			Reached: collected.Transport.Reached, Transport: collected.Transport.Kind,
			TLSVerification:     collected.Transport.TLSVerification,
			ProtocolNegotiation: collected.Transport.ProtocolNegotiation,
			Category:            collected.Transport.Category,
		},
	}
	values := []domain.Observation{transport}
	if collected.Identity != nil && reason == "" {
		values = append(values, domain.Observation{
			ID: executionID + "-identity", ScanID: scanID, ProbeExecutionID: executionID,
			ComponentID: "electrum.primary", Kind: "electrum.server.identity", SchemaVersion: "v1",
			ObservedAt: now, CapabilityID: string(capabilities.ElectrumRead),
			ProviderID: "configured.electrum-protocol", Sensitivity: "local_sensitive",
			Value: domain.ElectrumServerIdentity{
				ImplementationFamily: collected.Identity.Family,
				ProtocolVersion:      collected.Identity.ProtocolVersion,
			},
		})
	}
	if collected.Tip != nil && reason == "" {
		values = append(values, domain.Observation{
			ID: executionID + "-tip", ScanID: scanID, ProbeExecutionID: executionID,
			ComponentID: "electrum.primary", Kind: "electrum.chain.tip", SchemaVersion: "v1",
			ObservedAt: now, CapabilityID: string(capabilities.ElectrumRead),
			ProviderID: "configured.electrum-protocol", Sensitivity: "local_sensitive",
			Value: domain.ElectrumChainTip{
				Height: collected.Tip.Height, BlockHash: collected.Tip.BlockHash, HeaderTime: collected.Tip.HeaderTime,
			},
		})
	}
	for _, observation := range values {
		if err := observation.Validate(); err != nil {
			return Result{Reason: domain.InvalidResponse}
		}
	}
	return Result{Values: values, Reason: reason}
}
