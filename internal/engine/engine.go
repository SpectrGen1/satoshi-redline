package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/capabilities"
	"github.com/satoshi-redline/redline/internal/capabilities/bitcoinrpc"
	"github.com/satoshi-redline/redline/internal/capabilities/filesystem"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/evaluation"
	"github.com/satoshi-redline/redline/internal/probes"
	"time"
)

type Runner struct {
	Config app.Config
	Now    func() time.Time
}
type ScanStore interface {
	Save(context.Context, domain.Scan) error
}
type filesystemProvider struct{ path string }

func (p filesystemProvider) Stat(ctx context.Context) (capabilities.FilesystemCapacity, domain.UnknownReason) {
	v, r := filesystem.Stat(ctx, p.path)
	return capabilities.FilesystemCapacity{Total: v.Total, Available: v.Available, ReadOnly: v.ReadOnly, MountPresent: v.MountPresent}, r
}

// ScanAndPersist is the single application workflow shared by CLI and daemon.
func (r Runner) ScanAndPersist(ctx context.Context, store ScanStore) (domain.Scan, error) {
	x, e := r.Scan(ctx)
	if e != nil {
		return x, e
	}
	if e = store.Save(ctx, x); e != nil {
		return x, e
	}
	return x, nil
}

func (r Runner) Scan(ctx context.Context) (domain.Scan, error) {
	now := r.now()
	id, err := newID()
	if err != nil {
		return domain.Scan{}, err
	}
	scan := domain.Scan{ID: id, StartedAt: now, Status: "running"}
	scan.Observations = append(scan.Observations, domain.Observation{ID: id + "-clock", ScanID: id, Kind: "system.clock.current/v1", ComponentID: "system.local", ObservedAt: now})
	var rpcObs *domain.Observation
	client, createReason := bitcoinrpc.New(r.Config.Bitcoin.RPCEndpoint, r.Config.Bitcoin.CookieFile, r.Config.Bitcoin.CookieEnv, r.Config.Timeout())
	caps := capabilities.Set{Filesystem: filesystemProvider{r.Config.Bitcoin.DataDirectory}, Clock: capabilities.SystemClock{}}
	if client != nil {
		caps.Bitcoin = client
	}
	catalog, catalogErr := probes.NewCatalog(probes.BitcoinChainState{}, probes.FilesystemCapacity{})
	if catalogErr != nil {
		return domain.Scan{}, catalogErr
	}
	rpcReach := false
	rpcReason := createReason
	var fsReason domain.UnknownReason
	var fsObs *domain.Observation
	for _, p := range catalog.Probes() {
		executionID := id + "-" + p.Descriptor().ID
		started := r.now()
		result := p.Collect(ctx, caps, id, executionID, started)
		done := r.now()
		scan.Executions = append(scan.Executions, domain.ProbeExecution{ID: executionID, ScanID: id, ProbeID: p.Descriptor().ID, Outcome: executionOutcome(result.Reason), StartedAt: started, FinishedAt: done, DurationMS: done.Sub(started).Milliseconds(), Reason: result.Reason})
		for _, typed := range result.Values {
			switch v := typed.Value.(type) {
			case domain.RPCTransportResult:
				rpcReach = v.Reached
				if result.Reason != "" {
					rpcReason = result.Reason
				}
				scan.Observations = append(scan.Observations, domain.Observation{ID: typed.ID, ScanID: id, Kind: "bitcoin.rpc.response/v1", ComponentID: "bitcoin.primary", ObservedAt: typed.ObservedAt, RPCReachable: &v.Reached})
			case domain.BitcoinChainState:
				rpcObs = &domain.Observation{ID: typed.ID, ScanID: id, Kind: "bitcoin.chain.state/v1", ComponentID: "bitcoin.primary", ObservedAt: typed.ObservedAt, Blocks: &v.Blocks, Headers: &v.Headers, VerificationProgress: &v.VerificationProgress, InitialBlockDownload: &v.InitialBlockDownload, TipTime: &v.TipTime}
				scan.Observations = append(scan.Observations, *rpcObs)
			case domain.FilesystemCapacity:
				fsObs = &domain.Observation{ID: typed.ID, ScanID: id, Kind: "filesystem.capacity/v1", ComponentID: "bitcoin.primary", ObservedAt: typed.ObservedAt, TotalBytes: &v.TotalBytes, AvailableBytes: &v.AvailableBytes, ReadOnly: &v.ReadOnly, MountPresent: &v.MountPresent}
				scan.Observations = append(scan.Observations, *fsObs)
			}
		}
		if p.Descriptor().ID == "bitcoin.chain-state/v1" && result.Reason != "" {
			rpcReason = result.Reason
		}
		if p.Descriptor().ID == "filesystem.capacity/v1" {
			fsReason = result.Reason
		}
	}
	scan.Findings = append(scan.Findings, evaluation.RPC(id, &rpcReach, rpcReason))
	scan.Findings = append(scan.Findings, evaluation.Chain(id, rpcObs, rpcReason, r.Config.Sync, now)...)
	scan.Findings = append(scan.Findings, evaluation.Filesystem(id, fsObs, fsReason, r.Config.Filesystem))
	scan.Summary = evaluation.Summary(scan.Findings)
	scan.FinishedAt = r.now()
	scan.Status = "completed"
	return scan, nil
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
func executionOutcome(reason domain.UnknownReason) domain.ExecutionOutcome {
	if reason == "" {
		return domain.ExecutionCompleted
	}
	return domain.ExecutionUnavailable
}
