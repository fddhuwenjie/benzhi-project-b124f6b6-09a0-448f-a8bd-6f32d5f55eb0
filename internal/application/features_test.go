package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"wellseal/internal/archive"
	"wellseal/internal/domain"
	"wellseal/internal/store"
)

func TestCaseSearchCursorAndInvalidQueryAreReadOnly(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "list.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := New(s, archive.NewBuilder())
	app.now = func() time.Time { return time.Unix(100, 0).UTC() }
	ctx := context.Background()
	ids := map[string]bool{}
	for i, well := range []string{"A-1", "A-2", "B-1"} {
		v, createErr := app.Create(ctx, CreateCaseCommand{RequestID: well, Actor: "建档", WellCode: well, SiteName: map[bool]string{true: "目标场地", false: "其他场地"}[i < 2], TotalDepthM: 10, CasingDiameterMM: 100, OwnerName: "责任人"})
		if createErr != nil {
			t.Fatal(createErr)
		}
		ids[v.Case.CaseID] = true
	}
	seen := map[string]bool{}
	cursor := ""
	for {
		page, listErr := app.List(ctx, CaseListFilter{Keyword: "  目标场地 ", Limit: 1, Cursor: cursor})
		if listErr != nil {
			t.Fatal(listErr)
		}
		for _, c := range page.Items {
			if seen[c.CaseID] {
				t.Fatal("游标分页返回重复个案")
			}
			seen[c.CaseID] = true
		}
		cursor = page.NextCursor
		if cursor == "" {
			if page.Total != 2 || len(seen) != 2 {
				t.Fatalf("检索分页异常: total=%d seen=%d", page.Total, len(seen))
			}
			break
		}
	}
	firstID := ""
	for id := range ids {
		firstID = id
		break
	}
	before, _ := s.Events(ctx, firstID)
	from, to := time.Unix(200, 0), time.Unix(100, 0)
	if _, err = app.List(ctx, CaseListFilter{State: "unknown", CreatedFrom: &from, CreatedTo: &to}); domain.CodeOf(err) != domain.CodeInvalid {
		t.Fatalf("非法查询未返回字段错误: %v", err)
	}
	after, _ := s.Events(ctx, firstID)
	if len(before) != len(after) {
		t.Fatal("非法只读查询写入了审计事件")
	}
}

func TestBatchCheckpointIsAtomicAndIncrementsOnce(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "batch.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := New(s, archive.NewBuilder())
	ctx := context.Background()
	v, err := app.Create(ctx, CreateCaseCommand{RequestID: "create", Actor: "建档", WellCode: "W-1", SiteName: "场地", TotalDepthM: 20, CasingDiameterMM: 100, OwnerName: "责任人"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.LockBaseline(ctx, v.Case.CaseID, LockBaselineCommand{Meta: Meta{RequestID: "lock", ExpectedRevision: v.Case.Revision, Actor: "编制"}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.SetPlan(ctx, v.Case.CaseID, PlanCommand{Meta: Meta{RequestID: "plan", ExpectedRevision: v.Case.Revision, Actor: "复核"}, LayerSpecs: []domain.LayerSpec{{SequenceNo: 1, DepthFromM: 0, DepthToM: 10, MaterialLot: "A", TargetVolumeL: 50, StageType: "placement"}, {SequenceNo: 2, DepthFromM: 10, DepthToM: 20, MaterialLot: "B", TargetVolumeL: 60, StageType: "placement"}}, MaterialLots: []string{"A", "B"}, VolumeTolerancePercent: 10, RequiredEvidenceTypes: []string{"photo"}, PreparedBy: "编制", ReviewedBy: "复核"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = app.Start(ctx, v.Case.CaseID, StartCommand{Meta: Meta{RequestID: "start", ExpectedRevision: v.Case.Revision, Actor: "施工"}})
	if err != nil {
		t.Fatal(err)
	}
	before := v.Case.Revision
	items := []CheckpointItem{{StageType: "placement", DepthFromM: 0, DepthToM: 10, MaterialLot: "A", ActualVolumeL: 50, RecordedBy: "施工", EvidenceDigest: "d1", EvidenceTypes: []string{"photo"}, SequenceNo: 1}, {StageType: "placement", DepthFromM: 10, DepthToM: 20, MaterialLot: "B", ActualVolumeL: 60, RecordedBy: "施工", EvidenceDigest: "d2", EvidenceTypes: []string{"photo"}, Measurements: []domain.Measurement{{Name: "terminal_elevation", Unit: "m"}}, SequenceNo: 2}}
	v, err = app.Checkpoint(ctx, v.Case.CaseID, CheckpointCommand{Meta: Meta{RequestID: "batch", ExpectedRevision: before, Actor: "施工"}, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if v.Case.Revision != before+1 || len(v.Case.Checkpoints) != 2 || v.Progress.LayerCompletionPercent != 100 {
		t.Fatalf("批量结果异常: revision=%d checkpoints=%d progress=%+v", v.Case.Revision, len(v.Case.Checkpoints), v.Progress)
	}
	replayed, err := app.Checkpoint(ctx, v.Case.CaseID, CheckpointCommand{Meta: Meta{RequestID: "batch", ExpectedRevision: before, Actor: "施工"}, Items: items})
	if err != nil || !replayed.Replayed || len(replayed.Case.Checkpoints) != 2 {
		t.Fatalf("批量幂等重放异常: %+v %v", replayed, err)
	}
}

func TestBadBatchCreatesLocatedDeviationWithoutPartialFacts(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "bad.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	app := New(s, archive.NewBuilder())
	ctx := context.Background()
	v, _ := app.Create(ctx, CreateCaseCommand{RequestID: "c", Actor: "甲", WellCode: "W", SiteName: "S", TotalDepthM: 20, CasingDiameterMM: 100, OwnerName: "O"})
	v, _ = app.LockBaseline(ctx, v.Case.CaseID, LockBaselineCommand{Meta: Meta{RequestID: "l", ExpectedRevision: v.Case.Revision, Actor: "甲"}})
	v, _ = app.SetPlan(ctx, v.Case.CaseID, PlanCommand{Meta: Meta{RequestID: "p", ExpectedRevision: v.Case.Revision, Actor: "乙"}, LayerSpecs: []domain.LayerSpec{{SequenceNo: 1, DepthFromM: 0, DepthToM: 10, MaterialLot: "A", TargetVolumeL: 10, StageType: "placement"}, {SequenceNo: 2, DepthFromM: 10, DepthToM: 20, MaterialLot: "B", TargetVolumeL: 10, StageType: "placement"}}, MaterialLots: []string{"A", "B"}, VolumeTolerancePercent: 10, RequiredEvidenceTypes: []string{"photo"}, PreparedBy: "甲", ReviewedBy: "乙"})
	v, _ = app.Start(ctx, v.Case.CaseID, StartCommand{Meta: Meta{RequestID: "s", ExpectedRevision: v.Case.Revision, Actor: "丙"}})
	items := []CheckpointItem{{StageType: "placement", DepthFromM: 0, DepthToM: 10, MaterialLot: "A", ActualVolumeL: 10, RecordedBy: "丙", EvidenceDigest: "1", EvidenceTypes: []string{"photo"}, SequenceNo: 1}, {StageType: "placement", DepthFromM: 10, DepthToM: 20, MaterialLot: "WRONG", ActualVolumeL: 10, RecordedBy: "丙", EvidenceDigest: "2", EvidenceTypes: []string{"photo"}, Measurements: []domain.Measurement{{Name: "terminal_elevation", Unit: "m"}}, SequenceNo: 2}}
	v, err = app.Checkpoint(ctx, v.Case.CaseID, CheckpointCommand{Meta: Meta{RequestID: "bad", ExpectedRevision: v.Case.Revision, Actor: "丙"}, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if v.Case.State != domain.StateHeld || len(v.Case.Checkpoints) != 0 || len(v.Case.Deviations) != 1 || v.Case.Deviations[0].LayerSequenceNo == nil || *v.Case.Deviations[0].LayerSequenceNo != 2 {
		t.Fatalf("失败批次未保持原子语义: %+v", v.Case)
	}
}
