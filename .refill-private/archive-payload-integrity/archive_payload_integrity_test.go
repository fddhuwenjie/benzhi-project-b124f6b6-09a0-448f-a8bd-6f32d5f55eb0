package archive_payload_integrity

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
	"wellseal/internal/application"
	"wellseal/internal/archive"
	"wellseal/internal/domain"
	"wellseal/internal/store"
)

// TestArchiveVerificationDetectsTamperedBaselinePayload demonstrates that
// archive verification must reject a payload whose baseline facts were changed.
func TestArchiveVerificationDetectsTamperedBaselinePayload(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "archive.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := application.New(s, archive.NewBuilder())
	ctx := context.Background()

	v, err := app.Create(ctx, application.CreateCaseCommand{
		RequestID: "create", Actor: "建档", WellCode: "W-ORIGINAL", SiteName: "场地",
		TotalDepthM: 10, CasingDiameterMM: 100, OwnerName: "责任人",
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.LockBaseline(ctx, v.Case.CaseID, application.LockBaselineCommand{Meta: application.Meta{RequestID: "lock", ExpectedRevision: v.Case.Revision, Actor: "编制"}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.SetPlan(ctx, v.Case.CaseID, application.PlanCommand{
		Meta: application.Meta{RequestID: "plan", ExpectedRevision: v.Case.Revision, Actor: "复核"},
		LayerSpecs: []domain.LayerSpec{{SequenceNo: 1, DepthFromM: 0, DepthToM: 10, MaterialLot: "A", TargetVolumeL: 10, StageType: "placement"}},
		MaterialLots: []string{"A"}, VolumeTolerancePercent: 10, RequiredEvidenceTypes: []string{"photo"},
		PreparedBy: "编制", ReviewedBy: "复核",
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.Start(ctx, v.Case.CaseID, application.StartCommand{Meta: application.Meta{RequestID: "start", ExpectedRevision: v.Case.Revision, Actor: "施工"}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.Checkpoint(ctx, v.Case.CaseID, application.CheckpointCommand{
		Meta: application.Meta{RequestID: "checkpoint", ExpectedRevision: v.Case.Revision, Actor: "施工"},
		StageType: "placement", DepthFromM: 0, DepthToM: 10, MaterialLot: "A", ActualVolumeL: 10,
		RecordedBy: "施工", EvidenceDigest: "evidence", EvidenceTypes: []string{"photo"},
		Measurements: []domain.Measurement{{Name: "terminal_elevation", Unit: "m", Value: 1}}, SequenceNo: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.Verify(ctx, v.Case.CaseID, application.VerifyCommand{Meta: application.Meta{RequestID: "verify", ExpectedRevision: v.Case.Revision, Actor: "验证"}})
	if err != nil {
		t.Fatal(err)
	}
	checklist := []string{"plan", "verification", "deviations", "recorders", "event_chain"}
	_, err = app.Witness(ctx, v.Case.CaseID, application.WitnessCommand{
		Meta: application.Meta{RequestID: "witness", ExpectedRevision: v.Case.Revision, Actor: "见证"},
		Decision: "release", WitnessID: "独立见证人", DecisionNote: "确认", ConfirmedChecklist: checklist,
	})
	if err != nil {
		t.Fatal(err)
	}

	saved, err := s.Archive(ctx, v.Case.CaseID)
	if err != nil {
		t.Fatal(err)
	}
	var payload archive.Payload
	if err = json.Unmarshal(saved.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	payload.Case.WellCode = "W-TAMPERED"
	tampered, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	tampered = append(tampered, '\n')

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.ExecContext(ctx, `UPDATE archives SET payload=? WHERE case_id=?`, tampered, v.Case.CaseID); err != nil {
		t.Fatal(err)
	}

	if _, err = app.Archive(ctx, v.Case.CaseID); err == nil {
		t.Fatalf("篡改归档基线后校验仍然成功")
	}
}
