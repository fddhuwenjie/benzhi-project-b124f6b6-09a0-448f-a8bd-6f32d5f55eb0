package crosscaseidempotency

import (
	"context"
	"path/filepath"
	"testing"

	"wellseal/internal/application"
	"wellseal/internal/archive"
	"wellseal/internal/domain"
	"wellseal/internal/store"
)

func TestRequestIDMustNotReplayAcrossCases(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "cases.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := application.New(s, archive.NewBuilder())
	ctx := context.Background()
	create := func(id string) application.CaseView {
		v, err := app.Create(ctx, application.CreateCaseCommand{RequestID: id, Actor: "建档", WellCode: id, SiteName: "场地", TotalDepthM: 10, CasingDiameterMM: 100, OwnerName: "责任人"})
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	first, second := create("create-1"), create("create-2")
	cmd := application.LockBaselineCommand{Meta: application.Meta{RequestID: "shared-lock", ExpectedRevision: 1, Actor: "操作人"}}
	if _, err = app.LockBaseline(ctx, first.Case.CaseID, cmd); err != nil {
		t.Fatal(err)
	}
	got, err := app.LockBaseline(ctx, second.Case.CaseID, cmd)
	if domain.CodeOf(err) == domain.CodeIdempotency {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if got.Case == nil || got.Case.CaseID != second.Case.CaseID || got.Case.State != "baseline_locked" {
		t.Fatalf("跨个案幂等请求错误重放了其他个案: %+v", got.Case)
	}
}
