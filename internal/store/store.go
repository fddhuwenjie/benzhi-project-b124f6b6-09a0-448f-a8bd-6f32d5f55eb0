package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
	"wellseal/internal/domain"
)

type Store struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[string]*domain.SealCase
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, cache: map[string]*domain.SealCase{}}
	if err = s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err = s.CheckConsistency(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("SQLite 完整性检查失败: %w", err)
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) invalidate(caseID string) {
	s.mu.Lock()
	delete(s.cache, caseID)
	s.mu.Unlock()
}
func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA foreign_keys=ON`, `PRAGMA journal_mode=WAL`,
		`CREATE TABLE IF NOT EXISTS cases(case_id TEXT PRIMARY KEY, state TEXT NOT NULL, revision INTEGER NOT NULL, created_at TEXT NOT NULL, snapshot BLOB NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS audit_events(sequence INTEGER PRIMARY KEY AUTOINCREMENT, case_id TEXT NOT NULL REFERENCES cases(case_id), event_type TEXT NOT NULL, actor TEXT NOT NULL, occurred_at TEXT NOT NULL, revision INTEGER NOT NULL, data_digest TEXT NOT NULL, previous_digest TEXT NOT NULL, digest TEXT NOT NULL UNIQUE, change_summary TEXT NOT NULL DEFAULT '')`,
		`CREATE INDEX IF NOT EXISTS idx_audit_case_sequence ON audit_events(case_id,sequence)`,
		`CREATE TABLE IF NOT EXISTS idempotency(request_id TEXT PRIMARY KEY, fingerprint TEXT NOT NULL, case_id TEXT NOT NULL, response BLOB NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS archives(case_id TEXT PRIMARY KEY REFERENCES cases(case_id), archive_id TEXT NOT NULL UNIQUE, manifest_digest TEXT NOT NULL, payload BLOB NOT NULL, metadata BLOB NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS seal_plans(plan_id TEXT PRIMARY KEY, case_id TEXT NOT NULL UNIQUE REFERENCES cases(case_id), digest TEXT NOT NULL, locked_at TEXT NOT NULL, snapshot BLOB NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS construction_checkpoints(checkpoint_id TEXT PRIMARY KEY, case_id TEXT NOT NULL REFERENCES cases(case_id), sequence_no INTEGER NOT NULL, stage_type TEXT NOT NULL, measured_at TEXT NOT NULL, snapshot BLOB NOT NULL, UNIQUE(case_id,sequence_no))`,
		`CREATE TABLE IF NOT EXISTS deviations(deviation_id TEXT PRIMARY KEY, case_id TEXT NOT NULL REFERENCES cases(case_id), category TEXT NOT NULL, detected_at TEXT NOT NULL, closed_at TEXT, snapshot BLOB NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS release_decisions(case_id TEXT PRIMARY KEY REFERENCES cases(case_id), decision TEXT NOT NULL, witness_id TEXT NOT NULL, decided_at TEXT NOT NULL, snapshot BLOB NOT NULL)`,
	}
	for _, q := range statements {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("执行模式迁移失败: %w", err)
		}
	}
	// 为已有数据库补充审计摘要列。
	var hasSummary int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('audit_events') WHERE name='change_summary'`).Scan(&hasSummary); err != nil {
		return err
	} else if hasSummary == 0 {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE audit_events ADD COLUMN change_summary TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	var result string
	if err := s.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return fmt.Errorf("SQLite 完整性检查结果: %s", result)
	}
	return nil
}

func (s *Store) Create(ctx context.Context, c *domain.SealCase, requestID, fingerprint, actor string) (*domain.SealCase, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	if prior, replay, err := lookupIdempotency(ctx, tx, requestID, fingerprint); err != nil {
		return nil, false, err
	} else if replay {
		return prior, true, nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return nil, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO cases(case_id,state,revision,created_at,snapshot) VALUES(?,?,?,?,?)`, c.CaseID, c.State, c.Revision, c.CreatedAt.Format(time.RFC3339Nano), b)
	if err != nil {
		return nil, false, err
	}
	if err = appendEvent(ctx, tx, c, "case_created", actor); err != nil {
		return nil, false, err
	}
	if err = syncProjections(ctx, tx, c); err != nil {
		return nil, false, err
	}
	if err = saveIdempotency(ctx, tx, requestID, fingerprint, c.CaseID, b); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	s.invalidate(c.CaseID)
	return c, false, nil
}

type Mutator func(*domain.SealCase) error

func (s *Store) Apply(ctx context.Context, caseID string, expected int64, requestID, fingerprint, actor, eventType string, mutate Mutator) (*domain.SealCase, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	if prior, replay, err := lookupIdempotency(ctx, tx, requestID, fingerprint); err != nil {
		return nil, false, err
	} else if replay {
		return prior, true, nil
	}
	c, err := getCaseTx(ctx, tx, caseID)
	if err != nil {
		return nil, false, err
	}
	if c.State == domain.StateArchived {
		return nil, false, domain.Frozen()
	}
	if c.Revision != expected {
		return nil, false, domain.Conflict(expected, c.Revision)
	}
	if err = mutate(c); err != nil {
		return nil, false, err
	}
	c.Revision++
	b, err := json.Marshal(c)
	if err != nil {
		return nil, false, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE cases SET state=?,revision=?,snapshot=? WHERE case_id=? AND revision=?`, c.State, c.Revision, b, c.CaseID, expected)
	if err != nil {
		return nil, false, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, false, domain.Conflict(expected, c.Revision)
	}
	if err = appendEvent(ctx, tx, c, eventType, actor); err != nil {
		return nil, false, err
	}
	if err = syncProjections(ctx, tx, c); err != nil {
		return nil, false, err
	}
	if err = saveIdempotency(ctx, tx, requestID, fingerprint, c.CaseID, b); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	s.invalidate(c.CaseID)
	return c, false, nil
}

func (s *Store) FreezeArchive(ctx context.Context, caseID string, expected int64, requestID, fingerprint, actor string, a domain.ReleaseArchive) (*domain.SealCase, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	if prior, replay, err := lookupIdempotency(ctx, tx, requestID, fingerprint); err != nil {
		return nil, false, err
	} else if replay {
		return prior, true, nil
	}
	c, err := getCaseTx(ctx, tx, caseID)
	if err != nil {
		return nil, false, err
	}
	if c.State == domain.StateArchived {
		return nil, false, domain.Frozen()
	}
	if c.Revision != expected {
		return nil, false, domain.Conflict(expected, c.Revision)
	}
	if c.State != domain.StateReleased {
		return nil, false, domain.Gate("仅已放行个案可冻结归档")
	}
	now := a.GeneratedAt.UTC()
	if err = domain.Transition(c, domain.StateArchived); err != nil {
		return nil, false, err
	}
	c.ArchivedAt = &now
	c.Revision++
	b, _ := json.Marshal(c)
	meta, _ := json.Marshal(a)
	if _, err = tx.ExecContext(ctx, `INSERT INTO archives(case_id,archive_id,manifest_digest,payload,metadata,created_at) VALUES(?,?,?,?,?,?)`, caseID, a.ArchiveID, a.ManifestDigest, a.Payload, meta, now.Format(time.RFC3339Nano)); err != nil {
		return nil, false, err
	}
	res, err := tx.ExecContext(ctx, `UPDATE cases SET state=?,revision=?,snapshot=? WHERE case_id=? AND revision=?`, c.State, c.Revision, b, caseID, expected)
	if err != nil {
		return nil, false, err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return nil, false, domain.Conflict(expected, c.Revision)
	}
	if err = appendEvent(ctx, tx, c, "archive_frozen", actor); err != nil {
		return nil, false, err
	}
	if err = syncProjections(ctx, tx, c); err != nil {
		return nil, false, err
	}
	if err = saveIdempotency(ctx, tx, requestID, fingerprint, caseID, b); err != nil {
		return nil, false, err
	}
	if err = tx.Commit(); err != nil {
		return nil, false, err
	}
	s.invalidate(c.CaseID)
	return c, false, nil
}

func (s *Store) Get(ctx context.Context, id string) (*domain.SealCase, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	cached := s.cache[id]
	s.mu.RUnlock()
	if cached != nil {
		return cached.Clone(), nil
	}
	c, err := getCaseQuery(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if prior := s.cache[id]; prior != nil {
		cached = prior
	} else {
		s.cache[id] = c
		cached = c
	}
	s.mu.Unlock()
	return cached.Clone(), nil
}
func (s *Store) List(ctx context.Context, limit int) ([]domain.SealCase, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot FROM cases ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.SealCase{}
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		var c domain.SealCase
		if err = json.Unmarshal(b, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type CaseFilter struct {
	Keyword        string
	State          domain.State
	CreatedFrom    *time.Time
	CreatedTo      *time.Time
	AfterCreatedAt *time.Time
	AfterCaseID    string
	Limit          int
}
type CaseList struct {
	Items        []domain.SealCase
	HasMore      bool
	Total        int
	StatusCounts map[domain.State]int
}

type QualityFilter struct {
	State       domain.State
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	SiteName    string
	OwnerName   string
}

// QualitySnapshot 在单个只读事务中读取匹配个案及修订上界。
func (s *Store) QualitySnapshot(ctx context.Context, f QualityFilter) ([]domain.SealCase, int64, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()
	conditions, args := []string{"1=1"}, []any{}
	if f.State != "" {
		conditions = append(conditions, "state=?")
		args = append(args, f.State)
	}
	if f.CreatedFrom != nil {
		conditions = append(conditions, "created_at>=?")
		args = append(args, f.CreatedFrom.UTC().Format(time.RFC3339Nano))
	}
	if f.CreatedTo != nil {
		conditions = append(conditions, "created_at<=?")
		args = append(args, f.CreatedTo.UTC().Format(time.RFC3339Nano))
	}
	if strings.TrimSpace(f.SiteName) != "" {
		conditions = append(conditions, "lower(json_extract(snapshot,'$.site_name'))=lower(?)")
		args = append(args, strings.TrimSpace(f.SiteName))
	}
	if strings.TrimSpace(f.OwnerName) != "" {
		conditions = append(conditions, "lower(json_extract(snapshot,'$.owner_name'))=lower(?)")
		args = append(args, strings.TrimSpace(f.OwnerName))
	}
	where := strings.Join(conditions, " AND ")
	rows, err := tx.QueryContext(ctx, `SELECT snapshot FROM cases WHERE `+where+` ORDER BY created_at ASC,case_id ASC`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	cases := []domain.SealCase{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, 0, err
		}
		var c domain.SealCase
		if err = json.Unmarshal(raw, &c); err != nil {
			return nil, 0, err
		}
		cases = append(cases, c)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	var upper sql.NullInt64
	if err = tx.QueryRowContext(ctx, `SELECT MAX(revision) FROM cases WHERE `+where, args...).Scan(&upper); err != nil {
		return nil, 0, err
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, err
	}
	if !upper.Valid {
		return cases, 0, nil
	}
	return cases, upper.Int64, nil
}

func (s *Store) ListFiltered(ctx context.Context, f CaseFilter) (CaseList, error) {
	out := CaseList{Items: []domain.SealCase{}, StatusCounts: map[domain.State]int{}}
	for _, state := range []domain.State{domain.StateDraft, domain.StateBaselineLocked, domain.StateSealing, domain.StateHeld, domain.StateVerification, domain.StateReleased, domain.StateArchived} {
		out.StatusCounts[state] = 0
	}
	countRows, err := s.db.QueryContext(ctx, `SELECT state,COUNT(*) FROM cases GROUP BY state`)
	if err != nil {
		return CaseList{}, err
	}
	for countRows.Next() {
		var state domain.State
		var count int
		if err = countRows.Scan(&state, &count); err != nil {
			countRows.Close()
			return CaseList{}, err
		}
		out.StatusCounts[state] = count
	}
	if err = countRows.Close(); err != nil {
		return CaseList{}, err
	}
	conditions, args := []string{"1=1"}, []any{}
	if keyword := strings.TrimSpace(f.Keyword); keyword != "" {
		keyword = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(keyword)
		conditions = append(conditions, `(lower(json_extract(snapshot,'$.well_code')) LIKE lower(?) ESCAPE '\' OR lower(json_extract(snapshot,'$.site_name')) LIKE lower(?) ESCAPE '\' OR lower(json_extract(snapshot,'$.owner_name')) LIKE lower(?) ESCAPE '\')`)
		pattern := "%" + keyword + "%"
		args = append(args, pattern, pattern, pattern)
	}
	if f.State != "" {
		conditions = append(conditions, "state=?")
		args = append(args, f.State)
	}
	if f.CreatedFrom != nil {
		conditions = append(conditions, "created_at>=?")
		args = append(args, f.CreatedFrom.UTC().Format(time.RFC3339Nano))
	}
	if f.CreatedTo != nil {
		conditions = append(conditions, "created_at<=?")
		args = append(args, f.CreatedTo.UTC().Format(time.RFC3339Nano))
	}
	where := strings.Join(conditions, " AND ")
	if err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cases WHERE `+where, args...).Scan(&out.Total); err != nil {
		return CaseList{}, err
	}
	if f.AfterCreatedAt != nil {
		conditions = append(conditions, "(created_at<? OR (created_at=? AND case_id<?))")
		stamp := f.AfterCreatedAt.UTC().Format(time.RFC3339Nano)
		args = append(args, stamp, stamp, f.AfterCaseID)
	}
	limit := f.Limit
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot FROM cases WHERE `+strings.Join(conditions, " AND ")+` ORDER BY created_at DESC,case_id DESC LIMIT ?`, args...)
	if err != nil {
		return CaseList{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return CaseList{}, err
		}
		var c domain.SealCase
		if err = json.Unmarshal(raw, &c); err != nil {
			return CaseList{}, err
		}
		out.Items = append(out.Items, c)
	}
	if err = rows.Err(); err != nil {
		return CaseList{}, err
	}
	if len(out.Items) > limit {
		out.HasMore = true
		out.Items = out.Items[:limit]
	}
	return out, nil
}
func (s *Store) Timeline(ctx context.Context, id string, after int64, limit int) (domain.Page[domain.AuditEvent], error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT sequence,case_id,event_type,actor,occurred_at,revision,data_digest,previous_digest,digest FROM audit_events WHERE case_id=? AND sequence>? ORDER BY sequence LIMIT ?`, id, after, limit+1)
	if err != nil {
		return domain.Page[domain.AuditEvent]{}, err
	}
	defer rows.Close()
	items := []domain.AuditEvent{}
	for rows.Next() {
		var e domain.AuditEvent
		var at string
		if err = rows.Scan(&e.Sequence, &e.CaseID, &e.Type, &e.Actor, &at, &e.Revision, &e.DataDigest, &e.PreviousDigest, &e.Digest); err != nil {
			return domain.Page[domain.AuditEvent]{}, err
		}
		e.OccurredAt, _ = time.Parse(time.RFC3339Nano, at)
		items = append(items, e)
	}
	p := domain.Page[domain.AuditEvent]{Items: items}
	if len(items) > limit {
		p.NextCursor = items[limit-1].Sequence
		p.Items = items[:limit]
	}
	return p, rows.Err()
}
func (s *Store) Archive(ctx context.Context, id string) (domain.ReleaseArchive, error) {
	var meta, payload []byte
	err := s.db.QueryRowContext(ctx, `SELECT metadata,payload FROM archives WHERE case_id=?`, id).Scan(&meta, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ReleaseArchive{}, &domain.BusinessError{Code: domain.CodeNotFound, Message: "归档不存在"}
	}
	if err != nil {
		return domain.ReleaseArchive{}, err
	}
	var a domain.ReleaseArchive
	if err = json.Unmarshal(meta, &a); err != nil {
		return a, err
	}
	a.Payload = payload
	return a, nil
}
func (s *Store) Events(ctx context.Context, id string) ([]domain.AuditEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sequence,case_id,event_type,actor,occurred_at,revision,data_digest,previous_digest,digest,change_summary FROM audit_events WHERE case_id=? ORDER BY sequence`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []domain.AuditEvent{}
	for rows.Next() {
		var e domain.AuditEvent
		var at string
		if err = rows.Scan(&e.Sequence, &e.CaseID, &e.Type, &e.Actor, &at, &e.Revision, &e.DataDigest, &e.PreviousDigest, &e.Digest, &e.ChangeSummary); err != nil {
			return nil, err
		}
		e.OccurredAt, _ = time.Parse(time.RFC3339Nano, at)
		items = append(items, e)
	}
	return items, rows.Err()
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getCaseQuery(ctx context.Context, q queryer, id string) (*domain.SealCase, error) {
	var b []byte
	err := q.QueryRowContext(ctx, `SELECT snapshot FROM cases WHERE case_id=?`, id).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, &domain.BusinessError{Code: domain.CodeNotFound, Message: "个案不存在"}
	}
	if err != nil {
		return nil, err
	}
	var c domain.SealCase
	if err = json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
func getCaseTx(ctx context.Context, tx *sql.Tx, id string) (*domain.SealCase, error) {
	return getCaseQuery(ctx, tx, id)
}
func lookupIdempotency(ctx context.Context, tx *sql.Tx, id, fingerprint string) (*domain.SealCase, bool, error) {
	if id == "" {
		return nil, false, domain.Invalid("request_id 不能为空", nil)
	}
	var saved, response []byte
	err := tx.QueryRowContext(ctx, `SELECT fingerprint,response FROM idempotency WHERE request_id=?`, id).Scan(&saved, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if string(saved) != fingerprint {
		return nil, false, &domain.BusinessError{Code: domain.CodeIdempotency, Message: "request_id 已用于不同请求"}
	}
	var c domain.SealCase
	if err = json.Unmarshal(response, &c); err != nil {
		return nil, false, err
	}
	return &c, true, nil
}
func saveIdempotency(ctx context.Context, tx *sql.Tx, id, fingerprint, caseID string, response []byte) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO idempotency(request_id,fingerprint,case_id,response,created_at) VALUES(?,?,?,?,?)`, id, fingerprint, caseID, response, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
func appendEvent(ctx context.Context, tx *sql.Tx, c *domain.SealCase, eventType, actor string) error {
	changeSummary := ""
	if strings.HasPrefix(eventType, "baseline_revised:") {
		changeSummary = strings.TrimPrefix(eventType, "baseline_revised:")
		eventType = "baseline_revised"
	} else if strings.HasPrefix(eventType, "checkpoint_corrected:") {
		changeSummary = strings.TrimPrefix(eventType, "checkpoint_corrected:")
		eventType = "checkpoint_corrected"
	}
	var previous string
	_ = tx.QueryRowContext(ctx, `SELECT digest FROM audit_events WHERE case_id=? ORDER BY sequence DESC LIMIT 1`, c.CaseID).Scan(&previous)
	data := domain.MustDigest(c)
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `INSERT INTO audit_events(case_id,event_type,actor,occurred_at,revision,data_digest,previous_digest,digest,change_summary) VALUES(?,?,?,?,?,?,?,?,?)`, c.CaseID, eventType, actor, now.Format(time.RFC3339Nano), c.Revision, data, previous, "pending", changeSummary)
	if err != nil {
		return err
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return err
	}
	e := domain.AuditEvent{Sequence: seq, CaseID: c.CaseID, Type: eventType, ChangeSummary: changeSummary, Actor: actor, OccurredAt: now, Revision: c.Revision, DataDigest: data, PreviousDigest: previous}
	e.Digest = domain.EventDigest(e)
	_, err = tx.ExecContext(ctx, `UPDATE audit_events SET digest=? WHERE sequence=?`, e.Digest, seq)
	return err
}
