package store

import (
	"context"
	"kubescale/backend/internal/model"
	"path/filepath"
	"testing"
	"time"
)

func TestAlertsPersistAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alerts.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a := model.Alert{ID: "cycle", Namespace: "demo", Status: "firing", StartsAt: time.Now()}
	if err := s.SaveAlerts(context.Background(), []model.Alert{a}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	items, err := s.Alerts(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != "cycle" {
		t.Fatalf("%v %v", items, err)
	}
}

// 批量写入后半段失败时，前半段也必须回滚。
func TestAlertsBatchRollback(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "rollback.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	_, err = s.db.Exec(`CREATE TRIGGER reject_bad BEFORE INSERT ON alerts WHEN NEW.id='bad' BEGIN SELECT RAISE(ABORT, 'test failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	err = s.SaveAlerts(context.Background(), []model.Alert{{ID: "good", Status: "firing"}, {ID: "bad", Status: "firing"}})
	if err == nil {
		t.Fatal("expected failure")
	}
	items, err := s.Alerts(context.Background())
	if err != nil || len(items) != 0 {
		t.Fatalf("partial commit: %v %v", items, err)
	}
}
