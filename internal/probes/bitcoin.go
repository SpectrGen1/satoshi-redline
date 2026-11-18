package probes

import (
	"context"
	"github.com/satoshi-redline/redline/internal/capabilities"
	"github.com/satoshi-redline/redline/internal/domain"
	"time"
)

type BitcoinChainState struct {
	ComponentID string
}

func (BitcoinChainState) Descriptor() Descriptor {
	return Descriptor{ID: "bitcoin.chain-state/v1", Version: "1.0.0", ComponentKinds: []string{"bitcoin-node"}, Required: []capabilities.ID{capabilities.BitcoinRPCRead}, Mode: Safe, Timeout: 5 * time.Second, SideEffects: []SideEffect{ReadOnly}, Sensitivity: SensitiveNone, ObservationKinds: []string{"bitcoin.rpc.transport", "bitcoin.chain.state"}}
}

func (p BitcoinChainState) Collect(ctx context.Context, c capabilities.Set, scanID, executionID string, now time.Time) Result {
	subject := p.ComponentID
	if subject == "" {
		return Result{Reason: domain.RequiredObservationMissing}
	}
	if c.Bitcoin == nil {
		return Result{Reason: domain.RequiredObservationMissing}
	}
	info, reason, reached := c.Bitcoin.Chain(ctx)
	authentication := domain.AuthenticationUnknown
	if reason == "" || reason == domain.RPCError {
		authentication = domain.AuthenticationAccepted
	} else if reason == domain.AuthenticationRejected {
		authentication = domain.AuthenticationRejectedState
	}
	category := string(reason)
	if category == "" {
		category = "valid_json_rpc_response"
	}
	transport := domain.Observation{ID: executionID + "-rpc", ScanID: scanID, ProbeExecutionID: executionID, ComponentID: subject, Kind: "bitcoin.rpc.transport", SchemaVersion: "v1", CapabilityID: string(capabilities.BitcoinRPCRead), ProviderID: "configured.bitcoin-rpc", Sensitivity: "none", ObservedAt: now, Value: domain.RPCTransportResult{Reached: reached, Authentication: authentication, Category: category}}
	if reason != "" {
		return Result{Values: []domain.Observation{transport}, Reason: reason}
	}
	chain := domain.Observation{ID: executionID + "-chain", ScanID: scanID, ProbeExecutionID: executionID, ComponentID: subject, Kind: "bitcoin.chain.state", SchemaVersion: "v1", CapabilityID: string(capabilities.BitcoinRPCRead), ProviderID: "configured.bitcoin-rpc", Sensitivity: "none", ObservedAt: now, Value: domain.BitcoinChainState{Blocks: info.Blocks, Headers: info.Headers, VerificationProgress: info.VerificationProgress, InitialBlockDownload: info.InitialBlockDownload, TipTime: info.TipTime}}
	if e := chain.Validate(); e != nil {
		return Result{Values: []domain.Observation{transport}, Reason: domain.InvalidResponse}
	}
	return Result{Values: []domain.Observation{transport, chain}}
}
