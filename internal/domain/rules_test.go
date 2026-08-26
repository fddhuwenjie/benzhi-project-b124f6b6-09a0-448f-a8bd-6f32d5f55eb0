package domain

import (
	"testing"
	"time"
)

func validCase() *SealCase {
	c := &SealCase{CaseID: "case-1", WellCode: "W-1", SiteName: "场地", Latitude: 30, Longitude: 120, TotalDepthM: 20, CasingDiameterMM: 100, OwnerName: "责任人", State: StateBaselineLocked}
	p := &SealPlan{PlanID: "plan-1", CaseID: c.CaseID, LayerSpecs: []LayerSpec{{SequenceNo: 1, DepthFromM: 0, DepthToM: 10, MaterialLot: "A", TargetVolumeL: 50, StageType: "placement"}, {SequenceNo: 2, DepthFromM: 10, DepthToM: 20, MaterialLot: "B", TargetVolumeL: 60, StageType: "placement"}}, MaterialLots: []string{"A", "B"}, VolumeTolerancePercent: 10, RequiredEvidenceTypes: []string{"photo"}, PreparedBy: "甲", ReviewedBy: "乙", LockedAt: time.Now()}
	p.Digest = MustDigest(p.LayerSpecs)
	c.Plan = p
	return c
}

func TestValidatePlanRequiresContinuousCoverageAndSeparation(t *testing.T) {
	c := validCase()
	if err := ValidatePlan(c, c.Plan); err != nil {
		t.Fatalf("有效方案被拒绝: %v", err)
	}
	c.Plan.ReviewedBy = "甲"
	if CodeOf(ValidatePlan(c, c.Plan)) != CodeGate {
		t.Fatal("未拦截编制人与复核人为同一人")
	}
	c.Plan.ReviewedBy = "乙"
	c.Plan.LayerSpecs[1].DepthFromM = 11
	if CodeOf(ValidatePlan(c, c.Plan)) != CodeGate {
		t.Fatal("未拦截不连续区间")
	}
}

func TestCheckpointQualityAndVerification(t *testing.T) {
	c := validCase()
	base := ConstructionCheckpoint{CaseID: c.CaseID, StageType: "placement", DepthFromM: 0, DepthToM: 10, MaterialLot: "A", ActualVolumeL: 50, RecordedBy: "施工员", EvidenceDigest: "digest-a", EvidenceTypes: []string{"photo"}, Measurements: []Measurement{{Name: "terminal_elevation", Value: 0, Unit: "m"}}, SequenceNo: 1}
	if _, err := CheckCheckpoint(c, base); err != nil {
		t.Fatalf("有效检查点被拒绝: %v", err)
	}
	bad := base
	bad.MaterialLot = "B"
	if category, err := CheckCheckpoint(c, bad); category != "material_mismatch" || err == nil {
		t.Fatal("材料错误未被分类")
	}
	c.Checkpoints = []ConstructionCheckpoint{base}
	v := Verify(c, time.Unix(100, 0))
	if v.Passed || len(v.Findings) != 1 {
		t.Fatalf("缺少分层时结论异常: %+v", v)
	}
	second := base
	second.SequenceNo, second.DepthFromM, second.DepthToM, second.MaterialLot, second.ActualVolumeL = 2, 10, 20, "B", 60
	c.Checkpoints = append(c.Checkpoints, second)
	v = Verify(c, time.Unix(100, 0))
	if !v.Passed || v.Digest == "" {
		t.Fatalf("完整检查点未通过验证: %+v", v)
	}
}
