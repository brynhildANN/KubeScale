package store

import (
	"context"
	"encoding/json"
	"kubescale/backend/internal/model"
)

// SaveAlerts 原子提交一批通知；resolved 为同一周期的终态。
func (s *SQLite) SaveAlerts(ctx context.Context, alerts []model.Alert) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, a := range alerts {
		raw, err := json.Marshal(a)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO alerts(id,namespace,workload,status,severity,starts_at,document) VALUES(?,?,?,?,?,?,?)
   ON CONFLICT(id) DO UPDATE SET status=excluded.status,severity=excluded.severity,document=excluded.document WHERE alerts.status != 'resolved'`, a.ID, a.Namespace, a.WorkloadName, a.Status, a.Severity, a.StartsAt.UTC().Format(timestampFormat), string(raw))
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLite) Alerts(ctx context.Context) ([]model.Alert, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT document FROM alerts ORDER BY starts_at DESC,id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.Alert, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var a model.Alert
		if err := json.Unmarshal([]byte(raw), &a); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
