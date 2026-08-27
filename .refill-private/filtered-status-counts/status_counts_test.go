package filteredstatuscounts

import (
	"context"
	"path/filepath"
	"testing"

	"wellseal/internal/application"
	"wellseal/internal/archive"
	"wellseal/internal/domain"
	"wellseal/internal/store"
)

// TestFilteredListStatusCounts verifies that aggregate status counts use the
// same predicate as the filtered case list. It intentionally fails against
// the seeded implementation, which computes status_counts globally.
func TestFilteredListStatusCounts(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "cases.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := application.New(s, archive.NewBuilder())
	ctx := context.Background()

	draft, err := app.Create(ctx, application.CreateCaseCommand{
		RequestID: "create-draft", Actor: "建档人", WellCode: "W-DRAFT",
		SiteName: "甲场地", TotalDepthM: 10, CasingDiameterMM: 100, OwnerName: "责任人",
	})
	if err != nil {
		t.Fatal(err)
	}
	locked, err := app.Create(ctx, application.CreateCaseCommand{
		RequestID: "create-locked", Actor: "建档人", WellCode: "W-LOCKED",
		SiteName: "乙场地", TotalDepthM: 10, CasingDiameterMM: 100, OwnerName: "责任人",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = app.LockBaseline(ctx, locked.Case.CaseID, application.LockBaselineCommand{
		Meta: application.Meta{RequestID: "lock", ExpectedRevision: locked.Case.Revision, Actor: "复核人"},
	}); err != nil {
		t.Fatal(err)
	}

	result, err := app.List(ctx, application.CaseListFilter{State: domain.StateDraft, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].CaseID != draft.Case.CaseID {
		t.Fatalf("筛选列表异常: total=%d items=%v", result.Total, result.Items)
	}
	if result.StatusCounts[domain.StateDraft] != 1 || result.StatusCounts[domain.StateBaselineLocked] != 0 {
		t.Fatalf("筛选状态统计未与列表对齐: %+v", result.StatusCounts)
	}
}
