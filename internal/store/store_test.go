package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"wellseal/internal/domain"
)

func TestRevisionIdempotencyAndAuditChain(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	c := &domain.SealCase{CaseID: "case-1", WellCode: "W", SiteName: "S", TotalDepthM: 10, CasingDiameterMM: 100, OwnerName: "O", State: domain.StateDraft, Revision: 1, CreatedAt: time.Now().UTC(), Checkpoints: []domain.ConstructionCheckpoint{}, Deviations: []domain.Deviation{}}
	created, replay, err := s.Create(ctx, c, "req-create", "fp-create", "甲")
	if err != nil || replay || created.Revision != 1 {
		t.Fatalf("创建异常: %+v %v", created, err)
	}
	changed, replay, err := s.Apply(ctx, c.CaseID, 1, "req-lock", "fp-lock", "甲", "baseline_locked", func(c *domain.SealCase) error { c.State = domain.StateBaselineLocked; return nil })
	if err != nil || replay || changed.Revision != 2 {
		t.Fatalf("更新异常: %+v %v", changed, err)
	}
	replayed, replay, err := s.Apply(ctx, c.CaseID, 99, "req-lock", "fp-lock", "甲", "ignored", func(c *domain.SealCase) error { return nil })
	if err != nil || !replay || replayed.Revision != 2 {
		t.Fatalf("幂等重放异常: %+v %v", replayed, err)
	}
	_, _, err = s.Apply(ctx, c.CaseID, 1, "req-other", "fp-other", "甲", "conflict", func(c *domain.SealCase) error { return nil })
	if domain.CodeOf(err) != domain.CodeConflict {
		t.Fatalf("未返回修订冲突: %v", err)
	}
	_, _, err = s.Apply(ctx, c.CaseID, 2, "req-lock", "different", "甲", "conflict", func(c *domain.SealCase) error { return nil })
	if domain.CodeOf(err) != domain.CodeIdempotency {
		t.Fatalf("未返回幂等指纹冲突: %v", err)
	}
	events, err := s.Events(ctx, c.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("事件数=%d", len(events))
	}
	previous := ""
	for _, e := range events {
		if e.PreviousDigest != previous || domain.EventDigest(e) != e.Digest {
			t.Fatal("审计链不连续")
		}
		previous = e.Digest
	}
}
