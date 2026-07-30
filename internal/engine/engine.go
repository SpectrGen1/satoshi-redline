package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"github.com/satoshi-redline/redline/internal/app"
	"github.com/satoshi-redline/redline/internal/capabilities"
	"github.com/satoshi-redline/redline/internal/capabilities/bitcoinrpc"
	"github.com/satoshi-redline/redline/internal/capabilities/filesystem"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/evaluation"
	"github.com/satoshi-redline/redline/internal/probes"
	"strings"
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

func (p filesystemProvider) Stat(ctx context.Context) (capabilities.FilesystemCapacity, domain.UnknownReasonCode) {
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
	var rpcObs, transportObs, fsObs *domain.Observation
	client, createReason := bitcoinrpc.New(r.Config.Bitcoin.RPCEndpoint, r.Config.Bitcoin.CookieFile, r.Config.Bitcoin.CookieEnv, r.Config.Timeout())
	caps := capabilities.Set{Filesystem: filesystemProvider{r.Config.Bitcoin.DataDirectory}, Clock: capabilities.SystemClock{}}
	if client != nil {
		caps.Bitcoin = client
	}
	catalog, catalogErr := probes.NewCatalog(probes.BitcoinChainState{}, probes.FilesystemCapacity{})
	if catalogErr != nil {
		return domain.Scan{}, catalogErr
	}
	rpcReason := createReason
	var fsReason domain.UnknownReasonCode
	for _, p := range catalog.Probes() {
		d := p.Descriptor()
		executionID := id + "-" + strings.ReplaceAll(d.ID, "/", "-")
		started := r.now()
		probeCtx, cancel := context.WithTimeout(ctx, d.Timeout)
		result := p.Collect(probeCtx, caps.Scope(d.Required, d.Optional), id, executionID, started)
		ctxErr := probeCtx.Err()
		cancel()
		done := r.now()
		if ctxErr == context.DeadlineExceeded {
			result.Reason = domain.Timeout
		} else if ctxErr == context.Canceled {
			result.Reason = domain.Cancelled
		}
		outcome := executionOutcome(result.Reason)
		if len(result.Values) > 0 && result.Reason != "" {
			outcome = domain.ExecutionPartial
		}
		scan.Executions = append(scan.Executions, domain.ProbeExecution{ID: executionID, ScanID: id, ProbeID: d.ID, ProbeVersion: d.Version, Outcome: outcome, StartedAt: started, FinishedAt: done, DurationMS: done.Sub(started).Milliseconds(), Limitation: result.Reason})
		declared := map[string]bool{}
		for _, k := range d.ObservationKinds {
			declared[k] = true
		}
		for _, typed := range result.Values {
			if !declared[typed.Kind] {
				return domain.Scan{}, fmt.Errorf("probe %s emitted undeclared observation %s", d.ID, typed.Kind)
			}
			if e := typed.Validate(); e != nil {
				return domain.Scan{}, e
			}
			scan.Observations = append(scan.Observations, typed)
			switch typed.Value.(type) {
			case domain.RPCTransportResult:
				o := typed
				transportObs = &o
				if result.Reason != "" {
					rpcReason = result.Reason
				}
			case domain.BitcoinChainState:
				o := typed
				rpcObs = &o
			case domain.FilesystemCapacity:
				o := typed
				fsObs = &o
			}
		}
		if p.Descriptor().ID == "bitcoin.chain-state/v1" && result.Reason != "" {
			rpcReason = result.Reason
		}
		if p.Descriptor().ID == "filesystem.capacity/v1" {
			fsReason = result.Reason
		}
	}
	scan.Findings = append(scan.Findings, evaluation.RPC(id, transportObs, rpcReason, now))
	scan.Findings = append(scan.Findings, evaluation.Chain(id, rpcObs, rpcReason, r.Config.Sync, now)...)
	scan.Findings = append(scan.Findings, evaluation.Filesystem(id, fsObs, fsReason, r.Config.Filesystem, now))
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
func executionOutcome(reason domain.UnknownReasonCode) domain.ExecutionOutcome {
	if reason == "" {
		return domain.ExecutionCompleted
	}
	if reason == domain.Cancelled || reason == domain.Timeout {
		return domain.ExecutionCancelled
	}
	if reason == domain.NotConfigured || reason == domain.SecretReferenceUnavailable || reason == domain.PermissionDenied || reason == domain.RequiredObservationMissing {
		return domain.ExecutionUnavailable
	}
	return domain.ExecutionFailed
}
