package domain

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const MaxBatchCheckpoints = 25

type PlanIssue struct {
	Code       string `json:"code"`
	Field      string `json:"field"`
	SequenceNo int    `json:"sequence_no,omitempty"`
	Blocking   bool   `json:"blocking"`
	Message    string `json:"message"`
}

type LayerCalculation struct {
	SequenceNo        int     `json:"sequence_no"`
	TheoreticalL      float64 `json:"theoretical_volume_l"`
	TargetVolumeL     float64 `json:"target_volume_l"`
	DifferenceL       float64 `json:"difference_l"`
	DifferencePercent float64 `json:"difference_percent"`
}

type MaterialPlanTotal struct {
	MaterialLot   string  `json:"material_lot"`
	TargetVolumeL float64 `json:"target_volume_l"`
}

type PlanPreflight struct {
	NormalizedPlan SealPlan            `json:"normalized_plan"`
	Layers         []LayerCalculation  `json:"layers"`
	MaterialTotals []MaterialPlanTotal `json:"material_totals"`
	Issues         []PlanIssue         `json:"issues"`
	CanLock        bool                `json:"can_lock"`
	Digest         string              `json:"digest"`
}

func PreflightPlan(c *SealCase, input SealPlan) PlanPreflight {
	p := NormalizedPlan(input)
	issues := make([]PlanIssue, 0)
	add := func(code, field string, seq int, msg string) {
		issues = append(issues, PlanIssue{Code: code, Field: field, SequenceNo: seq, Blocking: true, Message: msg})
	}
	if strings.TrimSpace(p.PreparedBy) == "" {
		add("prepared_by_required", "prepared_by", 0, "编制人不能为空")
	}
	if strings.TrimSpace(p.ReviewedBy) == "" {
		add("reviewed_by_required", "reviewed_by", 0, "复核人不能为空")
	}
	if strings.TrimSpace(p.PreparedBy) != "" && p.PreparedBy == p.ReviewedBy {
		add("role_separation", "reviewed_by", 0, "方案复核人必须不同于编制人")
	}
	if p.VolumeTolerancePercent < 0 || p.VolumeTolerancePercent > 50 {
		add("tolerance_invalid", "volume_tolerance_percent", 0, "用量容差必须在 0 到 50 之间")
	}
	if len(p.LayerSpecs) == 0 {
		add("layers_required", "layer_specs", 0, "至少需要一个封填分层")
	}

	registered := map[string]int{}
	for i, raw := range input.MaterialLots {
		lot := strings.TrimSpace(raw)
		field := fmt.Sprintf("material_lots[%d]", i)
		if lot == "" {
			add("material_lot_empty", field, 0, "材料批次不能为空")
			continue
		}
		registered[lot]++
		if registered[lot] > 1 {
			add("material_lot_duplicate", field, 0, "材料批次重复："+lot)
		}
	}
	evidenceSeen := map[string]bool{}
	for i, raw := range input.RequiredEvidenceTypes {
		e := strings.TrimSpace(raw)
		field := fmt.Sprintf("required_evidence_types[%d]", i)
		if e == "" {
			add("evidence_type_empty", field, 0, "证据类型不能为空")
			continue
		}
		if evidenceSeen[e] {
			add("evidence_type_duplicate", field, 0, "证据类型重复："+e)
		}
		evidenceSeen[e] = true
	}
	if len(evidenceSeen) == 0 {
		add("evidence_required", "required_evidence_types", 0, "必须指定证据类型")
	}

	used := map[string]float64{}
	calcs := make([]LayerCalculation, 0, len(p.LayerSpecs))
	for i, layer := range p.LayerSpecs {
		field := fmt.Sprintf("layer_specs[%d]", i)
		if layer.SequenceNo != i+1 {
			add("sequence_discontinuous", field+".sequence_no", layer.SequenceNo, "分层序号必须从 1 连续递增")
		}
		if layer.DepthFromM < 0 || layer.DepthToM <= layer.DepthFromM || layer.DepthToM > c.TotalDepthM {
			add("depth_invalid", field+".depth_to_m", layer.SequenceNo, "分层深度区间无效")
		}
		if i > 0 && p.LayerSpecs[i-1].DepthToM != layer.DepthFromM {
			add("depth_gap", field+".depth_from_m", layer.SequenceNo, "分层深度必须连续且不得重叠")
		}
		if strings.TrimSpace(layer.StageType) == "" {
			add("stage_required", field+".stage_type", layer.SequenceNo, "施工阶段不能为空")
		}
		if layer.TargetVolumeL <= 0 {
			add("target_volume_invalid", field+".target_volume_l", layer.SequenceNo, "目标用量必须大于 0")
		}
		lot := strings.TrimSpace(layer.MaterialLot)
		if registered[lot] == 0 {
			add("material_lot_unregistered", field+".material_lot", layer.SequenceNo, "分层材料批次未登记："+lot)
		}
		used[lot] += layer.TargetVolumeL
		span := math.Max(0, layer.DepthToM-layer.DepthFromM)
		radiusM := c.CasingDiameterMM / 2000
		theoretical := round(math.Pi*radiusM*radiusM*span*1000, 3)
		difference := round(layer.TargetVolumeL-theoretical, 3)
		percent := 0.0
		if theoretical > 0 {
			percent = round(difference/theoretical*100, 2)
		}
		calcs = append(calcs, LayerCalculation{layer.SequenceNo, theoretical, round(layer.TargetVolumeL, 3), difference, percent})
	}
	if len(p.LayerSpecs) > 0 && (p.LayerSpecs[0].DepthFromM != 0 || p.LayerSpecs[len(p.LayerSpecs)-1].DepthToM != c.TotalDepthM) {
		add("coverage_incomplete", "layer_specs", 0, "方案必须覆盖从地表到井底的完整深度")
	}
	for lot := range registered {
		if used[lot] == 0 {
			add("material_lot_unused", "material_lots", 0, "登记批次未被分层引用："+lot)
		}
	}
	totals := make([]MaterialPlanTotal, 0, len(used))
	for lot, total := range used {
		if lot != "" {
			totals = append(totals, MaterialPlanTotal{lot, round(total, 3)})
		}
	}
	sort.Slice(totals, func(i, j int) bool { return totals[i].MaterialLot < totals[j].MaterialLot })
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].SequenceNo != issues[j].SequenceNo {
			return issues[i].SequenceNo < issues[j].SequenceNo
		}
		return issues[i].Field < issues[j].Field
	})
	out := PlanPreflight{NormalizedPlan: p, Layers: calcs, MaterialTotals: totals, Issues: issues, CanLock: len(issues) == 0}
	out.Digest = MustDigest(struct {
		Plan   SealPlan            `json:"plan"`
		Layers []LayerCalculation  `json:"layers"`
		Totals []MaterialPlanTotal `json:"material_totals"`
		Issues []PlanIssue         `json:"issues"`
	}{p, calcs, totals, issues})
	return out
}

type LayerProgress struct {
	SequenceNo               int                     `json:"sequence_no"`
	Status                   string                  `json:"status"`
	DepthM                   float64                 `json:"depth_m"`
	Checkpoint               *ConstructionCheckpoint `json:"checkpoint,omitempty"`
	MissingEvidenceTypes     []string                `json:"missing_evidence_types"`
	TerminalElevationPresent bool                    `json:"terminal_elevation_present"`
}
type MaterialProgress struct {
	MaterialLot       string  `json:"material_lot"`
	TargetVolumeL     float64 `json:"target_volume_l"`
	ActualVolumeL     float64 `json:"actual_volume_l"`
	DifferenceL       float64 `json:"difference_l"`
	DifferencePercent float64 `json:"difference_percent"`
	Marker            string  `json:"marker"`
}
type ProgressSummary struct {
	Computable             bool               `json:"computable"`
	Reason                 string             `json:"reason,omitempty"`
	SourceRevision         int64              `json:"source_revision"`
	CompletedLayers        int                `json:"completed_layers"`
	TotalLayers            int                `json:"total_layers"`
	LayerCompletionPercent float64            `json:"layer_completion_percent"`
	CompletedDepthM        float64            `json:"completed_depth_m"`
	RemainingDepthM        float64            `json:"remaining_depth_m"`
	DepthCompletionPercent float64            `json:"depth_completion_percent"`
	Layers                 []LayerProgress    `json:"layers"`
	Materials              []MaterialProgress `json:"materials"`
}

func SummarizeProgress(c *SealCase) ProgressSummary {
	out := ProgressSummary{SourceRevision: c.Revision, Layers: []LayerProgress{}, Materials: []MaterialProgress{}}
	if c.Plan == nil {
		out.Reason = "方案未锁定"
		return out
	}
	out.Computable = true
	bySeq := map[int]ConstructionCheckpoint{}
	for _, cp := range c.Checkpoints {
		bySeq[cp.SequenceNo] = cp
	}
	blocked := map[int]bool{}
	for _, d := range c.Deviations {
		if d.ClosedAt == nil && d.LayerSequenceNo != nil {
			blocked[*d.LayerSequenceNo] = true
		}
	}
	targets, actuals := map[string]float64{}, map[string]float64{}
	for _, layer := range c.Plan.LayerSpecs {
		span := layer.DepthToM - layer.DepthFromM
		lp := LayerProgress{SequenceNo: layer.SequenceNo, Status: "pending", DepthM: round(span, 3), MissingEvidenceTypes: []string{}}
		targets[layer.MaterialLot] += layer.TargetVolumeL
		if cp, ok := bySeq[layer.SequenceNo]; ok {
			copy := cp
			lp.Checkpoint = &copy
			lp.Status = "passed"
			out.CompletedLayers++
			out.CompletedDepthM += span
			actuals[cp.MaterialLot] += cp.ActualVolumeL
			required := layer.RequiredEvidenceTypes
			if len(required) == 0 {
				required = c.Plan.RequiredEvidenceTypes
			}
			have := map[string]bool{}
			for _, e := range cp.EvidenceTypes {
				have[e] = true
			}
			for _, e := range required {
				if !have[e] {
					lp.MissingEvidenceTypes = append(lp.MissingEvidenceTypes, e)
				}
			}
			for _, m := range cp.Measurements {
				if m.Name == "terminal_elevation" && m.Unit == "m" {
					lp.TerminalElevationPresent = true
				}
			}
		}
		if blocked[layer.SequenceNo] {
			lp.Status = "blocked"
		}
		out.Layers = append(out.Layers, lp)
	}
	out.TotalLayers = len(c.Plan.LayerSpecs)
	totalDepth := c.TotalDepthM
	out.CompletedDepthM = round(out.CompletedDepthM, 3)
	out.RemainingDepthM = round(math.Max(0, totalDepth-out.CompletedDepthM), 3)
	if out.TotalLayers > 0 {
		out.LayerCompletionPercent = round(float64(out.CompletedLayers)*100/float64(out.TotalLayers), 2)
	}
	if totalDepth > 0 {
		out.DepthCompletionPercent = round(out.CompletedDepthM*100/totalDepth, 2)
	}
	for lot, target := range targets {
		actual := actuals[lot]
		diff := round(actual-target, 3)
		pct := 0.0
		if target > 0 {
			pct = round(diff/target*100, 2)
		}
		marker := "normal"
		ratio := abs(pct)
		if ratio > c.Plan.VolumeTolerancePercent {
			marker = "exceeded"
		} else if c.Plan.VolumeTolerancePercent > 0 && ratio >= c.Plan.VolumeTolerancePercent*0.8 {
			marker = "near_limit"
		}
		out.Materials = append(out.Materials, MaterialProgress{lot, round(target, 3), round(actual, 3), diff, pct, marker})
	}
	sort.Slice(out.Materials, func(i, j int) bool { return out.Materials[i].MaterialLot < out.Materials[j].MaterialLot })
	return out
}

type VerificationReport struct {
	Passed         bool                `json:"passed"`
	SourceRevision int64               `json:"source_revision"`
	PlanDigest     string              `json:"plan_digest"`
	Checks         []VerificationCheck `json:"checks"`
	Digest         string              `json:"digest"`
}

func BuildVerificationReport(c *SealCase) VerificationReport {
	r := VerificationReport{SourceRevision: c.Revision, Checks: []VerificationCheck{}}
	add := func(group, code string, passed bool, seq int, msg string) {
		r.Checks = append(r.Checks, VerificationCheck{group, code, passed, seq, msg})
	}
	if c.Plan == nil {
		add("plan", "plan_locked", false, 0, "方案未锁定")
	} else {
		r.PlanDigest = c.Plan.Digest
		bySeq := map[int]ConstructionCheckpoint{}
		for _, cp := range c.Checkpoints {
			bySeq[cp.SequenceNo] = cp
		}
		for _, layer := range c.Plan.LayerSpecs {
			cp, ok := bySeq[layer.SequenceNo]
			if !ok {
				add("interval", "checkpoint_present", false, layer.SequenceNo, fmt.Sprintf("缺少第 %d 层检查点", layer.SequenceNo))
				add("material", "volume_tolerance", false, layer.SequenceNo, "无法核对材料用量")
				add("evidence", "required_evidence", false, layer.SequenceNo, "必需证据未登记")
				if layer.SequenceNo == len(c.Plan.LayerSpecs) {
					add("elevation", "terminal_elevation", false, layer.SequenceNo, "终孔标高未登记")
				}
				continue
			}
			category, err := CheckCheckpoint(c, cp)
			add("interval", "checkpoint_valid", err == nil || category == "volume_out_of_tolerance" || category == "evidence_missing" || category == "terminal_elevation_missing", layer.SequenceNo, checkMessage(err, "区间与阶段匹配"))
			allowed := layer.TargetVolumeL * c.Plan.VolumeTolerancePercent / 100
			volumeOK := abs(cp.ActualVolumeL-layer.TargetVolumeL) <= allowed
			add("material", "volume_tolerance", volumeOK, layer.SequenceNo, fmt.Sprintf("目标 %.3f L，实际 %.3f L", layer.TargetVolumeL, cp.ActualVolumeL))
			required := layer.RequiredEvidenceTypes
			if len(required) == 0 {
				required = c.Plan.RequiredEvidenceTypes
			}
			have := map[string]bool{}
			for _, e := range cp.EvidenceTypes {
				have[e] = true
			}
			missing := []string{}
			for _, e := range required {
				if !have[e] {
					missing = append(missing, e)
				}
			}
			add("evidence", "required_evidence", len(missing) == 0, layer.SequenceNo, missingMessage(missing))
			if layer.SequenceNo == len(c.Plan.LayerSpecs) {
				has := false
				for _, m := range cp.Measurements {
					if m.Name == "terminal_elevation" && m.Unit == "m" {
						has = true
					}
				}
				add("elevation", "terminal_elevation", has, layer.SequenceNo, map[bool]string{true: "终孔标高已登记", false: "终孔标高未登记"}[has])
			}
		}
	}
	open := 0
	for _, d := range c.Deviations {
		if d.ClosedAt == nil {
			open++
		}
	}
	add("deviation", "open_deviations", open == 0, 0, fmt.Sprintf("开放偏差 %d 项", open))
	r.Passed = true
	for _, c := range r.Checks {
		if !c.Passed {
			r.Passed = false
			break
		}
	}
	r.Digest = MustDigest(struct {
		CaseID   string              `json:"case_id"`
		Revision int64               `json:"revision"`
		Plan     string              `json:"plan_digest"`
		Checks   []VerificationCheck `json:"checks"`
	}{c.CaseID, r.SourceRevision, r.PlanDigest, r.Checks})
	return r
}

func BuildWitnessChecklist(c *SealCase, eventChainHead string) []WitnessChecklistItem {
	items := []WitnessChecklistItem{
		{Code: "plan", Required: true, SourceDigest: digestOfPlan(c), Description: "冻结方案与复核职责"},
		{Code: "verification", Required: true, SourceDigest: digestOfVerification(c), Description: "完整性验证逐层报告"},
		{Code: "deviations", Required: true, SourceDigest: MustDigest(openDeviationIDs(c)), Description: "开放偏差必须为零"},
		{Code: "recorders", Required: true, SourceDigest: MustDigest(sortedKeys(RecorderSet(c))), Description: "施工记录人集合"},
		{Code: "event_chain", Required: true, SourceDigest: eventChainHead, Description: "追加式审计事件链"},
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Code < items[j].Code })
	return items
}

func DeviationSeverity(category string) string {
	switch category {
	case "depth_out_of_range", "witness_return":
		return "critical"
	case "material_mismatch", "volume_out_of_tolerance":
		return "major"
	case "evidence_missing", "terminal_elevation_missing":
		return "moderate"
	default:
		return "minor"
	}
}

func ValidateDeviationResolution(d Deviation, corrective, result, closedBy string, values map[string]float64) error {
	fields := map[string]string{}
	if strings.TrimSpace(corrective) == "" {
		fields["corrective_action"] = "不能为空"
	}
	if result != "passed" && result != "failed" {
		fields["retest_result"] = "必须为 passed 或 failed"
	}
	if strings.TrimSpace(closedBy) == "" {
		fields["closed_by"] = "不能为空"
	}
	if result == "passed" {
		switch d.Category {
		case "depth_out_of_range", "terminal_elevation_missing":
			if _, ok := values["measured_depth_m"]; !ok {
				fields["retest_values.measured_depth_m"] = "该类别必须登记复测深度"
			}
		case "material_mismatch":
			if _, ok := values["material_lot_verified"]; !ok {
				fields["retest_values.material_lot_verified"] = "该类别必须登记材料批次复验值"
			}
		case "volume_out_of_tolerance":
			if _, ok := values["actual_volume_l"]; !ok {
				fields["retest_values.actual_volume_l"] = "该类别必须登记复测用量"
			}
		case "evidence_missing", "witness_return":
			if _, ok := values["evidence_verified"]; !ok {
				fields["retest_values.evidence_verified"] = "该类别必须登记证据复验值"
			}
		}
	}
	if len(fields) > 0 {
		return Invalid("偏差处置字段不完整", fields)
	}
	return nil
}

// SummarizeDeviations 按稳定键顺序汇总偏差分组及逐条复验指标。
func SummarizeDeviations(c *SealCase, open *bool, from, to *time.Time, now time.Time) DeviationSummary {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	out := DeviationSummary{Groups: []DeviationSummaryGroup{}}
	groups := map[string]*DeviationSummaryGroup{}
	for _, d := range c.Deviations {
		isOpen := d.ClosedAt == nil
		if open != nil && isOpen != *open {
			continue
		}
		if from != nil && d.DetectedAt.Before(*from) {
			continue
		}
		if to != nil && d.DetectedAt.After(*to) {
			continue
		}
		out.Total++
		if isOpen {
			out.Open++
			out.OpenCount++
		} else {
			out.Closed++
			out.ClosedCount++
		}
		key := d.Category + "|" + d.Severity
		g := groups[key]
		if g == nil {
			g = &DeviationSummaryGroup{Key: key, Category: d.Category, Severity: d.Severity, Items: []DeviationSummaryItem{}}
			groups[key] = g
		}
		g.Total++
		if isOpen {
			g.Open++
			g.OpenCount++
		} else {
			g.Closed++
			g.ClosedCount++
		}
		if g.EarliestDetectedAt == nil || d.DetectedAt.Before(*g.EarliestDetectedAt) {
			t := d.DetectedAt
			g.EarliestDetectedAt = &t
		}
		if g.LatestDetectedAt == nil || d.DetectedAt.After(*g.LatestDetectedAt) {
			t := d.DetectedAt
			g.LatestDetectedAt = &t
		}
		end := now
		if d.ClosedAt != nil {
			end = *d.ClosedAt
		}
		hours := end.Sub(d.DetectedAt).Hours()
		if hours < 0 {
			hours = 0
		}
		item := DeviationSummaryItem{DeviationID: d.DeviationID, Category: d.Category, Severity: d.Severity, DetectedAt: d.DetectedAt, ClosedAt: d.ClosedAt, Open: isOpen, DispositionHours: round(hours, 2), DispositionDurationHours: round(hours, 2)}
		var latestTime time.Time
		for _, disp := range d.Dispositions {
			if disp.RetestResult == "failed" {
				item.FailedRetests++
			}
			if disp.RetestResult == "passed" {
				item.PassedRetests++
			}
			if latestTime.IsZero() || disp.OccurredAt.After(latestTime) {
				latestTime = disp.OccurredAt
				item.LatestRetest = disp.RetestResult
				item.LatestRetestResult = disp.RetestResult
			}
		}
		g.Items = append(g.Items, item)
	}
	for _, g := range groups {
		if g.Total > 0 {
			var totalHours float64
			for _, item := range g.Items {
				totalHours += item.DispositionHours
			}
			g.AverageDispositionHours = round(totalHours/float64(g.Total), 2)
		}
		sort.Slice(g.Items, func(i, j int) bool { return g.Items[i].DeviationID < g.Items[j].DeviationID })
		out.Groups = append(out.Groups, *g)
	}
	sort.Slice(out.Groups, func(i, j int) bool { return out.Groups[i].Key < out.Groups[j].Key })
	return out
}

// SummarizeQuality 聚合一致性快照，输出固定状态键和稳定偏差分组。
func SummarizeQuality(cases []SealCase, queriedAt time.Time, revisionUpper int64) QualitySummary {
	if queriedAt.IsZero() {
		queriedAt = time.Now().UTC()
	}
	out := QualitySummary{TotalCases: len(cases), StatusCounts: map[State]int{}, QueriedAt: queriedAt.UTC(), DataRevisionUpperBound: revisionUpper, DeviationGroups: []DeviationSummaryGroup{}}
	for _, st := range []State{StateDraft, StateBaselineLocked, StateSealing, StateHeld, StateVerification, StateReleased, StateArchived} {
		out.StatusCounts[st] = 0
	}
	groups := map[string]*DeviationSummaryGroup{}
	for _, c := range cases {
		out.StatusCounts[c.State]++
		if c.State == StateArchived {
			out.VerificationCounts.Archived++
		} else if c.Verification == nil {
			out.VerificationCounts.Unverified++
		} else if c.Verification.Passed {
			out.VerificationCounts.Passed++
		} else {
			out.VerificationCounts.Failed++
		}
		for _, d := range c.Deviations {
			key := d.Category + "|" + d.Severity
			g := groups[key]
			if g == nil {
				g = &DeviationSummaryGroup{Key: key, Category: d.Category, Severity: d.Severity, Items: []DeviationSummaryItem{}}
				groups[key] = g
			}
			open := d.ClosedAt == nil
			g.Total++
			if open {
				g.Open++
				g.OpenCount++
			} else {
				g.Closed++
				g.ClosedCount++
			}
			if g.EarliestDetectedAt == nil || d.DetectedAt.Before(*g.EarliestDetectedAt) {
				t := d.DetectedAt
				g.EarliestDetectedAt = &t
			}
			if g.LatestDetectedAt == nil || d.DetectedAt.After(*g.LatestDetectedAt) {
				t := d.DetectedAt
				g.LatestDetectedAt = &t
			}
			end := queriedAt
			if d.ClosedAt != nil {
				end = *d.ClosedAt
			}
			hours := end.Sub(d.DetectedAt).Hours()
			if hours < 0 {
				hours = 0
			}
			g.Items = append(g.Items, DeviationSummaryItem{DeviationID: d.DeviationID, Category: d.Category, Severity: d.Severity, DetectedAt: d.DetectedAt, ClosedAt: d.ClosedAt, Open: open, DispositionHours: round(hours, 2), DispositionDurationHours: round(hours, 2)})
		}
	}
	for _, g := range groups {
		var hours float64
		for _, item := range g.Items {
			hours += item.DispositionHours
		}
		if g.Total > 0 {
			g.AverageDispositionHours = round(hours/float64(g.Total), 2)
		}
		sort.Slice(g.Items, func(i, j int) bool { return g.Items[i].DeviationID < g.Items[j].DeviationID })
		out.DeviationGroups = append(out.DeviationGroups, *g)
	}
	sort.Slice(out.DeviationGroups, func(i, j int) bool { return out.DeviationGroups[i].Key < out.DeviationGroups[j].Key })
	return out
}

func round(v float64, places int) float64 { p := math.Pow10(places); return math.Round(v*p) / p }
func checkMessage(err error, ok string) string {
	if err != nil {
		return err.Error()
	}
	return ok
}
func missingMessage(v []string) string {
	if len(v) == 0 {
		return "必需证据齐备"
	}
	return "缺少证据：" + strings.Join(v, ", ")
}
func digestOfPlan(c *SealCase) string {
	if c.Plan == nil {
		return ""
	}
	return c.Plan.Digest
}
func digestOfVerification(c *SealCase) string {
	if c.Verification == nil {
		return ""
	}
	return c.Verification.Digest
}
func openDeviationIDs(c *SealCase) []string {
	v := []string{}
	for _, d := range c.Deviations {
		if d.ClosedAt == nil {
			v = append(v, d.DeviationID)
		}
	}
	sort.Strings(v)
	return v
}
func sortedKeys(m map[string]bool) []string {
	v := []string{}
	for k := range m {
		v = append(v, k)
	}
	sort.Strings(v)
	return v
}

func ValidState(s State) bool {
	switch s {
	case StateDraft, StateBaselineLocked, StateSealing, StateHeld, StateVerification, StateReleased, StateArchived:
		return true
	}
	return false
}
func ValidateTimeRange(from, to *time.Time) error {
	if from != nil && to != nil && to.Before(*from) {
		return Invalid("创建时间范围无效", map[string]string{"created_to": "不得早于 created_from"})
	}
	return nil
}

func ValidateDetectedTimeRange(from, to *time.Time) error {
	if from != nil && to != nil && to.Before(*from) {
		return Invalid("偏差发现时间范围无效", map[string]string{"detected_to": "不得早于 detected_from"})
	}
	return nil
}
