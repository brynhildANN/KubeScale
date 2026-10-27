// 持久化模块：将策略和操作详情保存为 JSON，使用独立列完成筛选、分页和策略版本检查。
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"kubescale/backend/internal/model"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("version conflict")

const timestampFormat = "2006-01-02T15:04:05.000000000Z"

// SQLite 策略与操作记录的 SQLite 存储实现。
type SQLite struct{ db *sql.DB }

// Open 打开 SQLite，启用 WAL 和忙等待，并创建策略表、操作表及索引。
func Open(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, statement := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		`CREATE TABLE IF NOT EXISTS alerts (id TEXT PRIMARY KEY,namespace TEXT NOT NULL,workload TEXT NOT NULL,status TEXT NOT NULL,severity TEXT NOT NULL,starts_at TEXT NOT NULL,document TEXT NOT NULL)`,
		"CREATE INDEX IF NOT EXISTS alerts_scope ON alerts(namespace,workload,status)",
		`CREATE TABLE IF NOT EXISTS policies (
			id TEXT PRIMARY KEY, namespace TEXT NOT NULL, workload TEXT NOT NULL,
			status TEXT NOT NULL, version INTEGER NOT NULL, document TEXT NOT NULL,
			UNIQUE(namespace, workload)
		)`,
		`CREATE TABLE IF NOT EXISTS operations (
			id TEXT PRIMARY KEY, namespace TEXT NOT NULL, workload TEXT NOT NULL,
			status TEXT NOT NULL, kind TEXT NOT NULL, created_at TEXT NOT NULL,
			document TEXT NOT NULL
		)`,
		"CREATE INDEX IF NOT EXISTS operations_created ON operations(created_at DESC)",
	} {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize sqlite: %w", err)
		}
	}
	return &SQLite{db: db}, nil
}

// Close 关闭数据库连接。
func (s *SQLite) Close() error { return s.db.Close() }

// CreatePolicy 保存策略；同一命名空间下的工作负载只能绑定一条策略。
func (s *SQLite) CreatePolicy(ctx context.Context, policy model.Policy) error {
	data, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO policies(id,namespace,workload,status,version,document) VALUES(?,?,?,?,?,?)",
		policy.ID, policy.Namespace, policy.WorkloadName, policy.Status, policy.Version, string(data))
	return err
}

// GetPolicy 按 ID 读取策略，不存在时返回 ErrNotFound。
func (s *SQLite) GetPolicy(ctx context.Context, id string) (model.Policy, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, "SELECT document FROM policies WHERE id=?", id).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Policy{}, ErrNotFound
		}
		return model.Policy{}, err
	}
	var policy model.Policy
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		return model.Policy{}, err
	}
	return policy, nil
}

// PolicyForWorkload 查询工作负载绑定的策略，未绑定时返回 nil。
func (s *SQLite) PolicyForWorkload(ctx context.Context, namespace, workload string) (*model.Policy, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT document FROM policies WHERE namespace=? AND workload=?", namespace, workload).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var policy model.Policy
	if err := json.Unmarshal([]byte(raw), &policy); err != nil {
		return nil, err
	}
	return &policy, nil
}

// ListPolicies 按可选状态筛选策略，并按命名空间和工作负载排序。
func (s *SQLite) ListPolicies(ctx context.Context, status string) ([]model.Policy, error) {
	query := "SELECT document FROM policies"
	args := []any{}
	if status != "" {
		query += " WHERE status=?"
		args = append(args, status)
	}
	query += " ORDER BY namespace,workload"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.Policy, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var p model.Policy
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			return nil, err
		}
		items = append(items, p)
	}
	return items, rows.Err()
}

// UpdatePolicy 仅在 expectedVersion 匹配时更新策略并递增版本，防止旧页面覆盖新数据。
func (s *SQLite) UpdatePolicy(ctx context.Context, policy model.Policy, expectedVersion int) error {
	policy.Version = expectedVersion + 1
	policy.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(policy)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, "UPDATE policies SET status=?,version=?,document=? WHERE id=? AND version=?",
		policy.Status, policy.Version, string(data), policy.ID, expectedVersion)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrConflict
	}
	return nil
}

// DeletePolicy 只删除未启用的策略；未删除任何记录时返回 ErrConflict。
func (s *SQLite) DeletePolicy(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, "DELETE FROM policies WHERE id=? AND status!='enabled'", id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrConflict
	}
	return nil
}

// InsertOperation 保存一次决策或手动伸缩记录，将空检查列表统一写为空数组。
func (s *SQLite) InsertOperation(ctx context.Context, operation model.Operation) error {
	if operation.Checks == nil {
		operation.Checks = []model.Check{}
	}
	data, err := json.Marshal(operation)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "INSERT INTO operations(id,namespace,workload,status,kind,created_at,document) VALUES(?,?,?,?,?,?,?)",
		operation.ID, operation.Namespace, operation.WorkloadName, operation.Status, operation.Type, operation.CreatedAt.UTC().Format(timestampFormat), string(data))
	return err
}

// GetOperation 按 ID 读取操作详情，不存在时返回 ErrNotFound。
func (s *SQLite) GetOperation(ctx context.Context, id string) (model.Operation, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, "SELECT document FROM operations WHERE id=?", id).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Operation{}, ErrNotFound
		}
		return model.Operation{}, err
	}
	var op model.Operation
	if err := json.Unmarshal([]byte(raw), &op); err != nil {
		return model.Operation{}, err
	}
	return op, nil
}

// ListOperations 按命名空间、工作负载和状态筛选操作，按创建时间倒序分页并返回总数。
func (s *SQLite) ListOperations(ctx context.Context, namespace, workload, status string, page, pageSize int) (model.Page[model.Operation], error) {
	where := " WHERE 1=1"
	args := []any{}
	if namespace != "" {
		where += " AND namespace=?"
		args = append(args, namespace)
	}
	if workload != "" {
		where += " AND workload=?"
		args = append(args, workload)
	}
	if status != "" {
		where += " AND status=?"
		args = append(args, status)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM operations"+where, args...).Scan(&total); err != nil {
		return model.Page[model.Operation]{}, err
	}
	queryArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, "SELECT document FROM operations"+where+" ORDER BY created_at DESC LIMIT ? OFFSET ?", queryArgs...)
	if err != nil {
		return model.Page[model.Operation]{}, err
	}
	defer rows.Close()
	items := make([]model.Operation, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return model.Page[model.Operation]{}, err
		}
		var op model.Operation
		if err := json.Unmarshal([]byte(raw), &op); err != nil {
			return model.Page[model.Operation]{}, err
		}
		items = append(items, op)
	}
	return model.Page[model.Operation]{Items: items, Total: total, Page: page, PageSize: pageSize}, rows.Err()
}

// LastSuccessfulScale 读取最近一次成功的自动或手动伸缩创建时间，用于冷却检查。
func (s *SQLite) LastSuccessfulScale(ctx context.Context, namespace, workload string) (*time.Time, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT created_at FROM operations WHERE namespace=? AND workload=? AND status='succeeded' AND kind IN ('自动扩容','自动缩容','手动伸缩') ORDER BY created_at DESC LIMIT 1`, namespace, workload).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	when, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, err
	}
	return &when, nil
}

// CountScaleActionsSince 统计指定时间以来成功的自动扩缩容次数，可按命名空间筛选。
func (s *SQLite) CountScaleActionsSince(ctx context.Context, namespace string, since time.Time) (int, error) {
	query := `SELECT COUNT(*) FROM operations WHERE status='succeeded' AND kind IN ('自动扩容','自动缩容') AND created_at>=?`
	args := []any{since.UTC().Format(timestampFormat)}
	if namespace != "" {
		query += " AND namespace=?"
		args = append(args, namespace)
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}
