package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"github.com/satoshi-redline/redline/internal/domain"
	"io/fs"
	_ "modernc.org/sqlite"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, e := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(4)
	s := &Store{db}
	if e = s.Migrate(context.Background()); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	entries, e := fs.ReadDir(migrationFS, "migrations")
	if e != nil {
		return e
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	seen := map[int]bool{}
	for _, entry := range entries {
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return fmt.Errorf("malformed migration %q", entry.Name())
		}
		v, e := strconv.Atoi(parts[0])
		if e != nil || v < 1 || seen[v] {
			return fmt.Errorf("invalid migration number %q", entry.Name())
		}
		seen[v] = true
		var applied int
		e = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&applied)
		if e != nil {
			return e
		}
		if applied > 0 {
			e = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version=?", v).Scan(&applied)
			if e != nil {
				return e
			}
			if applied > 0 {
				continue
			}
		}
		b, e := migrationFS.ReadFile("migrations/" + entry.Name())
		if e != nil {
			return e
		}
		tx, e := s.db.BeginTx(ctx, nil)
		if e != nil {
			return e
		}
		defer tx.Rollback()
		if _, e = tx.ExecContext(ctx, string(b)); e != nil {
			return fmt.Errorf("apply migration %03d: %w", v, e)
		}
		if _, e = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES(?)", v); e != nil {
			return e
		}
		if e = tx.Commit(); e != nil {
			return e
		}
	}
	return nil
}
func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func b(v bool) int {
	if v {
		return 1
	}
	return 0
}
func (s *Store) Save(ctx context.Context, x domain.Scan) error {
	if e := x.Validate(); e != nil {
		return fmt.Errorf("validate scan: %w", e)
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `INSERT INTO scans(id,started_at,finished_at,status,health,coverage_state,applicable_required,assessed_required,unknown_required) VALUES(?,?,?,?,?,?,?,?,?)`, x.ID, ts(x.StartedAt), ts(x.FinishedAt), x.Status, x.Summary.Health, x.Summary.Coverage.State, x.Summary.Coverage.ApplicableRequired, x.Summary.Coverage.AssessedRequired, x.Summary.Coverage.UnknownRequired); e != nil {
		return e
	}
	for _, p := range x.Executions {
		if _, e = tx.ExecContext(ctx, `INSERT INTO probe_executions(id,scan_id,probe_id,probe_version,outcome,started_at,finished_at,duration_ms,limitation_code) VALUES(?,?,?,?,?,?,?,?,?)`, p.ID, p.ScanID, p.ProbeID, p.ProbeVersion, p.Outcome, ts(p.StartedAt), ts(p.FinishedAt), p.DurationMS, p.Limitation); e != nil {
			return e
		}
	}
	for _, o := range x.Observations {
		if _, e = tx.ExecContext(ctx, `INSERT INTO observations(id,scan_id,probe_execution_id,component_id,kind,schema_version,observed_at,capability_id,provider_id,sensitivity) VALUES(?,?,?,?,?,?,?,?,?,?)`, o.ID, o.ScanID, o.ProbeExecutionID, o.ComponentID, o.Kind, o.SchemaVersion, ts(o.ObservedAt), o.CapabilityID, o.ProviderID, o.Sensitivity); e != nil {
			return e
		}
		switch v := o.Value.(type) {
		case domain.RPCTransportResult:
			_, e = tx.ExecContext(ctx, `INSERT INTO observation_bitcoin_rpc_transport(observation_id,reached,authenticated,category) VALUES(?,?,?,?)`, o.ID, b(v.Reached), b(v.Authenticated), v.Category)
		case domain.BitcoinChainState:
			_, e = tx.ExecContext(ctx, `INSERT INTO observation_bitcoin_chain_state(observation_id,blocks,headers,verification_progress,initial_block_download,tip_time) VALUES(?,?,?,?,?,?)`, o.ID, v.Blocks, v.Headers, v.VerificationProgress, b(v.InitialBlockDownload), ts(v.TipTime))
		case domain.FilesystemCapacity:
			_, e = tx.ExecContext(ctx, `INSERT INTO observation_filesystem_capacity(observation_id,total_bytes,available_bytes,read_only,mount_present) VALUES(?,?,?,?,?)`, o.ID, v.TotalBytes, v.AvailableBytes, b(v.ReadOnly), b(v.MountPresent))
		case domain.ClockState:
			_, e = tx.ExecContext(ctx, `INSERT INTO observation_clock_state(observation_id,current_time) VALUES(?,?)`, o.ID, ts(v.Current))
		default:
			return fmt.Errorf("unsupported observation value")
		}
		if e != nil {
			return e
		}
	}
	for _, f := range x.Findings {
		if _, e = tx.ExecContext(ctx, `INSERT INTO findings(id,scan_id,code,component_id,status,applicability,severity,confidence,evaluator_id,evaluator_version,evaluated_at,coverage_role) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, f.ID, f.ScanID, f.Code, f.ComponentID, f.Status, f.Applicability, f.Severity, f.Confidence, f.EvaluatorID, f.EvaluatorVersion, ts(f.EvaluatedAt), f.Role); e != nil {
			return e
		}
		for i, r := range f.Reasons {
			_, e = tx.ExecContext(ctx, `INSERT INTO finding_unknown_reasons(finding_id,ordinal,code,stage,retryability,resolution_class,capability_id,provider_id) VALUES(?,?,?,?,?,?,?,?)`, f.ID, i, r.Code, r.Stage, r.Retryability, r.Resolution, r.CapabilityID, r.ProviderID)
			if e != nil {
				return e
			}
		}
		for i, r := range f.Evidence {
			_, e = tx.ExecContext(ctx, `INSERT INTO finding_evidence(finding_id,ordinal,observation_id,observation_kind,evidence_role) VALUES(?,?,?,?,?)`, f.ID, i, r.ObservationID, r.ObservationKind, r.Role)
			if e != nil {
				return e
			}
		}
		for i, r := range f.MissingEvidence {
			_, e = tx.ExecContext(ctx, `INSERT INTO finding_missing_evidence(finding_id,ordinal,observation_kind,problem,reason_code) VALUES(?,?,?,?,?)`, f.ID, i, r.ObservationKind, r.Problem, r.ReasonCode)
			if e != nil {
				return e
			}
		}
		t := f.Thresholds
		_, e = tx.ExecContext(ctx, `INSERT INTO finding_thresholds(finding_id,max_tip_age_seconds,min_verification_progress,max_header_lag,warning_available_percent,failure_available_percent) VALUES(?,?,?,?,?,?)`, f.ID, t.MaxTipAgeSeconds, t.MinVerificationProgress, t.MaxHeaderLag, t.WarningAvailablePercent, t.FailureAvailablePercent)
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}
func parse(v string) (time.Time, error) { return time.Parse(time.RFC3339Nano, v) }
func (s *Store) Get(ctx context.Context, id string) (domain.Scan, error) {
	var x domain.Scan
	var a, z string
	e := s.db.QueryRowContext(ctx, `SELECT id,started_at,finished_at,status,health,coverage_state,applicable_required,assessed_required,unknown_required FROM scans WHERE id=?`, id).Scan(&x.ID, &a, &z, &x.Status, &x.Summary.Health, &x.Summary.Coverage.State, &x.Summary.Coverage.ApplicableRequired, &x.Summary.Coverage.AssessedRequired, &x.Summary.Coverage.UnknownRequired)
	if e != nil {
		return x, e
	}
	if x.StartedAt, e = parse(a); e != nil {
		return x, e
	}
	if x.FinishedAt, e = parse(z); e != nil {
		return x, e
	}
	x.Summary.ComponentID = "bitcoin.primary"
	rows, e := s.db.QueryContext(ctx, `SELECT id,scan_id,probe_id,probe_version,outcome,started_at,finished_at,duration_ms,limitation_code FROM probe_executions WHERE scan_id=? ORDER BY id`, id)
	if e != nil {
		return x, e
	}
	for rows.Next() {
		var p domain.ProbeExecution
		var st, ft string
		if e = rows.Scan(&p.ID, &p.ScanID, &p.ProbeID, &p.ProbeVersion, &p.Outcome, &st, &ft, &p.DurationMS, &p.Limitation); e != nil {
			rows.Close()
			return x, e
		}
		p.StartedAt, e = parse(st)
		if e != nil {
			return x, e
		}
		p.FinishedAt, e = parse(ft)
		if e != nil {
			return x, e
		}
		x.Executions = append(x.Executions, p)
	}
	rows.Close()
	if e = rows.Err(); e != nil {
		return x, e
	}
	if x.Observations, e = s.loadObservations(ctx, id); e != nil {
		return x, e
	}
	if x.Findings, e = s.loadFindings(ctx, id); e != nil {
		return x, e
	}
	if e = x.Validate(); e != nil {
		return x, fmt.Errorf("validate loaded scan: %w", e)
	}
	return x, nil
}
func (s *Store) loadObservations(ctx context.Context, id string) ([]domain.Observation, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,scan_id,probe_execution_id,component_id,kind,schema_version,observed_at,capability_id,provider_id,sensitivity FROM observations WHERE scan_id=? ORDER BY id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Observation
	for rows.Next() {
		var o domain.Observation
		var at string
		if e = rows.Scan(&o.ID, &o.ScanID, &o.ProbeExecutionID, &o.ComponentID, &o.Kind, &o.SchemaVersion, &at, &o.CapabilityID, &o.ProviderID, &o.Sensitivity); e != nil {
			return nil, e
		}
		o.ObservedAt, e = parse(at)
		if e != nil {
			return nil, e
		}
		switch o.Kind {
		case "bitcoin.rpc.transport":
			var reached, auth int
			var cat string
			e = s.db.QueryRowContext(ctx, `SELECT reached,authenticated,category FROM observation_bitcoin_rpc_transport WHERE observation_id=?`, o.ID).Scan(&reached, &auth, &cat)
			o.Value = domain.RPCTransportResult{Reached: reached != 0, Authenticated: auth != 0, Category: cat}
		case "bitcoin.chain.state":
			var v domain.BitcoinChainState
			var ibd int
			var tip string
			e = s.db.QueryRowContext(ctx, `SELECT blocks,headers,verification_progress,initial_block_download,tip_time FROM observation_bitcoin_chain_state WHERE observation_id=?`, o.ID).Scan(&v.Blocks, &v.Headers, &v.VerificationProgress, &ibd, &tip)
			v.InitialBlockDownload = ibd != 0
			v.TipTime, _ = parse(tip)
			o.Value = v
		case "filesystem.capacity":
			var v domain.FilesystemCapacity
			var ro, mp int
			e = s.db.QueryRowContext(ctx, `SELECT total_bytes,available_bytes,read_only,mount_present FROM observation_filesystem_capacity WHERE observation_id=?`, o.ID).Scan(&v.TotalBytes, &v.AvailableBytes, &ro, &mp)
			v.ReadOnly = ro != 0
			v.MountPresent = mp != 0
			o.Value = v
		default:
			return nil, fmt.Errorf("unsupported observation kind")
		}
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (s *Store) loadFindings(ctx context.Context, id string) ([]domain.Finding, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,scan_id,code,component_id,status,applicability,severity,confidence,evaluator_id,evaluator_version,evaluated_at,coverage_role FROM findings WHERE scan_id=? ORDER BY id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Finding
	for rows.Next() {
		var f domain.Finding
		var at string
		if e = rows.Scan(&f.ID, &f.ScanID, &f.Code, &f.ComponentID, &f.Status, &f.Applicability, &f.Severity, &f.Confidence, &f.EvaluatorID, &f.EvaluatorVersion, &at, &f.Role); e != nil {
			return nil, e
		}
		f.EvaluatedAt, e = parse(at)
		if e != nil {
			return nil, e
		}
		rr, e := s.db.QueryContext(ctx, `SELECT code,stage,retryability,resolution_class,capability_id,provider_id FROM finding_unknown_reasons WHERE finding_id=? ORDER BY ordinal`, f.ID)
		if e != nil {
			return nil, e
		}
		for rr.Next() {
			var r domain.UnknownReason
			if e = rr.Scan(&r.Code, &r.Stage, &r.Retryability, &r.Resolution, &r.CapabilityID, &r.ProviderID); e != nil {
				return nil, e
			}
			f.Reasons = append(f.Reasons, r)
		}
		rr.Close()
		er, e := s.db.QueryContext(ctx, `SELECT observation_id,observation_kind,evidence_role FROM finding_evidence WHERE finding_id=? ORDER BY ordinal`, f.ID)
		if e != nil {
			return nil, e
		}
		for er.Next() {
			var r domain.EvidenceRef
			if e = er.Scan(&r.ObservationID, &r.ObservationKind, &r.Role); e != nil {
				return nil, e
			}
			f.Evidence = append(f.Evidence, r)
		}
		er.Close()
		mr, e := s.db.QueryContext(ctx, `SELECT observation_kind,problem,reason_code FROM finding_missing_evidence WHERE finding_id=? ORDER BY ordinal`, f.ID)
		if e != nil {
			return nil, e
		}
		for mr.Next() {
			var r domain.MissingEvidence
			if e = mr.Scan(&r.ObservationKind, &r.Problem, &r.ReasonCode); e != nil {
				return nil, e
			}
			f.MissingEvidence = append(f.MissingEvidence, r)
		}
		mr.Close()
		e = s.db.QueryRowContext(ctx, `SELECT max_tip_age_seconds,min_verification_progress,max_header_lag,warning_available_percent,failure_available_percent FROM finding_thresholds WHERE finding_id=?`, f.ID).Scan(&f.Thresholds.MaxTipAgeSeconds, &f.Thresholds.MinVerificationProgress, &f.Thresholds.MaxHeaderLag, &f.Thresholds.WarningAvailablePercent, &f.Thresholds.FailureAvailablePercent)
		if e != nil {
			return nil, e
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
