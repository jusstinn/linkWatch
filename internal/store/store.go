package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Store defines all DB operations
type Store interface {
	UpsertTargetByURL(ctx context.Context, canonicalURL, host string) (*Target, bool, error)
	GetTargets(ctx context.Context, hostFilter string, afterCreatedAt time.Time, afterID string, limit int) ([]*Target, *Cursor, error)
	InsertCheckResult(ctx context.Context, result *CheckResult) error
	GetResults(ctx context.Context, targetID string, since time.Time, limit int) ([]*CheckResult, error)
	UpsertIdempotencyKey(ctx context.Context, key, requestHash, targetID string, responseCode int, responseBody interface{}) (*IdempotencyResponse, bool, error)
	GetIdempotencyKey(ctx context.Context, key string) (*IdempotencyResponse, bool, error)
}

type Target struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Host      string    `json:"host"`
	CreatedAt time.Time `json:"created_at"`
}

type CheckResult struct {
	ID         int64     `json:"id"`
	TargetID   string    `json:"target_id"`
	CheckedAt  time.Time `json:"checked_at"`
	StatusCode *int      `json:"status_code"`
	LatencyMs  int       `json:"latency_ms"`
	Error      *string   `json:"error"`
}

type Cursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

type IdempotencyResponse struct {
	ResponseCode int         `json:"response_code"`
	ResponseBody interface{} `json:"response_body"`
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

const (
	qSelectTargetByURL = `
		SELECT id, url, host, created_at
		FROM targets
		WHERE url = ?`

	qInsertTarget = `
		INSERT INTO targets (id, url, host, created_at)
		VALUES (?, ?, ?, ?)`

	qSelectTargetsBase = `
		SELECT id, url, host, created_at
		FROM targets
		WHERE 1=1`

	qInsertCheckResult = `
		INSERT INTO check_results (target_id, checked_at, status_code, latency_ms, error)
		VALUES (?, ?, ?, ?, ?)`

	qSelectResults = `
		SELECT id, target_id, checked_at, status_code, latency_ms, error
		FROM check_results
		WHERE target_id = ? AND checked_at >= ?
		ORDER BY checked_at DESC
		LIMIT ?`

	qSelectIdempotency = `
		SELECT response_code, response_body
		FROM idempotency_keys
		WHERE key = ?`

	qInsertIdempotency = `
		INSERT INTO idempotency_keys (key, request_hash, target_id, response_code, response_body)
		VALUES (?, ?, ?, ?, ?)`
)

// UpsertTargetByURL returns existing or creates new target
func (s *SQLiteStore) UpsertTargetByURL(ctx context.Context, canonicalURL, host string) (*Target, bool, error) {
	var t Target
	var created string
	err := s.db.QueryRowContext(ctx, qSelectTargetByURL, canonicalURL).
		Scan(&t.ID, &t.URL, &t.Host, &created)

	if err == nil {
		t.CreatedAt = parseTime(created)
		return &t, false, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, fmt.Errorf("query target: %w", err)
	}

	t.ID = "t_" + generateID()
	t.URL = canonicalURL
	t.Host = host
	t.CreatedAt = time.Now()

	_, err = s.db.ExecContext(ctx, qInsertTarget,
		t.ID, t.URL, t.Host, formatTime(t.CreatedAt))
	if err != nil {
		return nil, false, fmt.Errorf("insert target: %w", err)
	}

	return &t, true, nil
}

// GetTargets fetches targets with filtering and pagination
func (s *SQLiteStore) GetTargets(ctx context.Context, hostFilter string, afterCreatedAt time.Time, afterID string, limit int) ([]*Target, *Cursor, error) {
	query := qSelectTargetsBase
	args := []any{}

	if hostFilter != "" {
		query += " AND host = ?"
		args = append(args, hostFilter)
	}
	if !afterCreatedAt.IsZero() {
		query += " AND (created_at > ? OR (created_at = ? AND id > ?))"
		ts := formatTime(afterCreatedAt)
		args = append(args, ts, ts, afterID)
	}
	query += " ORDER BY created_at, id LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("get targets: %w", err)
	}
	defer rows.Close()

	var targets []*Target
	for rows.Next() {
		var t Target
		var created string
		if err := rows.Scan(&t.ID, &t.URL, &t.Host, &created); err != nil {
			return nil, nil, err
		}
		t.CreatedAt = parseTime(created)
		targets = append(targets, &t)
	}

	if len(targets) == 0 {
		return nil, nil, nil
	}
	last := targets[len(targets)-1]
	cursor := &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}

	return targets, cursor, nil
}

// InsertCheckResult saves a check result
func (s *SQLiteStore) InsertCheckResult(ctx context.Context, r *CheckResult) error {
	_, err := s.db.ExecContext(ctx, qInsertCheckResult,
		r.TargetID, formatTime(r.CheckedAt), r.StatusCode, r.LatencyMs, r.Error)
	if err != nil {
		return fmt.Errorf("insert result: %w", err)
	}
	return nil
}

// GetResults fetches results for a target
func (s *SQLiteStore) GetResults(ctx context.Context, targetID string, since time.Time, limit int) ([]*CheckResult, error) {
	rows, err := s.db.QueryContext(ctx, qSelectResults,
		targetID, formatTime(since), limit)
	if err != nil {
		return nil, fmt.Errorf("get results: %w", err)
	}
	defer rows.Close()

	var results []*CheckResult
	for rows.Next() {
		var r CheckResult
		var checked string
		if err := rows.Scan(&r.ID, &r.TargetID, &checked, &r.StatusCode, &r.LatencyMs, &r.Error); err != nil {
			return nil, err
		}
		r.CheckedAt = parseTime(checked)
		results = append(results, &r)
	}
	return results, nil
}

func generateID() string {
	return uuid.NewString()
}

// UpsertIdempotencyKey stores or returns cached response
func (s *SQLiteStore) UpsertIdempotencyKey(ctx context.Context, key, requestHash, targetID string, responseCode int, responseBody interface{}) (*IdempotencyResponse, bool, error) {
	var resp IdempotencyResponse
	var rawBody string
	err := s.db.QueryRowContext(ctx, qSelectIdempotency, key).
		Scan(&resp.ResponseCode, &rawBody)
	if err == nil {
		_ = json.Unmarshal([]byte(rawBody), &resp.ResponseBody)
		return &resp, false, nil
	}
	if err != sql.ErrNoRows {
		return nil, false, fmt.Errorf("check idempotency: %w", err)
	}
	bodyJSON, _ := json.Marshal(responseBody)
	_, err = s.db.ExecContext(ctx, qInsertIdempotency,
		key, requestHash, targetID, responseCode, string(bodyJSON))
	if err != nil {
		return nil, false, fmt.Errorf("insert idempotency: %w", err)
	}

	return &IdempotencyResponse{ResponseCode: responseCode, ResponseBody: responseBody}, true, nil
}

// GetIdempotencyKey returns cached response if key exists
func (s *SQLiteStore) GetIdempotencyKey(ctx context.Context, key string) (*IdempotencyResponse, bool, error) {
	var resp IdempotencyResponse
	var rawBody string
	err := s.db.QueryRowContext(ctx, qSelectIdempotency, key).
		Scan(&resp.ResponseCode, &rawBody)

	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("check idempotency key: %w", err)
	}

	if err := json.Unmarshal([]byte(rawBody), &resp.ResponseBody); err != nil {
		return nil, false, fmt.Errorf("unmarshal cached response: %w", err)
	}

	return &resp, true, nil
}
