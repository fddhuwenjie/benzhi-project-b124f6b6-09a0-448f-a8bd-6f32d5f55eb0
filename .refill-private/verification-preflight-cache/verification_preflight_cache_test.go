package verificationpreflightcache

import (
	"context"
	"path/filepath"
	"testing"

	"wellseal/internal/application"
	"wellseal/internal/archive"
	"wellseal/internal/domain"
	"wellseal/internal/store"
)

func TestVerificationPreflightRefreshesAfterRevisionChange(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "preflight.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	app := application.New(db, archive.NewBuilder())

	view, err := app.Create(ctx, application.CreateCaseCommand{
		RequestID: "create", Actor: "建档人", WellCode: "W-CACHE", SiteName: "测试场地",
		TotalDepthM: 10, CasingDiameterMM: 100, OwnerName: "责任人",
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err = app.LockBaseline(ctx, view.Case.CaseID, application.LockBaselineCommand{Meta: application.Meta{
		RequestID: "lock", ExpectedRevision: view.Case.Revision, Actor: "编制人",
	}})
	if err != nil {
		t.Fatal(err)
	}
	view, err = app.SetPlan(ctx, view.Case.CaseID, application.PlanCommand{
		Meta: application.Meta{RequestID: "plan", ExpectedRevision: view.Case.Revision, Actor: "复核人"},
		LayerSpecs: []domain.LayerSpec{{
			SequenceNo: 1, DepthFromM: 0, DepthToM: 10, MaterialLot: "LOT-1", TargetVolumeL: 50, StageType: "placement",
		}},
		MaterialLots: []string{"LOT-1"}, VolumeTolerancePercent: 10,
		RequiredEvidenceTypes: []string{"photo"}, PreparedBy: "编制人", ReviewedBy: "复核人",
	})
	if err != nil {
		t.Fatal(err)
	}
	view, err = app.Start(ctx, view.Case.CaseID, application.StartCommand{Meta: application.Meta{
		RequestID: "start", ExpectedRevision: view.Case.Revision, Actor: "施工人",
	}})
	if err != nil {
		t.Fatal(err)
	}

	before, err := app.VerificationPreflight(ctx, view.Case.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Passed {
		t.Fatal("未登记检查点时预检不应通过")
	}

	view, err = app.Checkpoint(ctx, view.Case.CaseID, application.CheckpointCommand{
		Meta:      application.Meta{RequestID: "checkpoint", ExpectedRevision: view.Case.Revision, Actor: "施工人"},
		StageType: "placement", DepthFromM: 0, DepthToM: 10, MaterialLot: "LOT-1", ActualVolumeL: 50,
		RecordedBy: "施工人", EvidenceDigest: "sha256:evidence", EvidenceTypes: []string{"photo"}, SequenceNo: 1,
		Measurements: []domain.Measurement{{Name: "terminal_elevation", Unit: "m", Value: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := app.VerificationPreflight(ctx, view.Case.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	if after.SourceRevision != view.Case.Revision || !after.Passed {
		t.Fatalf("施工修订后仍返回过期预检: got revision=%d passed=%v, want revision=%d passed=true", after.SourceRevision, after.Passed, view.Case.Revision)
	}
}
