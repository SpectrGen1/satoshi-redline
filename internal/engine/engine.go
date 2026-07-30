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
	"github.com/satoshi-redline/redline/internal/capabilities/filesystem"
	"github.com/satoshi-redline/redline/internal/domain"
	"github.com/satoshi-redline/redline/internal/evaluation"
	"github.com/satoshi-redline/redline/internal/probes"
)

type Runner struct {
	Config  app.Config
	Now     func() time.Time
	Events  EventSink
	catalog *probes.Catalog
}
type Event struct {
	ID, Type, ScanID              string
	OccurredAt                    time.Time
	ProbeID, FindingCode, Outcome string
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

// execute performs collection and evaluation without emitting a terminal event.
// The workflow owner emits exactly one terminal event after all of its work,
// including persistence when applicable, has succeeded or failed.
func (r Runner) execute(ctx context.Context) (domain.Scan, error) {
	now := r.now()
	id, err := newID()
	if err != nil {
		return domain.Scan{}, err
	}
	scan := domain.Scan{ID: id, StartedAt: now, Status: "running", Components: domain.BaseComponents(r.Config.Electrum.Enabled)}
	if r.Config.Electrum.Enabled {
		scan.DependencyEdges = []domain.DependencyEdge{{
			ID: "electrum.primary-uses-chain-from-bitcoin.primary", SourceComponentID: "electrum.primary",
			DestinationComponentID: r.Config.Electrum.BitcoinComponentID, Relationship: domain.UsesChainFrom,
			Provenance: domain.ProvenanceConfiguration, Confidence: domain.ConfidenceHigh, RecordedAt: now,
		}}
	}
	r.emit(Event{ID: id + "-started", Type: "scan.started", ScanID: id, OccurredAt: now})
	var rpcObs, transportObs, fsObs *domain.Observation
	client, createReason := bitcoinrpc.New(r.Config.Bitcoin.RPCEndpoint, r.Config.Bitcoin.CookieFile, r.Config.Bitcoin.CookieEnv, r.Config.Timeout())
	caps := capabilities.Set{Filesystem: filesystemProvider{r.Config.Bitcoin.DataDirectory}, Clock: capabilities.SystemClock{}}
	if client != nil {
		caps.Bitcoin = client
	}
	catalog, catalogErr := probes.NewCatalog(probes.BitcoinChainState{}, probes.FilesystemCapacity{})
	if r.catalog != nil {
		catalog = *r.catalog
	} else if catalogErr != nil {
		return scan, catalogErr
	}
	rpcReason := createReason
	var fsReason domain.UnknownReasonCode
	for _, p := range catalog.Probes() {
		d := p.Descriptor()
		executionID := id + "-" + strings.ReplaceAll(d.ID, "/", "-")
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
		scan.Executions = append(scan.Executions, domain.ProbeExecution{ID: executionID, ScanID: id, ProbeID: d.ID, ProbeVersion: d.Version, Outcome: outcome, StartedAt: started, FinishedAt: done, DurationMS: done.Sub(started).Milliseconds(), Limitation: result.Reason})
		r.emit(Event{ID: executionID + "-completed", Type: "scan.probe.completed", ScanID: id, OccurredAt: done, ProbeID: d.ID, Outcome: string(outcome)})
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
	for _, f := range scan.Findings {
		r.emit(Event{ID: f.ID + "-created", Type: "scan.finding.created", ScanID: id, OccurredAt: f.EvaluatedAt, FindingCode: f.Code, Outcome: string(f.Status)})
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
