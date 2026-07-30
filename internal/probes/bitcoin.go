package probes

import (
	"context"
	"github.com/satoshi-redline/redline/internal/capabilities"
	"github.com/satoshi-redline/redline/internal/domain"
	"time"
)

type BitcoinChainState struct{}

func (BitcoinChainState) Descriptor() Descriptor {
	return Descriptor{ID: "bitcoin.chain-state/v1", Version: "1.0.0", ComponentKinds: []string{"bitcoin-node"}, Required: []capabilities.ID{capabilities.BitcoinRPCRead}, Mode: Safe, Timeout: 5 * time.Second, SideEffects: []SideEffect{ReadOnly}, Sensitivity: SensitiveNone, ObservationKinds: []string{"bitcoin.rpc.transport", "bitcoin.chain.state"}}
}
func (BitcoinChainState) Collect(ctx context.Context, c capabilities.Set, scanID, executionID string, now time.Time) Result {
	if c.Bitcoin == nil {
		return Result{Reason: domain.RequiredObservationMissing}
	}
	info, reason, reached := c.Bitcoin.Chain(ctx)
	transport := domain.Observation{ID: executionID + "-rpc", ScanID: scanID, ProbeExecutionID: executionID, ComponentID: "bitcoin.primary", Kind: "bitcoin.rpc.transport", SchemaVersion: "v1", CapabilityID: string(capabilities.BitcoinRPCRead), ProviderID: "configured.bitcoin-rpc", Sensitivity: "none", ObservedAt: now, Value: domain.RPCTransportResult{Reached: reached, Authenticated: reason == "", Category: string(reason)}}
	if reason != "" {
		return Result{Values: []domain.Observation{transport}, Reason: reason}
	}
	chain := domain.Observation{ID: executionID + "-chain", ScanID: scanID, ProbeExecutionID: executionID, ComponentID: "bitcoin.primary", Kind: "bitcoin.chain.state", SchemaVersion: "v1", CapabilityID: string(capabilities.BitcoinRPCRead), ProviderID: "configured.bitcoin-rpc", Sensitivity: "none", ObservedAt: now, Value: domain.BitcoinChainState{Blocks: info.Blocks, Headers: info.Headers, VerificationProgress: info.VerificationProgress, InitialBlockDownload: info.InitialBlockDownload, TipTime: info.TipTime}}
	if e := chain.Validate(); e != nil {
		return Result{Values: []domain.Observation{transport}, Reason: domain.InvalidResponse}
	}
	return Result{Values: []domain.Observation{transport, chain}}
}
