package contextcancellationpropagation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"wellseal/internal/domain"
	"wellseal/internal/store"
)

func TestCanceledContextDoesNotCommitApply(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "cancel.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	created := &domain.SealCase{
		CaseID:           "case-cancel",
		WellCode:         "W-1",
		SiteName:         "场地",
		TotalDepthM:      10,
		CasingDiameterMM: 100,
		OwnerName:        "责任人",
		State:            domain.StateDraft,
		Revision:         1,
		CreatedAt:        time.Now().UTC(),
		Checkpoints:      []domain.ConstructionCheckpoint{},
		Deviations:       []domain.Deviation{},
	}
	if _, _, err = s.Create(context.Background(), created, "create-cancel", "create-fingerprint", "建档"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	_, _, err = s.Apply(ctx, created.CaseID, created.Revision, "apply-cancel", "apply-fingerprint", "操作人", "baseline_locked", func(c *domain.SealCase) error {
		// 取消发生在事务已建立、业务回调执行期间；后续存储步骤必须仍观察到它。
		cancel()
		c.State = domain.StateBaselineLocked
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("取消上下文应向调用方传播 context.Canceled，实际错误: %v", err)
	}

	got, err := s.Get(context.Background(), created.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.StateDraft || got.Revision != created.Revision {
		t.Fatalf("取消请求不应提交变更，实际状态=%s 修订=%d", got.State, got.Revision)
	}
	events, err := s.Events(context.Background(), created.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("取消请求不应追加审计事件，实际事件数=%d", len(events))
	}
}
