package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func ValidateBaseline(c *SealCase) error {
	f := map[string]string{}
	if strings.TrimSpace(c.WellCode) == "" {
		f["well_code"] = "不能为空"
	}
	if strings.TrimSpace(c.SiteName) == "" {
		f["site_name"] = "不能为空"
	}
	if strings.TrimSpace(c.OwnerName) == "" {
		f["owner_name"] = "不能为空"
	}
	if c.TotalDepthM <= 0 {
		f["total_depth_m"] = "必须大于 0"
	}
	if c.CasingDiameterMM <= 0 {
		f["casing_diameter_mm"] = "必须大于 0"
	}
	if c.Latitude < -90 || c.Latitude > 90 {
		f["latitude"] = "必须在 -90 到 90 之间"
	}
	if c.Longitude < -180 || c.Longitude > 180 {
		f["longitude"] = "必须在 -180 到 180 之间"
	}
	if len(f) > 0 {
		return Invalid("井况基线字段不完整", f)
	}
	return nil
}
func ValidatePlan(c *SealCase, p *SealPlan) error {
	if err := ValidateBaseline(c); err != nil {
		return err
	}
	preflight := PreflightPlan(c, *p)
	if !preflight.CanLock {
		fields := map[string]string{}
		gate := false
		for _, issue := range preflight.Issues {
			fields[issue.Field] = issue.Message
			if issue.Code == "role_separation" || issue.Code == "depth_gap" || issue.Code == "coverage_incomplete" {
				gate = true
			}
		}
		if gate {
			return Gate("方案预检存在职责分离或连续覆盖阻断问题")
		}
		return Invalid("方案预检存在阻断问题", fields)
	}
	*p = preflight.NormalizedPlan
	return nil
}
func CheckCheckpoint(c *SealCase, cp ConstructionCheckpoint) (string, error) {
	if c.Plan == nil {
		return "", Gate("施工方案尚未冻结")
	}
	if cp.SequenceNo < 1 || cp.SequenceNo > len(c.Plan.LayerSpecs) {
		return "depth_out_of_range", Gate("检查点序号不在方案内")
	}
	l := c.Plan.LayerSpecs[cp.SequenceNo-1]
	if cp.StageType != l.StageType {
		return "stage_mismatch", Gate("检查点施工阶段与方案不符")
	}
	if cp.DepthFromM != l.DepthFromM || cp.DepthToM != l.DepthToM {
		return "depth_out_of_range", Gate("检查点深度与方案不符")
	}
	if cp.MaterialLot != l.MaterialLot {
		return "material_mismatch", Gate("检查点材料批次与方案不符")
	}
	if cp.ActualVolumeL <= 0 {
		return "volume_out_of_tolerance", Gate("实际材料用量必须大于 0")
	}
	allowed := l.TargetVolumeL * c.Plan.VolumeTolerancePercent / 100
	if abs(cp.ActualVolumeL-l.TargetVolumeL) > allowed {
		return "volume_out_of_tolerance", Gate("实际材料用量超出方案容差")
	}
	if strings.TrimSpace(cp.RecordedBy) == "" {
		return "recorder_missing", Invalid("记录人不能为空", nil)
	}
	if strings.TrimSpace(cp.EvidenceDigest) == "" {
		return "evidence_missing", Gate("证据摘要不能为空")
	}
	have := map[string]bool{}
	for _, e := range cp.EvidenceTypes {
		have[e] = true
	}
	requiredTypes := l.RequiredEvidenceTypes
	if len(requiredTypes) == 0 {
		requiredTypes = c.Plan.RequiredEvidenceTypes
	}
	for _, required := range requiredTypes {
		if !have[required] {
			return "evidence_missing", Gate("缺少必需证据类型：" + required)
		}
	}
	hasElevation := false
	for _, measurement := range cp.Measurements {
		if strings.TrimSpace(measurement.Name) == "" || strings.TrimSpace(measurement.Unit) == "" {
			return "measurement_invalid", Invalid("结构化测量名称和单位不能为空", nil)
		}
		if measurement.Name == "terminal_elevation" && measurement.Unit == "m" {
			hasElevation = true
		}
	}
	if cp.SequenceNo == len(c.Plan.LayerSpecs) && !hasElevation {
		return "terminal_elevation_missing", Gate("最终区间缺少终孔标高测量")
	}
	return "", nil
}
func Verify(c *SealCase, now time.Time) VerificationResult {
	findings := []string{}
	if c.Plan == nil {
		findings = append(findings, "方案不存在")
	} else {
		bySeq := map[int]ConstructionCheckpoint{}
		for _, cp := range c.Checkpoints {
			bySeq[cp.SequenceNo] = cp
		}
		for _, layer := range c.Plan.LayerSpecs {
			cp, ok := bySeq[layer.SequenceNo]
			if !ok {
				findings = append(findings, fmt.Sprintf("缺少第 %d 层检查点", layer.SequenceNo))
				continue
			}
			if cat, err := CheckCheckpoint(c, cp); err != nil {
				findings = append(findings, fmt.Sprintf("第 %d 层不合格：%s（%s）", layer.SequenceNo, err, cat))
			}
		}
	}
	for _, d := range c.Deviations {
		if d.ClosedAt == nil {
			findings = append(findings, "存在未关闭偏差："+d.DeviationID)
		}
	}
	sort.Strings(findings)
	v := VerificationResult{Passed: len(findings) == 0, Findings: findings, VerifiedAt: now.UTC()}
	planDigest := ""
	if c.Plan != nil {
		planDigest = c.Plan.Digest
	}
	v.Digest = MustDigest(struct {
		CaseID      string                   `json:"case_id"`
		Plan        string                   `json:"plan_digest"`
		Checkpoints []ConstructionCheckpoint `json:"checkpoints"`
		Findings    []string                 `json:"findings"`
	}{c.CaseID, planDigest, NormalizedCheckpoints(c.Checkpoints), findings})
	return v
}
func ConstructionComplete(c *SealCase) bool {
	return c.Plan != nil && len(c.Checkpoints) == len(c.Plan.LayerSpecs)
}
func OpenDeviation(c *SealCase) bool {
	for _, d := range c.Deviations {
		if d.ClosedAt == nil {
			return true
		}
	}
	return false
}
func RecorderSet(c *SealCase) map[string]bool {
	out := map[string]bool{}
	for _, cp := range c.Checkpoints {
		out[cp.RecordedBy] = true
	}
	return out
}
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
