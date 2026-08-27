package cachealias

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"wellseal/internal/domain"
	"wellseal/internal/store"
)

func TestGetDoesNotExposeCachedAlias(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "alias.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	created := &domain.SealCase{
		CaseID: "case-alias", WellCode: "W-1", SiteName: "场地", OwnerName: "责任人",
		TotalDepthM: 10, CasingDiameterMM: 100, State: domain.StateDraft,
		Revision: 1, CreatedAt: time.Now().UTC(), Checkpoints: []domain.ConstructionCheckpoint{}, Deviations: []domain.Deviation{},
	}
	if _, _, err = s.Create(ctx, created, "create-alias", "fp-alias", "建档"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, created.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	got.State = domain.StateArchived
	got.Checkpoints = append(got.Checkpoints, domain.ConstructionCheckpoint{SequenceNo: 99})

	fresh, err := s.Get(ctx, created.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.State != domain.StateDraft || len(fresh.Checkpoints) != 0 {
		t.Fatalf("缓存返回了可修改别名: state=%s checkpoints=%d", fresh.State, len(fresh.Checkpoints))
	}
}
