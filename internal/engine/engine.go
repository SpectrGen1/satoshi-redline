package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/capabilities/bitcoinrpc"
	"github.com/satoshi-redline/redline/internal/capabilities/filesystem"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/evaluation"
	"time"
)

type Runner struct {
	Config app.Config
	Now    func() time.Time
}
type ScanStore interface {
	Save(context.Context, domain.Scan) error
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
	rpcReach := false
	rpcReason := createReason
	if client != nil {
		info, reason, reachable := client.Chain(ctx)
		rpcReach = reachable
		rpcReason = reason
		if reason == "" {
			o := domain.Observation{ID: id + "-rpc", ScanID: id, Kind: "bitcoin.rpc.response/v1", ComponentID: "bitcoin.primary", ObservedAt: now, RPCReachable: &rpcReach}
			scan.Observations = append(scan.Observations, o)
			rpcObs = &domain.Observation{ID: id + "-chain", ScanID: id, Kind: "bitcoin.chain.state/v1", ComponentID: "bitcoin.primary", ObservedAt: now, Blocks: &info.Blocks, Headers: &info.Headers, VerificationProgress: &info.VerificationProgress, InitialBlockDownload: &info.InitialBlockDownload, TipTime: &info.TipTime}
			scan.Observations = append(scan.Observations, *rpcObs)
		}
	}
	bitcoinDone := r.now()
	scan.Executions = append(scan.Executions, domain.ProbeExecution{ID: id + "-bitcoin-probe", ScanID: id, ProbeID: "bitcoin.chain-state/v1", Outcome: executionOutcome(rpcReason), StartedAt: now, FinishedAt: bitcoinDone, DurationMS: bitcoinDone.Sub(now).Milliseconds(), Reason: rpcReason})
	scan.Findings = append(scan.Findings, evaluation.RPC(id, &rpcReach, rpcReason))
	scan.Findings = append(scan.Findings, evaluation.Chain(id, rpcObs, rpcReason, r.Config.Sync, now)...)
	cap, fsReason := filesystem.Stat(ctx, r.Config.Bitcoin.DataDirectory)
	var fsObs *domain.Observation
	if fsReason == "" {
		fsObs = &domain.Observation{ID: id + "-fs", ScanID: id, Kind: "filesystem.capacity/v1", ComponentID: "bitcoin.primary", ObservedAt: now, TotalBytes: &cap.Total, AvailableBytes: &cap.Available, ReadOnly: &cap.ReadOnly, MountPresent: &cap.MountPresent}
		scan.Observations = append(scan.Observations, *fsObs)
	}
	filesystemDone := r.now()
	scan.Executions = append(scan.Executions, domain.ProbeExecution{ID: id + "-filesystem-probe", ScanID: id, ProbeID: "filesystem.capacity/v1", Outcome: executionOutcome(fsReason), StartedAt: bitcoinDone, FinishedAt: filesystemDone, DurationMS: filesystemDone.Sub(bitcoinDone).Milliseconds(), Reason: fsReason})
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
