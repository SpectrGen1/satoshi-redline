package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"github.com/satoshi-redline/redline/internal/domain"
	"io/fs"
	_ "modernc.org/sqlite"
	"path"
	"regexp"
	"sort"
	"strconv"
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

type migration struct {
	version int
	name    string
	sql     string
}

var migrationName = regexp.MustCompile(`^([0-9]{3})_[a-z0-9]+(?:_[a-z0-9]+)*\.sql$`)

func discoverMigrations(source fs.FS, directory string) ([]migration, error) {
	entries, e := fs.ReadDir(source, directory)
	if e != nil {
		return nil, e
	}
	seen := map[int]bool{}
	out := make([]migration, 0, len(entries))
	for _, entry := range entries {
		matches := migrationName.FindStringSubmatch(entry.Name())
		if entry.IsDir() || matches == nil {
			return nil, fmt.Errorf("malformed migration %q", entry.Name())
		}
		v, e := strconv.Atoi(matches[1])
		if e != nil || v <= 0 || seen[v] {
			return nil, fmt.Errorf("invalid migration number %q", entry.Name())
		}
		seen[v] = true
		body, e := fs.ReadFile(source, path.Join(directory, entry.Name()))
		if e != nil {
			return nil, e
		}
		out = append(out, migration{version: v, name: entry.Name(), sql: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

func (s *Store) Migrate(ctx context.Context) error {
	migrations, e := discoverMigrations(migrationFS, "migrations")
	if e != nil {
		return e
	}
	for _, migration := range migrations {
		var applied int
		e = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&applied)
		if e != nil {
			return e
		}
		if applied > 0 {
			e = s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version=?", migration.version).Scan(&applied)
			if e != nil {
				return e
			}
			if applied > 0 {
				continue
			}
		}
		if e = s.applyMigration(ctx, migration); e != nil {
			return e
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, migration migration) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, migration.sql); e != nil {
		return fmt.Errorf("apply migration %03d: %w", migration.version, e)
	}
	if _, e = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version) VALUES(?)", migration.version); e != nil {
		return fmt.Errorf("record migration %03d: %w", migration.version, e)
	}
	if e = tx.Commit(); e != nil {
		return fmt.Errorf("commit migration %03d: %w", migration.version, e)
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
	for _, component := range x.Components {
		if _, e = tx.ExecContext(ctx, `INSERT INTO components(scan_id,id,kind,implementation_family,implementation_version,display_alias,provenance,inventory_state) VALUES(?,?,?,?,?,?,?,?)`, x.ID, component.ID, component.Kind, component.ImplementationFamily, component.ImplementationVersion, component.DisplayAlias, component.Provenance, component.State); e != nil {
			return e
		}
	}
	for _, edge := range x.DependencyEdges {
		if _, e = tx.ExecContext(ctx, `INSERT INTO dependency_edges(scan_id,id,source_component_id,destination_component_id,relationship_kind,provenance,confidence,recorded_at) VALUES(?,?,?,?,?,?,?,?)`, x.ID, edge.ID, edge.SourceComponentID, edge.DestinationComponentID, edge.Relationship, edge.Provenance, edge.Confidence, ts(edge.RecordedAt)); e != nil {
			return e
		}
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
			_, e = tx.ExecContext(ctx, `INSERT INTO observation_bitcoin_rpc_transport(observation_id,reached,authentication,category) VALUES(?,?,?,?)`, o.ID, b(v.Reached), v.Authentication, v.Category)
		case domain.BitcoinChainState:
			_, e = tx.ExecContext(ctx, `INSERT INTO observation_bitcoin_chain_state(observation_id,blocks,headers,verification_progress,initial_block_download,tip_time) VALUES(?,?,?,?,?,?)`, o.ID, v.Blocks, v.Headers, v.VerificationProgress, b(v.InitialBlockDownload), ts(v.TipTime))
		case domain.FilesystemCapacity:
			_, e = tx.ExecContext(ctx, `INSERT INTO observation_filesystem_capacity(observation_id,total_bytes,available_bytes,read_only,mount_present) VALUES(?,?,?,?,?)`, o.ID, v.TotalBytes, v.AvailableBytes, b(v.ReadOnly), b(v.MountPresent))
		case domain.ClockState:
			_, e = tx.ExecContext(ctx, `INSERT INTO observation_clock_state(observation_id,current_time) VALUES(?,?)`, o.ID, ts(v.Current))
		case domain.ElectrumProtocolTransport:
			_, e = tx.ExecContext(ctx, `INSERT INTO observation_electrum_protocol_transport(observation_id,reached,transport_kind,tls_verification,protocol_negotiation,category) VALUES(?,?,?,?,?,?)`, o.ID, b(v.Reached), v.Transport, v.TLSVerification, v.ProtocolNegotiation, v.Category)
		case domain.ElectrumServerIdentity:
			_, e = tx.ExecContext(ctx, `INSERT INTO observation_electrum_server_identity(observation_id,implementation_family,protocol_version) VALUES(?,?,?)`, o.ID, v.ImplementationFamily, v.ProtocolVersion)
		case domain.ElectrumChainTip:
			_, e = tx.ExecContext(ctx, `INSERT INTO observation_electrum_chain_tip(observation_id,height,block_hash,header_time) VALUES(?,?,?,?)`, o.ID, v.Height, v.BlockHash, ts(v.HeaderTime))
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
	componentRows, e := s.db.QueryContext(ctx, `SELECT id,kind,implementation_family,implementation_version,display_alias,provenance,inventory_state FROM components WHERE scan_id=? ORDER BY id`, id)
	if e != nil {
		return x, e
	}
	for componentRows.Next() {
		var component domain.Component
		if e = componentRows.Scan(&component.ID, &component.Kind, &component.ImplementationFamily, &component.ImplementationVersion, &component.DisplayAlias, &component.Provenance, &component.State); e != nil {
			componentRows.Close()
			return x, e
		}
		x.Components = append(x.Components, component)
	}
	if e = componentRows.Err(); e != nil {
		componentRows.Close()
		return x, e
	}
	componentRows.Close()
	edgeRows, e := s.db.QueryContext(ctx, `SELECT id,source_component_id,destination_component_id,relationship_kind,provenance,confidence,recorded_at FROM dependency_edges WHERE scan_id=? ORDER BY id`, id)
	if e != nil {
		return x, e
	}
	for edgeRows.Next() {
		var edge domain.DependencyEdge
		var recordedAt string
		if e = edgeRows.Scan(&edge.ID, &edge.SourceComponentID, &edge.DestinationComponentID, &edge.Relationship, &edge.Provenance, &edge.Confidence, &recordedAt); e != nil {
			edgeRows.Close()
			return x, e
		}
		if edge.RecordedAt, e = parse(recordedAt); e != nil {
			edgeRows.Close()
			return x, e
		}
		x.DependencyEdges = append(x.DependencyEdges, edge)
	}
	if e = edgeRows.Err(); e != nil {
		edgeRows.Close()
		return x, e
	}
	edgeRows.Close()
	rows, e := s.db.QueryContext(ctx, `SELECT id,scan_id,probe_id,probe_version,outcome,started_at,finished_at,duration_ms,limitation_code FROM probe_executions WHERE scan_id=? ORDER BY id`, id)
	if e != nil {
		return x, e
	}
	defer rows.Close()
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
		storedKind, e := s.typedValueKind(ctx, o.ID)
		if e != nil {
			return nil, e
		}
		if storedKind != o.Kind {
			return nil, fmt.Errorf("observation typed value kind mismatch")
		}
		switch o.Kind {
		case "bitcoin.rpc.transport":
			var reached int
			var auth domain.AuthenticationState
			var cat string
			e = s.db.QueryRowContext(ctx, `SELECT reached,authentication,category FROM observation_bitcoin_rpc_transport WHERE observation_id=?`, o.ID).Scan(&reached, &auth, &cat)
			o.Value = domain.RPCTransportResult{Reached: reached != 0, Authentication: auth, Category: cat}
		case "bitcoin.chain.state":
			var v domain.BitcoinChainState
			var ibd int
			var tip string
			e = s.db.QueryRowContext(ctx, `SELECT blocks,headers,verification_progress,initial_block_download,tip_time FROM observation_bitcoin_chain_state WHERE observation_id=?`, o.ID).Scan(&v.Blocks, &v.Headers, &v.VerificationProgress, &ibd, &tip)
			v.InitialBlockDownload = ibd != 0
			if e == nil {
				v.TipTime, e = parse(tip)
			}
			o.Value = v
		case "filesystem.capacity":
			var v domain.FilesystemCapacity
			var ro, mp int
			e = s.db.QueryRowContext(ctx, `SELECT total_bytes,available_bytes,read_only,mount_present FROM observation_filesystem_capacity WHERE observation_id=?`, o.ID).Scan(&v.TotalBytes, &v.AvailableBytes, &ro, &mp)
			v.ReadOnly = ro != 0
			v.MountPresent = mp != 0
			o.Value = v
		case "system.clock.current":
			var current string
			e = s.db.QueryRowContext(ctx, `SELECT "current_time" FROM observation_clock_state WHERE observation_id=?`, o.ID).Scan(&current)
			var v domain.ClockState
			if e == nil {
				v.Current, e = parse(current)
			}
			o.Value = v
		case "electrum.protocol.transport":
			var v domain.ElectrumProtocolTransport
			var reached int
			e = s.db.QueryRowContext(ctx, `SELECT reached,transport_kind,tls_verification,protocol_negotiation,category FROM observation_electrum_protocol_transport WHERE observation_id=?`, o.ID).Scan(&reached, &v.Transport, &v.TLSVerification, &v.ProtocolNegotiation, &v.Category)
			v.Reached = reached != 0
			o.Value = v
		case "electrum.server.identity":
			var v domain.ElectrumServerIdentity
			e = s.db.QueryRowContext(ctx, `SELECT implementation_family,protocol_version FROM observation_electrum_server_identity WHERE observation_id=?`, o.ID).Scan(&v.ImplementationFamily, &v.ProtocolVersion)
			o.Value = v
		case "electrum.chain.tip":
			var v domain.ElectrumChainTip
			var headerTime string
			e = s.db.QueryRowContext(ctx, `SELECT height,block_hash,header_time FROM observation_electrum_chain_tip WHERE observation_id=?`, o.ID).Scan(&v.Height, &v.BlockHash, &headerTime)
			if e == nil {
				v.HeaderTime, e = parse(headerTime)
			}
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

func (s *Store) typedValueKind(ctx context.Context, observationID string) (string, error) {
	rows, e := s.db.QueryContext(ctx, `
		SELECT kind FROM (
			SELECT 'bitcoin.rpc.transport' AS kind FROM observation_bitcoin_rpc_transport WHERE observation_id=?
			UNION ALL SELECT 'bitcoin.chain.state' FROM observation_bitcoin_chain_state WHERE observation_id=?
			UNION ALL SELECT 'filesystem.capacity' FROM observation_filesystem_capacity WHERE observation_id=?
			UNION ALL SELECT 'system.clock.current' FROM observation_clock_state WHERE observation_id=?
			UNION ALL SELECT 'electrum.protocol.transport' FROM observation_electrum_protocol_transport WHERE observation_id=?
			UNION ALL SELECT 'electrum.server.identity' FROM observation_electrum_server_identity WHERE observation_id=?
			UNION ALL SELECT 'electrum.chain.tip' FROM observation_electrum_chain_tip WHERE observation_id=?
		)`, observationID, observationID, observationID, observationID, observationID, observationID, observationID)
	if e != nil {
		return "", e
	}
	defer rows.Close()
	var kinds []string
	for rows.Next() {
		var kind string
		if e = rows.Scan(&kind); e != nil {
			return "", e
		}
		kinds = append(kinds, kind)
	}
	if e = rows.Err(); e != nil {
		return "", e
	}
	if len(kinds) != 1 {
		return "", fmt.Errorf("observation must have exactly one typed value")
	}
	return kinds[0], nil
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
				rr.Close()
				return nil, e
			}
			f.Reasons = append(f.Reasons, r)
		}
		if e = rr.Err(); e != nil {
			rr.Close()
			return nil, e
		}
		rr.Close()
		er, e := s.db.QueryContext(ctx, `SELECT observation_id,observation_kind,evidence_role FROM finding_evidence WHERE finding_id=? ORDER BY ordinal`, f.ID)
		if e != nil {
			return nil, e
		}
		for er.Next() {
			var r domain.EvidenceRef
			if e = er.Scan(&r.ObservationID, &r.ObservationKind, &r.Role); e != nil {
				er.Close()
				return nil, e
			}
			f.Evidence = append(f.Evidence, r)
		}
		if e = er.Err(); e != nil {
			er.Close()
			return nil, e
		}
		er.Close()
		mr, e := s.db.QueryContext(ctx, `SELECT observation_kind,problem,reason_code FROM finding_missing_evidence WHERE finding_id=? ORDER BY ordinal`, f.ID)
		if e != nil {
			return nil, e
		}
		for mr.Next() {
			var r domain.MissingEvidence
			if e = mr.Scan(&r.ObservationKind, &r.Problem, &r.ReasonCode); e != nil {
				mr.Close()
				return nil, e
			}
			f.MissingEvidence = append(f.MissingEvidence, r)
		}
		if e = mr.Err(); e != nil {
			mr.Close()
			return nil, e
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
