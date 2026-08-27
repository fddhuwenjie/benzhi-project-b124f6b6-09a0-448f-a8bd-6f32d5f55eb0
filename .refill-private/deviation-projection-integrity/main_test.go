package deviationprojectionintegrity

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
	"wellseal/internal/application"
	"wellseal/internal/archive"
	"wellseal/internal/domain"
	"wellseal/internal/store"
)

func TestConsistencyDetectsCorruptedDeviationProjection(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cases.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := application.New(s, archive.NewBuilder())
	ctx := context.Background()
	v, err := app.Create(ctx, application.CreateCaseCommand{RequestID: "create", Actor: "建档", WellCode: "W", SiteName: "场地", TotalDepthM: 10, CasingDiameterMM: 100, OwnerName: "责任人"})
	if err != nil { t.Fatal(err) }
	v, err = app.LockBaseline(ctx, v.Case.CaseID, application.LockBaselineCommand{Meta: application.Meta{RequestID: "lock", ExpectedRevision: v.Case.Revision, Actor: "操作人"}})
	if err != nil { t.Fatal(err) }
	v, err = app.SetPlan(ctx, v.Case.CaseID, application.PlanCommand{Meta: application.Meta{RequestID: "plan", ExpectedRevision: v.Case.Revision, Actor: "复核人"}, LayerSpecs: []domain.LayerSpec{{SequenceNo: 1, DepthFromM: 0, DepthToM: 10, MaterialLot: "LOT", TargetVolumeL: 10, StageType: "placement"}}, MaterialLots: []string{"LOT"}, VolumeTolerancePercent: 10, RequiredEvidenceTypes: []string{"photo"}, PreparedBy: "编制人", ReviewedBy: "复核人"})
	if err != nil { t.Fatal(err) }
	v, err = app.Start(ctx, v.Case.CaseID, application.StartCommand{Meta: application.Meta{RequestID: "start", ExpectedRevision: v.Case.Revision, Actor: "施工人"}})
	if err != nil { t.Fatal(err) }
	_, err = app.Checkpoint(ctx, v.Case.CaseID, application.CheckpointCommand{Meta: application.Meta{RequestID: "bad", ExpectedRevision: v.Case.Revision, Actor: "施工人"}, StageType: "placement", DepthFromM: 0, DepthToM: 10, MaterialLot: "WRONG", ActualVolumeL: 10, RecordedBy: "施工人", EvidenceDigest: "e", EvidenceTypes: []string{"photo"}, SequenceNo: 1})
	if err != nil { t.Fatal(err) }

	db, err := sql.Open("sqlite", dbPath)
	if err != nil { t.Fatal(err) }
	defer db.Close()
	if _, err = db.Exec(`UPDATE deviations SET snapshot=? WHERE case_id=?`, []byte(`{"category":"tampered"}`), v.Case.CaseID); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckConsistency(ctx); err == nil {
		t.Fatal("偏差投影快照被篡改后完整性检查仍报告通过")
	}
}
