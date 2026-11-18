package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/capabilities"
	"github.com/satoshi-redline/redline/internal/capabilities/bitcoinrpc"
	"github.com/satoshi-redline/redline/internal/capabilities/electrum"
	"github.com/satoshi-redline/redline/internal/capabilities/filesystem"
	"github.com/satoshi-redline/redline/internal/correlation"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/evaluation"
	"github.com/satoshi-redline/redline/internal/probes"
)

type Runner struct {
	Config app.Config
	Now    func() time.Time
	Events EventSink
}
type Event struct {
	ID, Type, ScanID                             string
	OccurredAt                                   time.Time
	ProbeID, FindingCode, DiagnosisCode, Outcome string
}
type EventSink interface{ Emit(Event) }

func (r Runner) emit(e Event) {
	if r.Events != nil {
		func() { defer func() { _ = recover() }(); r.Events.Emit(e) }()
	}
}

type ScanStore interface {
	Save(context.Context, domain.Scan) error
}
type filesystemProvider struct{ path string }

func (p filesystemProvider) Stat(ctx context.Context) (capabilities.FilesystemCapacity, domain.UnknownReasonCode) {
	v, r := filesystem.Stat(ctx, p.path)
	return capabilities.FilesystemCapacity{Total: v.Total, Available: v.Available, ReadOnly: v.ReadOnly, MountPresent: v.MountPresent}, r
}

// ScanAndPersist is the single application workflow shared by CLI and daemon.
func (r Runner) ScanAndPersist(ctx context.Context, store ScanStore) (domain.Scan, error) {
	x, e := r.execute(ctx)
	if e != nil {
		r.failed(x.ID, "engine_failed")
		return x, e
	}
	if e = store.Save(ctx, x); e != nil {
		r.failed(x.ID, "persistence_failed")
		return x, e
	}
	r.completed(x)
	return x, nil
}

func (r Runner) Scan(ctx context.Context) (domain.Scan, error) {
	x, err := r.execute(ctx)
	if err != nil {
		r.failed(x.ID, "engine_failed")
		return x, err
	}
	r.completed(x)
	return x, nil
}

func (r Runner) execute(ctx context.Context) (domain.Scan, error) {
	now := r.now()
	id, err := newID()
	if err != nil {
		return domain.Scan{}, err
	}

	bitcoinNodes := r.Config.EnabledBitcoinNodes()
	electrumBackends := r.Config.EnabledElectrumBackends()
	var invBitcoin []domain.InventoryBitcoin
	for _, n := range bitcoinNodes {
		invBitcoin = append(invBitcoin, domain.InventoryBitcoin{ID: n.ID, DisplayAlias: n.DisplayAlias})
	}
	var invElectrum []domain.InventoryElectrum
	for _, b := range electrumBackends {
		invElectrum = append(invElectrum, domain.InventoryElectrum{ID: b.ID, DisplayAlias: b.DisplayAlias, BitcoinComponentID: b.BitcoinComponentID})
	}
	inv := domain.BuildInventory(invBitcoin, invElectrum, now)
	scan := domain.Scan{ID: id, StartedAt: now, Status: "running", Components: inv.Components, DependencyEdges: inv.Edges}
	r.emit(Event{ID: id + "-started", Type: "scan.started", ScanID: id, OccurredAt: now})

	// Subject-scoped capability providers.
	bitcoinClients := map[string]capabilities.BitcoinRPCReadAccess{}
	bitcoinCreateReason := map[string]domain.UnknownReasonCode{}
	fsProviders := map[string]capabilities.FilesystemStatAccess{}
	for _, node := range bitcoinNodes {
		client, createReason := bitcoinrpc.New(node.RPCEndpoint, node.CookieFile, node.CookieEnv, r.Config.Timeout())
		bitcoinCreateReason[node.ID] = createReason
		if client != nil {
			bitcoinClients[node.ID] = client
		}
		fsProviders[node.ID] = filesystemProvider{node.DataDirectory}
	}
	electrumClients := map[string]capabilities.ElectrumReadAccess{}
	electrumCreateReason := map[string]domain.UnknownReasonCode{}
	for _, backend := range electrumBackends {
		client, electrumErr := electrum.New(backend.Endpoint, time.Duration(backend.TimeoutMS)*time.Millisecond)
		if electrumErr == nil {
			electrumClients[backend.ID] = client
		} else {
			electrumCreateReason[backend.ID] = domain.NotConfigured
		}
	}

	// Observations keyed for evaluation.
	type bitcoinObs struct {
		transport, chain, fs *domain.Observation
		rpcReason, fsReason  domain.UnknownReasonCode
	}
	bitcoinCollected := map[string]*bitcoinObs{}
	for _, node := range bitcoinNodes {
		bitcoinCollected[node.ID] = &bitcoinObs{rpcReason: bitcoinCreateReason[node.ID]}
	}
	type electrumObs struct {
		transport, identity, tip, bitcoinHash *domain.Observation
		reason                                domain.UnknownReasonCode
	}
	electrumCollected := map[string]*electrumObs{}
	for _, backend := range electrumBackends {
		electrumCollected[backend.ID] = &electrumObs{reason: electrumCreateReason[backend.ID]}
	}

	runProbe := func(p probes.Probe, subject string, caps capabilities.Set) error {
		d := p.Descriptor()
		executionID := id + "-" + subject + "-" + strings.ReplaceAll(d.ID, "/", "-")
		started := r.now()
		r.emit(Event{ID: executionID + "-started", Type: "scan.probe.started", ScanID: id, OccurredAt: started, ProbeID: d.ID})
		probeCtx, cancel := context.WithTimeout(ctx, d.Timeout)
		result := p.Collect(probeCtx, caps.Scope(d.Required, d.Optional), id, executionID, started)
		ctxErr := probeCtx.Err()
		cancel()
		done := r.now()
		parentErr := ctx.Err()
		if parentErr != nil {
			result.Reason = domain.Cancelled
		} else if ctxErr == context.DeadlineExceeded {
			result.Reason = domain.Timeout
		} else if ctxErr == context.Canceled {
			result.Reason = domain.Cancelled
		}
		outcome := executionOutcome(result, parentErr, ctxErr)
		scan.Executions = append(scan.Executions, domain.ProbeExecution{
			ID: executionID, ScanID: id, ProbeID: d.ID, ProbeVersion: d.Version, Outcome: outcome,
			StartedAt: started, FinishedAt: done, DurationMS: done.Sub(started).Milliseconds(), Limitation: result.Reason,
		})
		r.emit(Event{ID: executionID + "-completed", Type: "scan.probe.completed", ScanID: id, OccurredAt: done, ProbeID: d.ID, Outcome: string(outcome)})
		declared := map[string]bool{}
		for _, k := range d.ObservationKinds {
			declared[k] = true
		}
		for _, typed := range result.Values {
			if !declared[typed.Kind] {
				return fmt.Errorf("probe %s emitted undeclared observation %s", d.ID, typed.Kind)
			}
			if e := typed.Validate(); e != nil {
				return e
			}
			scan.Observations = append(scan.Observations, typed)
			switch typed.Value.(type) {
			case domain.RPCTransportResult:
				o := typed
				if bucket := bitcoinCollected[typed.ComponentID]; bucket != nil {
					bucket.transport = &o
					if result.Reason != "" {
						bucket.rpcReason = result.Reason
					}
				}
			case domain.BitcoinChainState:
				o := typed
				if bucket := bitcoinCollected[typed.ComponentID]; bucket != nil {
					bucket.chain = &o
				}
			case domain.FilesystemCapacity:
				o := typed
				if bucket := bitcoinCollected[typed.ComponentID]; bucket != nil {
					bucket.fs = &o
					bucket.fsReason = result.Reason
				}
			case domain.ElectrumProtocolTransport:
				o := typed
				if bucket := electrumCollected[typed.ComponentID]; bucket != nil {
					bucket.transport = &o
				}
			case domain.ElectrumServerIdentity:
				o := typed
				if bucket := electrumCollected[typed.ComponentID]; bucket != nil {
					bucket.identity = &o
				}
			case domain.ElectrumChainTip:
				o := typed
				if bucket := electrumCollected[typed.ComponentID]; bucket != nil {
					bucket.tip = &o
				}
			case domain.BitcoinBlockHash:
				// Hash is attributed to bitcoin component; attach to electrum run that produced it via scan order.
				// Find electrum bucket by current subject if this is electrum probe.
				if bucket := electrumCollected[subject]; bucket != nil {
					o := typed
					bucket.bitcoinHash = &o
				}
			}
		}
		if d.ID == "bitcoin.chain-state/v1" {
			if bucket := bitcoinCollected[subject]; bucket != nil && result.Reason != "" {
				bucket.rpcReason = result.Reason
			}
		}
		if d.ID == "filesystem.capacity/v1" {
			if bucket := bitcoinCollected[subject]; bucket != nil {
				bucket.fsReason = result.Reason
			}
		}
		if d.ID == "electrum.protocol-state/v1" {
			if bucket := electrumCollected[subject]; bucket != nil {
				if electrumCreateReason[subject] != "" {
					bucket.reason = electrumCreateReason[subject]
				} else {
					bucket.reason = result.Reason
				}
			}
		}
		return nil
	}

	// Bitcoin nodes: chain-state + filesystem.
	for _, node := range bitcoinNodes {
		caps := capabilities.Set{
			Bitcoin:    bitcoinClients[node.ID],
			Filesystem: fsProviders[node.ID],
			Clock:      capabilities.SystemClock{},
		}
		if err := runProbe(probes.BitcoinChainState{ComponentID: node.ID}, node.ID, caps); err != nil {
			return scan, err
		}
		if err := runProbe(probes.FilesystemCapacity{ComponentID: node.ID}, node.ID, caps); err != nil {
			return scan, err
		}
	}

	// Electrum backends: protocol-state with bound bitcoin capability.
	for _, backend := range electrumBackends {
		sideEffect := probes.SideEffect(electrum.EndpointSideEffect(backend.Endpoint))
		timeout := time.Duration(backend.TimeoutMS) * time.Millisecond
		caps := capabilities.Set{
			Electrum: electrumClients[backend.ID],
			Bitcoin:  bitcoinClients[backend.BitcoinComponentID],
			Clock:    capabilities.SystemClock{},
		}
		p := probes.ElectrumProtocolState{
			ComponentID: backend.ID, BitcoinComponentID: backend.BitcoinComponentID,
			SideEffect: sideEffect, Timeout: timeout,
		}
		if err := runProbe(p, backend.ID, caps); err != nil {
			return scan, err
		}
	}

	evaluationTime := r.now()
	for _, node := range bitcoinNodes {
		bucket := bitcoinCollected[node.ID]
		scan.Findings = append(scan.Findings, evaluation.RPC(id, node.ID, bucket.transport, bucket.rpcReason, evaluationTime))
		scan.Findings = append(scan.Findings, evaluation.Chain(id, node.ID, bucket.chain, bucket.rpcReason, r.Config.Sync, evaluationTime)...)
		scan.Findings = append(scan.Findings, evaluation.Filesystem(id, node.ID, bucket.fs, bucket.fsReason, r.Config.Filesystem, evaluationTime))
	}
	for _, backend := range electrumBackends {
		bucket := electrumCollected[backend.ID]
		bitcoinBucket := bitcoinCollected[backend.BitcoinComponentID]
		var bitcoinChain *domain.Observation
		if bitcoinBucket != nil {
			bitcoinChain = bitcoinBucket.chain
		}
		scan.Findings = append(scan.Findings, evaluation.Electrum(id, backend.ID, evaluation.ElectrumEvidence{
			Transport: bucket.transport, Identity: bucket.identity, Tip: bucket.tip,
			Bitcoin: bitcoinChain, BitcoinHash: bucket.bitcoinHash, Limitation: bucket.reason,
		}, r.Config.ElectrumThresholds, evaluationTime)...)
	}
	for _, f := range scan.Findings {
		r.emit(Event{ID: f.ID + "-created", Type: "scan.finding.created", ScanID: id, OccurredAt: f.EvaluatedAt, FindingCode: f.Code, Outcome: string(f.Status)})
	}
	if len(electrumBackends) > 0 {
		scan.Diagnoses = correlation.Electrum(id, scan.DependencyEdges, scan.Findings, evaluationTime)
		for _, diagnosis := range scan.Diagnoses {
			r.emit(Event{ID: diagnosis.ID + "-created", Type: "scan.diagnosis.created", ScanID: id, OccurredAt: diagnosis.DiagnosedAt, DiagnosisCode: diagnosis.Code, Outcome: string(diagnosis.Classification)})
		}
	}
	scan.Summary = evaluation.Summary(scan.Findings)
	scan.FinishedAt = r.now()
	scan.Status = "completed"
	return scan, nil
}

func (r Runner) completed(scan domain.Scan) {
	r.emit(Event{ID: scan.ID + "-completed", Type: "scan.completed", ScanID: scan.ID, OccurredAt: scan.FinishedAt, Outcome: scan.Status})
}

func (r Runner) failed(scanID, outcome string) {
	if scanID == "" {
		return
	}
	r.emit(Event{ID: scanID + "-failed", Type: "scan.failed", ScanID: scanID, OccurredAt: r.now(), Outcome: outcome})
}
func (r Runner) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}
func newID() (string, error) {
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func executionOutcome(result probes.Result, parentErr, probeErr error) domain.ExecutionOutcome {
	if parentErr != nil || probeErr == context.Canceled {
		return domain.ExecutionCancelled
	}
	if probeErr == context.DeadlineExceeded {
		return domain.ExecutionFailed
	}
	if len(result.Values) > 0 && result.Reason != "" {
		return domain.ExecutionPartial
	}
	if result.Reason == "" {
		return domain.ExecutionCompleted
	}
	if result.Reason == domain.NotConfigured || result.Reason == domain.SecretReferenceUnavailable || result.Reason == domain.PermissionDenied || result.Reason == domain.RequiredObservationMissing {
		return domain.ExecutionUnavailable
	}
	return domain.ExecutionFailed
}
