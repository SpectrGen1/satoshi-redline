package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/satoshi-redline/redline/internal/domain"
	_ "modernc.org/sqlite"
	"time"
)

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	s := &Store{db}
	return s, s.Migrate(context.Background())
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	_, e := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY); CREATE TABLE IF NOT EXISTS scans(id TEXT PRIMARY KEY,started_at TEXT NOT NULL,finished_at TEXT NOT NULL,status TEXT NOT NULL,health TEXT NOT NULL,coverage_state TEXT NOT NULL,applicable_required INTEGER NOT NULL,assessed_required INTEGER NOT NULL,unknown_required INTEGER NOT NULL); CREATE TABLE IF NOT EXISTS probe_executions(id TEXT PRIMARY KEY,scan_id TEXT NOT NULL,probe_id TEXT NOT NULL,outcome TEXT NOT NULL,started_at TEXT NOT NULL,finished_at TEXT NOT NULL,duration_ms INTEGER NOT NULL,reason TEXT NOT NULL); CREATE TABLE IF NOT EXISTS observations(id TEXT PRIMARY KEY,scan_id TEXT NOT NULL,kind TEXT NOT NULL,component_id TEXT NOT NULL,observed_at TEXT NOT NULL,rpc_reachable INTEGER,blocks INTEGER,headers INTEGER,verification_progress REAL,initial_block_download INTEGER,tip_time TEXT,total_bytes INTEGER,available_bytes INTEGER,read_only INTEGER,mount_present INTEGER); CREATE TABLE IF NOT EXISTS findings(id TEXT PRIMARY KEY,scan_id TEXT NOT NULL,code TEXT NOT NULL,component_id TEXT NOT NULL,status TEXT NOT NULL,applicability TEXT NOT NULL,reasons TEXT NOT NULL,evaluator TEXT NOT NULL,provenance TEXT NOT NULL,evaluated_at TEXT NOT NULL,required INTEGER NOT NULL);`)
	return e
}
func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func bi(p *bool) any {
	if p == nil {
		return nil
	}
	if *p {
		return 1
	}
	return 0
}
func (s *Store) Save(ctx context.Context, x domain.Scan) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	for _, p := range x.Executions {
		_, e = tx.ExecContext(ctx, `INSERT INTO probe_executions VALUES(?,?,?,?,?,?,?,?)`, p.ID, p.ScanID, p.ProbeID, p.Outcome, ts(p.StartedAt), ts(p.FinishedAt), p.DurationMS, p.Reason)
		if e != nil {
			return e
		}
	}
	defer tx.Rollback()
	_, e = tx.ExecContext(ctx, `INSERT INTO scans VALUES(?,?,?,?,?,?,?,?,?)`, x.ID, ts(x.StartedAt), ts(x.FinishedAt), x.Status, x.Summary.Health, x.Summary.Coverage.State, x.Summary.Coverage.ApplicableRequired, x.Summary.Coverage.AssessedRequired, x.Summary.Coverage.UnknownRequired)
	if e != nil {
		return e
	}
	for _, o := range x.Observations {
		_, e = tx.ExecContext(ctx, `INSERT INTO observations VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, o.ID, o.ScanID, o.Kind, o.ComponentID, ts(o.ObservedAt), bi(o.RPCReachable), o.Blocks, o.Headers, o.VerificationProgress, bi(o.InitialBlockDownload), nullableTime(o.TipTime), o.TotalBytes, o.AvailableBytes, bi(o.ReadOnly), bi(o.MountPresent))
		if e != nil {
			return e
		}
	}
	for _, f := range x.Findings {
		r, _ := json.Marshal(f.Reasons)
		_, e = tx.ExecContext(ctx, `INSERT INTO findings VALUES(?,?,?,?,?,?,?,?,?,?,?)`, f.ID, f.ScanID, f.Code, f.ComponentID, f.Status, f.Applicability, string(r), f.Evaluator, f.Provenance, ts(f.EvaluatedAt), boolInt(f.Required))
		if e != nil {
			return e
		}
	}
	return tx.Commit()
}
func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return ts(*t)
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func (s *Store) Get(ctx context.Context, id string) (domain.Scan, error) {
	var x domain.Scan
	var st, ft string
	row := s.db.QueryRowContext(ctx, `SELECT id,started_at,finished_at,status,health,coverage_state,applicable_required,assessed_required,unknown_required FROM scans WHERE id=?`, id)
	e := row.Scan(&x.ID, &st, &ft, &x.Status, &x.Summary.Health, &x.Summary.Coverage.State, &x.Summary.Coverage.ApplicableRequired, &x.Summary.Coverage.AssessedRequired, &x.Summary.Coverage.UnknownRequired)
	if e != nil {
		return x, e
	}
	x.Summary.ComponentID = "bitcoin.primary"
	x.StartedAt, _ = time.Parse(time.RFC3339Nano, st)
	x.FinishedAt, _ = time.Parse(time.RFC3339Nano, ft)
	x.Observations, e = s.observations(ctx, id)
	if e != nil {
		return x, e
	}
	x.Findings, e = s.findings(ctx, id)
	return x, e
}
func (s *Store) observations(ctx context.Context, id string) ([]domain.Observation, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,scan_id,kind,component_id,observed_at,rpc_reachable,blocks,headers,verification_progress,initial_block_download,tip_time,total_bytes,available_bytes,read_only,mount_present FROM observations WHERE scan_id=?`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Observation
	for rows.Next() {
		var o domain.Observation
		var at string
		var rr, ibd, ro, mp sql.NullInt64
		var tip sql.NullString
		e = rows.Scan(&o.ID, &o.ScanID, &o.Kind, &o.ComponentID, &at, &rr, &o.Blocks, &o.Headers, &o.VerificationProgress, &ibd, &tip, &o.TotalBytes, &o.AvailableBytes, &ro, &mp)
		if e != nil {
			return nil, e
		}
		o.ObservedAt, _ = time.Parse(time.RFC3339Nano, at)
		o.RPCReachable = np(rr)
		o.InitialBlockDownload = np(ibd)
		o.ReadOnly = np(ro)
		o.MountPresent = np(mp)
		if tip.Valid {
			t, _ := time.Parse(time.RFC3339Nano, tip.String)
			o.TipTime = &t
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func np(n sql.NullInt64) *bool {
	if !n.Valid {
		return nil
	}
	b := n.Int64 != 0
	return &b
}
func (s *Store) findings(ctx context.Context, id string) ([]domain.Finding, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT id,scan_id,code,component_id,status,applicability,reasons,evaluator,provenance,evaluated_at,required FROM findings WHERE scan_id=?`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Finding
	for rows.Next() {
		var f domain.Finding
		var rs, at string
		var req int
		e = rows.Scan(&f.ID, &f.ScanID, &f.Code, &f.ComponentID, &f.Status, &f.Applicability, &rs, &f.Evaluator, &f.Provenance, &at, &req)
		if e != nil {
			return nil, e
		}
		if e = json.Unmarshal([]byte(rs), &f.Reasons); e != nil {
			return nil, fmt.Errorf("invalid stored reason")
		}
		f.EvaluatedAt, _ = time.Parse(time.RFC3339Nano, at)
		f.Required = req != 0
		out = append(out, f)
	}
	return out, rows.Err()
}
