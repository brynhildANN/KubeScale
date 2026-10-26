// 数据库测试：使用临时 SQLite 文件验证策略版本冲突、更新结果和操作记录查询统计。
package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"kubescale/backend/internal/model"
)

// TestPolicyVersionAndOperationPersistence 验证策略版本递增、旧版本冲突，以及操作保存、分页和自动伸缩次数统计。
func TestPolicyVersionAndOperationPersistence(t *testing.T) {
	// 使用临时目录创建数据库，测试结束后自动清理，不影响运行中的数据。
	repo, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ctx := context.Background()
	policy := model.Policy{ID: "p1", Name: "demo", Namespace: "demo", WorkloadName: "api", Metric: "rps", TargetValue: 50,
		MinReplicas: 1, MaxReplicas: 8, ScaleUpConsecutivePeriods: 1, ScaleDownConsecutivePeriods: 1,
		SamplingIntervalSeconds: 15, Status: "enabled", Version: 1, UpdatedAt: time.Now().UTC()}
	if err := repo.CreatePolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	// 同一旧版本只能更新一次；第二次提交应触发版本冲突。
	policy.TargetValue = 60
	if err := repo.UpdatePolicy(ctx, policy, 1); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdatePolicy(ctx, policy, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	saved, err := repo.GetPolicy(ctx, "p1")
	if err != nil || saved.Version != 2 || saved.TargetValue != 60 {
		t.Fatalf("saved policy = %+v, %v", saved, err)
	}
	// 写入成功操作后，检查列表读取和自动伸缩次数统计。
	op := model.Operation{ID: "op1", Type: "自动扩容", Status: "succeeded", Namespace: "demo", WorkloadName: "api", CreatedAt: time.Now().UTC()}
	if err := repo.InsertOperation(ctx, op); err != nil {
		t.Fatal(err)
	}
	page, err := repo.ListOperations(ctx, "demo", "api", "", 1, 20)
	if err != nil || page.Total != 1 || page.Items[0].ID != "op1" {
		t.Fatalf("operations = %+v, %v", page, err)
	}
	count, err := repo.CountScaleActionsSince(ctx, "demo", time.Now().Add(-time.Hour))
	if err != nil || count != 1 {
		t.Fatalf("24h scale actions = %d, %v", count, err)
	}
}
