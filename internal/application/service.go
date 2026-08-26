package application

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"wellseal/internal/archive"
	"wellseal/internal/domain"
	"wellseal/internal/store"
)

type Service struct {
	store             *store.Store
	archive           *archive.Builder
	now               func() time.Time
	verificationMu    sync.RWMutex
	verificationCache map[string]domain.VerificationReport
}

func New(s *store.Store, a *archive.Builder) *Service {
	return &Service{store: s, archive: a, now: time.Now, verificationCache: map[string]domain.VerificationReport{}}
}
func (s *Service) Create(ctx context.Context, cmd CreateCaseCommand) (CaseView, error) {
	if strings.TrimSpace(cmd.Actor) == "" {
		return CaseView{}, domain.Invalid("actor 不能为空", nil)
	}
	c := &domain.SealCase{CaseID: newID("case"), WellCode: strings.TrimSpace(cmd.WellCode), SiteName: strings.TrimSpace(cmd.SiteName), Latitude: cmd.Latitude, Longitude: cmd.Longitude, TotalDepthM: cmd.TotalDepthM, CasingDiameterMM: cmd.CasingDiameterMM, OwnerName: strings.TrimSpace(cmd.OwnerName), State: domain.StateDraft, Revision: 1, CreatedAt: s.now().UTC(), Checkpoints: []domain.ConstructionCheckpoint{}, Deviations: []domain.Deviation{}}
	if err := domain.ValidateBaseline(c); err != nil {
		return CaseView{}, err
	}
	saved, replay, err := s.store.Create(ctx, c, cmd.RequestID, fingerprint(cmd), cmd.Actor)
	return view(saved, replay), err
}
func (s *Service) LockBaseline(ctx context.Context, id string, cmd LockBaselineCommand) (CaseView, error) {
	return s.apply(ctx, id, cmd.Meta, cmd, "baseline_locked", func(c *domain.SealCase) error {
		if c.State != domain.StateDraft {
			return domain.Gate("仅草稿个案可冻结井况基线")
		}
		if err := domain.ValidateBaseline(c); err != nil {
			return err
		}
		return domain.Transition(c, domain.StateBaselineLocked)
	})
}

func (s *Service) ReviseBaseline(ctx context.Context, id string, cmd BaselinePatchCommand) (CaseView, error) {
	changes := []string{}
	if cmd.WellCode != nil {
		changes = append(changes, "well_code")
	}
	if cmd.SiteName != nil {
		changes = append(changes, "site_name")
	}
	if cmd.Latitude != nil {
		changes = append(changes, "latitude")
	}
	if cmd.Longitude != nil {
		changes = append(changes, "longitude")
	}
	if cmd.TotalDepthM != nil {
		changes = append(changes, "total_depth_m")
	}
	if cmd.CasingDiameterMM != nil {
		changes = append(changes, "casing_diameter_mm")
	}
	if cmd.OwnerName != nil {
		changes = append(changes, "owner_name")
	}
	if len(changes) == 0 {
		return CaseView{}, domain.Invalid("至少提供一个基线字段", map[string]string{"baseline": "不能为空"})
	}
	sort.Strings(changes)
	event := "baseline_revised:" + strings.Join(changes, ",")
	return s.apply(ctx, id, cmd.Meta, cmd, event, func(c *domain.SealCase) error {
		if c.State != domain.StateDraft {
			return domain.Gate("仅草稿个案可修订井况基线")
		}
		if cmd.WellCode != nil {
			c.WellCode = strings.TrimSpace(*cmd.WellCode)
		}
		if cmd.SiteName != nil {
			c.SiteName = strings.TrimSpace(*cmd.SiteName)
		}
		if cmd.OwnerName != nil {
			c.OwnerName = strings.TrimSpace(*cmd.OwnerName)
		}
		if cmd.Latitude != nil {
			c.Latitude = *cmd.Latitude
		}
		if cmd.Longitude != nil {
			c.Longitude = *cmd.Longitude
		}
		if cmd.TotalDepthM != nil {
			c.TotalDepthM = *cmd.TotalDepthM
		}
		if cmd.CasingDiameterMM != nil {
			c.CasingDiameterMM = *cmd.CasingDiameterMM
		}
		return domain.ValidateBaseline(c)
	})
}

func (s *Service) UpdateBaseline(ctx context.Context, id string, cmd BaselinePatchCommand) (CaseView, error) {
	return s.ReviseBaseline(ctx, id, cmd)
}
func (s *Service) SetPlan(ctx context.Context, id string, cmd PlanCommand) (CaseView, error) {
	return s.apply(ctx, id, cmd.Meta, cmd, "plan_locked", func(c *domain.SealCase) error {
		if c.State != domain.StateBaselineLocked {
			return domain.Gate("仅冻结基线后可编制方案")
		}
		p := &domain.SealPlan{PlanID: newID("plan"), CaseID: id, LayerSpecs: cmd.LayerSpecs, MaterialLots: cmd.MaterialLots, VolumeTolerancePercent: cmd.VolumeTolerancePercent, RequiredEvidenceTypes: cmd.RequiredEvidenceTypes, PreparedBy: strings.TrimSpace(cmd.PreparedBy), ReviewedBy: strings.TrimSpace(cmd.ReviewedBy), LockedAt: s.now().UTC()}
		if err := domain.ValidatePlan(c, p); err != nil {
			return err
		}
		p.Digest = domain.PlanDigest(*p)
		c.Plan = p
		return nil
	})
}
func (s *Service) PreflightPlan(ctx context.Context, id string, cmd PlanCommand) (domain.PlanPreflight, error) {
	c, err := s.store.Get(ctx, id)
	if err != nil {
		return domain.PlanPreflight{}, err
	}
	if c.State != domain.StateBaselineLocked {
		return domain.PlanPreflight{}, domain.Gate("仅冻结基线后可预检方案")
	}
	p := domain.SealPlan{CaseID: id, LayerSpecs: cmd.LayerSpecs, MaterialLots: cmd.MaterialLots, VolumeTolerancePercent: cmd.VolumeTolerancePercent, RequiredEvidenceTypes: cmd.RequiredEvidenceTypes, PreparedBy: strings.TrimSpace(cmd.PreparedBy), ReviewedBy: strings.TrimSpace(cmd.ReviewedBy)}
	return domain.PreflightPlan(c, p), nil
}
func (s *Service) Start(ctx context.Context, id string, cmd StartCommand) (CaseView, error) {
	return s.apply(ctx, id, cmd.Meta, cmd, "sealing_started", func(c *domain.SealCase) error {
		if c.State != domain.StateBaselineLocked || c.Plan == nil {
			return domain.Gate("必须先冻结有效方案")
		}
		return domain.Transition(c, domain.StateSealing)
	})
}
func (s *Service) Checkpoint(ctx context.Context, id string, cmd CheckpointCommand) (CaseView, error) {
	items := cmd.Items
	if len(items) == 0 {
		items = []CheckpointItem{{StageType: cmd.StageType, DepthFromM: cmd.DepthFromM, DepthToM: cmd.DepthToM, MaterialLot: cmd.MaterialLot, ActualVolumeL: cmd.ActualVolumeL, RecordedBy: cmd.RecordedBy, EvidenceDigest: cmd.EvidenceDigest, EvidenceTypes: cmd.EvidenceTypes, Measurements: cmd.Measurements, SequenceNo: cmd.SequenceNo}}
	}
	if len(items) > domain.MaxBatchCheckpoints {
		return CaseView{}, domain.Invalid("批量检查点超过上限", map[string]string{"items": fmt.Sprintf("最多 %d 项", domain.MaxBatchCheckpoints)})
	}
	eventType := "checkpoint_batch_recorded"
	result, err := s.apply(ctx, id, cmd.Meta, cmd, eventType, func(c *domain.SealCase) error {
		if c.State != domain.StateSealing {
			return domain.Gate("当前状态不可记录施工检查点")
		}
		existing := map[int]bool{}
		for _, prior := range c.Checkpoints {
			existing[prior.SequenceNo] = true
		}
		next := 1
		for existing[next] {
			next++
		}
		seen := map[int]bool{}
		batch := make([]domain.ConstructionCheckpoint, 0, len(items))
		now := s.now().UTC()
		for i, item := range items {
			fail := func(category, msg string) error {
				seq := item.SequenceNo
				c.Deviations = append(c.Deviations, domain.Deviation{DeviationID: newID("dev"), CaseID: id, Category: category, Severity: domain.DeviationSeverity(category), Description: fmt.Sprintf("items[%d] / 第 %d 层：%s", i, seq, msg), DetectedAt: now, LayerSequenceNo: &seq, Source: "checkpoint_batch"})
				return domain.Transition(c, domain.StateHeld)
			}
			if item.SequenceNo != next+i {
				return fail("depth_out_of_range", "必须从当前首个待施工层连续登记")
			}
			if seen[item.SequenceNo] || existing[item.SequenceNo] {
				return fail("sequence_duplicate", "序号重复或已登记")
			}
			seen[item.SequenceNo] = true
			cp := domain.ConstructionCheckpoint{CheckpointID: newID("cp"), CaseID: id, StageType: item.StageType, DepthFromM: item.DepthFromM, DepthToM: item.DepthToM, MaterialLot: item.MaterialLot, ActualVolumeL: item.ActualVolumeL, MeasuredAt: now, RecordedBy: strings.TrimSpace(item.RecordedBy), EvidenceDigest: strings.TrimSpace(item.EvidenceDigest), EvidenceTypes: item.EvidenceTypes, Measurements: item.Measurements, SequenceNo: item.SequenceNo}
			category, gateErr := domain.CheckCheckpoint(c, cp)
			if gateErr != nil {
				return fail(category, gateErr.Error())
			}
			batch = append(batch, cp)
		}
		c.Checkpoints = append(c.Checkpoints, batch...)
		return nil
	})
	return result, err
}

func (s *Service) CorrectCheckpoint(ctx context.Context, id string, cmd CheckpointCorrectionCommand) (CaseView, error) {
	reason := strings.TrimSpace(cmd.Reason)
	if reason == "" {
		reason = strings.TrimSpace(cmd.CorrectionReason)
	}
	if reason == "" {
		return CaseView{}, domain.Invalid("更正原因不能为空", map[string]string{"reason": "不能为空"})
	}
	prior, err := s.store.Get(ctx, id)
	if err != nil {
		return CaseView{}, err
	}
	var old domain.ConstructionCheckpoint
	found := false
	for _, cp := range prior.Checkpoints {
		if cp.SequenceNo == cmd.SequenceNo {
			old = cp
			found = true
			break
		}
	}
	if !found {
		return CaseView{}, domain.Gate("待更正检查点不存在")
	}
	newCP := domain.ConstructionCheckpoint{CheckpointID: newID("cp"), CaseID: id, StageType: strings.TrimSpace(cmd.StageType), DepthFromM: cmd.DepthFromM, DepthToM: cmd.DepthToM, MaterialLot: strings.TrimSpace(cmd.MaterialLot), ActualVolumeL: cmd.ActualVolumeL, MeasuredAt: s.now().UTC(), RecordedBy: strings.TrimSpace(cmd.RecordedBy), EvidenceDigest: strings.TrimSpace(cmd.EvidenceDigest), EvidenceTypes: cmd.EvidenceTypes, Measurements: cmd.Measurements, SequenceNo: cmd.SequenceNo}
	event := "checkpoint_corrected:" + fmt.Sprintf("sequence=%d;old=%s;new=%s;old_volume=%.3f;new_volume=%.3f;old_evidence=%s;new_evidence=%s;reason=%s", cmd.SequenceNo, fingerprint(old), fingerprint(newCP), old.ActualVolumeL, newCP.ActualVolumeL, old.EvidenceDigest, newCP.EvidenceDigest, reason)
	result, err := s.apply(ctx, id, cmd.Meta, cmd, event, func(c *domain.SealCase) error {
		if c.State == domain.StateHeld {
			return domain.Gate("个案处于暂停状态，必须先完成偏差处置")
		}
		if c.State != domain.StateSealing {
			return domain.Gate("仅施工状态可更正检查点")
		}
		if c.Plan == nil {
			return domain.Gate("施工方案尚未冻结")
		}
		if cmd.SequenceNo < 1 || cmd.SequenceNo > len(c.Plan.LayerSpecs) {
			return domain.Gate("定位序号不在已锁定方案内")
		}
		idx := -1
		for i := range c.Checkpoints {
			if c.Checkpoints[i].SequenceNo == cmd.SequenceNo {
				idx = i
				break
			}
		}
		if idx < 0 {
			return domain.Gate("待更正检查点不存在")
		}
		if _, checkErr := domain.CheckCheckpoint(c, newCP); checkErr != nil {
			return checkErr
		}
		c.Checkpoints[idx] = newCP
		return nil
	})
	return result, err
}

func (s *Service) ResolveDeviation(ctx context.Context, id string, cmd ResolveDeviationCommand) (CaseView, error) {
	return s.apply(ctx, id, cmd.Meta, cmd, "deviation_disposed", func(c *domain.SealCase) error {
		if c.State != domain.StateHeld {
			return domain.Gate("个案不在暂停状态")
		}
		if strings.TrimSpace(cmd.ClosedBy) != strings.TrimSpace(cmd.Actor) {
			return domain.Invalid("关闭人必须与当前处置操作人一致", map[string]string{"closed_by": "必须与 actor 一致"})
		}
		found := false
		now := s.now().UTC()
		for i := range c.Deviations {
			if c.Deviations[i].DeviationID == cmd.DeviationID && c.Deviations[i].ClosedAt == nil {
				if err := domain.ValidateDeviationResolution(c.Deviations[i], cmd.CorrectiveAction, cmd.RetestResult, cmd.ClosedBy, cmd.RetestValues); err != nil {
					return err
				}
				disp := domain.DeviationDisposition{CorrectiveAction: strings.TrimSpace(cmd.CorrectiveAction), RetestResult: cmd.RetestResult, ClosedBy: strings.TrimSpace(cmd.ClosedBy), RetestValues: cmd.RetestValues, OccurredAt: now}
				c.Deviations[i].Dispositions = append(c.Deviations[i].Dispositions, disp)
				if cmd.RetestResult == "failed" {
					found = true
					break
				}
				c.Deviations[i].CorrectiveAction = cmd.CorrectiveAction
				c.Deviations[i].RetestResult = cmd.RetestResult
				c.Deviations[i].ClosedBy = cmd.ClosedBy
				c.Deviations[i].ClosedAt = &now
				found = true
			}
		}
		if !found {
			return domain.Gate("未找到待关闭偏差")
		}
		if !domain.OpenDeviation(c) {
			return domain.Transition(c, domain.StateSealing)
		}
		return nil
	})
}
func (s *Service) Verify(ctx context.Context, id string, cmd VerifyCommand) (CaseView, error) {
	result, err := s.apply(ctx, id, cmd.Meta, cmd, "integrity_verified", func(c *domain.SealCase) error {
		if c.State != domain.StateSealing {
			return domain.Gate("仅施工状态可执行完整性验证")
		}
		report := domain.BuildVerificationReport(c)
		if cmd.PreflightDigest != "" && cmd.PreflightDigest != report.Digest {
			return domain.Conflict(cmd.ExpectedRevision, c.Revision)
		}
		v := domain.VerificationResult{Passed: report.Passed, Digest: report.Digest, VerifiedAt: s.now().UTC(), Checks: report.Checks, SourceRevision: report.SourceRevision, PlanDigest: report.PlanDigest}
		for _, check := range report.Checks {
			if !check.Passed {
				v.Findings = append(v.Findings, check.Message)
			}
		}
		c.Verification = &v
		if !v.Passed {
			return nil
		}
		return domain.Transition(c, domain.StateVerification)
	})
	if err == nil && result.Case != nil && result.Case.Verification != nil && !result.Case.Verification.Passed {
		return result, domain.Gate("完整性验证未通过，请根据分层报告整改")
	}
	return result, err
}
func (s *Service) Witness(ctx context.Context, id string, cmd WitnessCommand) (CaseView, error) {
	if cmd.Decision != "release" && cmd.Decision != "return" {
		return CaseView{}, domain.Invalid("decision 必须为 release 或 return", nil)
	}
	checkCase, checkErr := s.store.Get(ctx, id)
	if checkErr != nil {
		return CaseView{}, checkErr
	}
	checkEvents, checkErr := s.store.Events(ctx, id)
	if checkErr != nil {
		return CaseView{}, checkErr
	}
	checkHead := ""
	if len(checkEvents) > 0 {
		checkHead = checkEvents[len(checkEvents)-1].Digest
	}
	precomputedChecklist := domain.BuildWitnessChecklist(checkCase, checkHead)
	v, err := s.apply(ctx, id, cmd.Meta, cmd, "witness_decided", func(c *domain.SealCase) error {
		if c.State != domain.StateVerification || c.Verification == nil || !c.Verification.Passed {
			return domain.Gate("仅验证通过的个案可供见证决定")
		}
		if strings.TrimSpace(cmd.WitnessID) == "" {
			return domain.Invalid("witness_id 不能为空", nil)
		}
		if strings.TrimSpace(cmd.DecisionNote) == "" {
			return domain.Invalid("决定说明不能为空", map[string]string{"decision_note": "不能为空"})
		}
		participants := domain.RecorderSet(c)
		if c.Plan != nil {
			participants[c.Plan.PreparedBy] = true
			participants[c.Plan.ReviewedBy] = true
		}
		for _, d := range c.Deviations {
			if d.ClosedBy != "" {
				participants[d.ClosedBy] = true
			}
			for _, x := range d.Dispositions {
				participants[x.ClosedBy] = true
			}
		}
		if participants[cmd.WitnessID] {
			return domain.Gate("独立见证人不得参与方案、施工或偏差处置")
		}
		if cmd.VerificationDigest != "" && cmd.VerificationDigest != c.Verification.Digest {
			return domain.Gate("验证摘要已变化，请刷新后重试")
		}
		checklist := precomputedChecklist
		required := map[string]bool{}
		for _, x := range checklist {
			if x.Required {
				required[x.Code] = true
			}
		}
		confirmed := map[string]bool{}
		for _, x := range cmd.ConfirmedChecklist {
			confirmed[x] = true
		}
		if cmd.Decision == "release" {
			for code := range required {
				if !confirmed[code] {
					return domain.Invalid("见证清单未全部确认", map[string]string{"confirmed_checklist": "缺少必选项：" + code})
				}
			}
		} else if len(cmd.ReturnedItems) == 0 {
			return domain.Invalid("退回必须选择未通过清单项", map[string]string{"returned_items": "至少选择一项"})
		}
		checklistDigest := domain.MustDigest(struct {
			Items     []domain.WitnessChecklistItem `json:"items"`
			Confirmed []string                      `json:"confirmed"`
		}{checklist, sortedStrings(cmd.ConfirmedChecklist)})
		c.Release = &domain.ReleaseDecision{Decision: cmd.Decision, WitnessID: cmd.WitnessID, DecisionNote: cmd.DecisionNote, DecidedAt: s.now().UTC(), ConfirmedChecklist: sortedStrings(cmd.ConfirmedChecklist), ChecklistDigest: checklistDigest, ReturnedItems: sortedStrings(cmd.ReturnedItems), LayerSequenceNo: cmd.LayerSequenceNo}
		if cmd.Decision == "return" {
			c.Deviations = append(c.Deviations, domain.Deviation{DeviationID: newID("dev"), CaseID: id, Category: "witness_return", Severity: domain.DeviationSeverity("witness_return"), Description: cmd.DecisionNote, DetectedAt: s.now().UTC(), LayerSequenceNo: cmd.LayerSequenceNo, Source: strings.Join(cmd.ReturnedItems, ",")})
			return domain.Transition(c, domain.StateHeld)
		} else {
			return domain.Transition(c, domain.StateReleased)
		}
	})
	if err != nil || cmd.Decision == "return" || v.Replayed && v.Case.State == domain.StateArchived {
		return v, err
	}
	events, err := s.store.Events(ctx, id)
	if err != nil {
		return CaseView{}, err
	}
	a, err := s.archive.Build(v.Case, events)
	if err != nil {
		return CaseView{}, err
	}
	archived, replay, err := s.store.FreezeArchive(ctx, id, v.Case.Revision, cmd.RequestID+":archive", fingerprint(struct {
		Command  WitnessCommand `json:"command"`
		Manifest string         `json:"manifest"`
	}{cmd, a.ManifestDigest}), cmd.WitnessID, a)
	return view(archived, replay), err
}
func (s *Service) Get(ctx context.Context, id string) (CaseView, error) {
	c, err := s.store.Get(ctx, id)
	if err != nil {
		return CaseView{}, err
	}
	v := view(c, false)
	report := domain.BuildVerificationReport(c)
	v.VerificationPreflight = &report
	v.DeviationTasks = deviationTasks(c, DeviationFilter{})
	summary := domain.SummarizeDeviations(c, nil, nil, nil, s.now().UTC())
	v.DeviationSummary = &summary
	if c.State == domain.StateVerification {
		events, _ := s.store.Events(ctx, id)
		head := ""
		if len(events) > 0 {
			head = events[len(events)-1].Digest
		}
		v.WitnessChecklist = domain.BuildWitnessChecklist(c, head)
	}
	return v, nil
}
func (s *Service) List(ctx context.Context, f CaseListFilter) (CaseListResult, error) {
	if len(f.Keyword) > 100 {
		return CaseListResult{}, domain.Invalid("检索关键词过长", map[string]string{"keyword": "最多 100 个字符"})
	}
	f.Keyword = strings.TrimSpace(f.Keyword)
	if f.State != "" && !domain.ValidState(f.State) {
		return CaseListResult{}, domain.Invalid("状态筛选无效", map[string]string{"state": "未知状态"})
	}
	if err := domain.ValidateTimeRange(f.CreatedFrom, f.CreatedTo); err != nil {
		return CaseListResult{}, err
	}
	sf := store.CaseFilter{Keyword: f.Keyword, State: f.State, CreatedFrom: f.CreatedFrom, CreatedTo: f.CreatedTo, Limit: f.Limit}
	if f.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(f.Cursor)
		if err != nil {
			return CaseListResult{}, domain.Invalid("游标无效或已过期", map[string]string{"cursor": "无法解析"})
		}
		var cur struct {
			CreatedAt time.Time `json:"created_at"`
			CaseID    string    `json:"case_id"`
			Filter    string    `json:"filter"`
		}
		if json.Unmarshal(raw, &cur) != nil || cur.Filter != listFilterDigest(f) {
			return CaseListResult{}, domain.Invalid("游标无效或与筛选条件不匹配", map[string]string{"cursor": "请重新查询"})
		}
		sf.AfterCreatedAt = &cur.CreatedAt
		sf.AfterCaseID = cur.CaseID
	}
	got, err := s.store.ListFiltered(ctx, sf)
	if err != nil {
		return CaseListResult{}, err
	}
	out := CaseListResult{Items: got.Items, Total: got.Total, StatusCounts: got.StatusCounts}
	if got.HasMore && len(got.Items) > 0 {
		last := got.Items[len(got.Items)-1]
		raw, _ := json.Marshal(struct {
			CreatedAt time.Time `json:"created_at"`
			CaseID    string    `json:"case_id"`
			Filter    string    `json:"filter"`
		}{last.CreatedAt, last.CaseID, listFilterDigest(f)})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	return out, nil
}

func (s *Service) QualitySummary(ctx context.Context, f QualitySummaryFilter) (domain.QualitySummary, error) {
	if f.State != "" && !domain.ValidState(f.State) {
		return domain.QualitySummary{}, domain.Invalid("状态筛选无效", map[string]string{"state": "未知状态"})
	}
	if len([]rune(strings.TrimSpace(f.SiteName))) > 100 {
		return domain.QualitySummary{}, domain.Invalid("场地筛选过长", map[string]string{"site_name": "最多 100 个字符"})
	}
	if len([]rune(strings.TrimSpace(f.OwnerName))) > 100 {
		return domain.QualitySummary{}, domain.Invalid("责任人筛选过长", map[string]string{"owner_name": "最多 100 个字符"})
	}
	if err := domain.ValidateTimeRange(f.CreatedFrom, f.CreatedTo); err != nil {
		return domain.QualitySummary{}, err
	}
	cases, upper, err := s.store.QualitySnapshot(ctx, store.QualityFilter{State: f.State, CreatedFrom: f.CreatedFrom, CreatedTo: f.CreatedTo, SiteName: f.SiteName, OwnerName: f.OwnerName})
	if err != nil {
		return domain.QualitySummary{}, err
	}
	return domain.SummarizeQuality(cases, s.now().UTC(), upper), nil
}
func (s *Service) Timeline(ctx context.Context, id string, after int64) (domain.Page[domain.AuditEvent], error) {
	return s.store.Timeline(ctx, id, after, 50)
}
func (s *Service) Archive(ctx context.Context, id string) (ArchiveVerification, error) {
	a, err := s.store.Archive(ctx, id)
	if err != nil {
		return ArchiveVerification{}, err
	}
	c, err := s.store.Get(ctx, id)
	if err != nil {
		return ArchiveVerification{}, err
	}
	events, err := s.store.Events(ctx, id)
	if err != nil {
		return ArchiveVerification{}, err
	}
	checks := []ArchiveCheck{}
	add := func(code string, e error) {
		checks = append(checks, ArchiveCheck{code, e == nil, map[bool]string{true: "通过", false: eString(e)}[e == nil]})
	}
	add("payload", archive.VerifyPayload(a))
	add("event_chain", archive.ValidateChain(events))
	var payload archive.Payload
	parseErr := json.Unmarshal(a.Payload, &payload)
	add("json", parseErr)
	var compareErr error
	if parseErr == nil {
		if domain.MustDigest(payload.Case.Plan) != domain.MustDigest(c.Plan) || domain.MustDigest(domain.NormalizedCheckpoints(payload.Case.Checkpoints)) != domain.MustDigest(domain.NormalizedCheckpoints(c.Checkpoints)) || domain.MustDigest(payload.Case.Release) != domain.MustDigest(c.Release) {
			compareErr = fmt.Errorf("冻结业务事实与保存载荷不一致")
		}
		if len(events) < len(payload.Events) || domain.MustDigest(events[:len(payload.Events)]) != domain.MustDigest(payload.Events) {
			compareErr = fmt.Errorf("冻结事件前缀与保存载荷不一致")
		}
	}
	add("rebuild", compareErr)
	for _, x := range checks {
		if !x.Passed {
			return ArchiveVerification{}, domain.Integrity(fmt.Sprintf("归档一致性核验失败（%s）：%s", x.Code, x.Message))
		}
	}
	return ArchiveVerification{Archive: a, Checks: checks, EventCount: len(events), EvidenceCount: len(c.Checkpoints), VerifiedAt: s.now().UTC()}, nil
}
func (s *Service) apply(ctx context.Context, id string, m Meta, body any, event string, fn store.Mutator) (CaseView, error) {
	if strings.TrimSpace(m.Actor) == "" {
		return CaseView{}, domain.Invalid("actor 不能为空", nil)
	}
	c, replay, err := s.store.Apply(ctx, id, m.ExpectedRevision, m.RequestID, fingerprint(body), m.Actor, event, fn)
	return view(c, replay), err
}
func view(c *domain.SealCase, replay bool) CaseView {
	if c == nil {
		return CaseView{}
	}
	return CaseView{Case: c, Progress: domain.SummarizeProgress(c), Replayed: replay}
}
func fingerprint(v any) string { return domain.MustDigest(v) }
func newID(prefix string) string {
	var b [10]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b[:]))
}
func (s *Service) VerificationPreflight(ctx context.Context, id string) (domain.VerificationReport, error) {
	c, err := s.store.Get(ctx, id)
	if err != nil {
		return domain.VerificationReport{}, err
	}
	if c.State != domain.StateSealing {
		return domain.VerificationReport{}, domain.Gate("仅施工状态可执行验证预检")
	}
	s.verificationMu.RLock()
	report, ok := s.verificationCache[id]
	s.verificationMu.RUnlock()
	if ok {
		return report, nil
	}
	report = domain.BuildVerificationReport(c)
	s.verificationMu.Lock()
	s.verificationCache[id] = report
	s.verificationMu.Unlock()
	return report, nil
}
func (s *Service) Deviations(ctx context.Context, id string, f DeviationFilter) (DeviationTaskSummary, error) {
	c, err := s.store.Get(ctx, id)
	if err != nil {
		return DeviationTaskSummary{}, err
	}
	return *deviationTasks(c, f), nil
}

func (s *Service) DeviationSummary(ctx context.Context, id string, f DeviationFilter) (domain.DeviationSummary, error) {
	if err := domain.ValidateDetectedTimeRange(f.DetectedFrom, f.DetectedTo); err != nil {
		return domain.DeviationSummary{}, err
	}
	c, err := s.store.Get(ctx, id)
	if err != nil {
		return domain.DeviationSummary{}, err
	}
	if f.Category != "" || f.Severity != "" {
		filtered := make([]domain.Deviation, 0, len(c.Deviations))
		for _, d := range c.Deviations {
			if (f.Category == "" || d.Category == f.Category) && (f.Severity == "" || d.Severity == f.Severity) {
				filtered = append(filtered, d)
			}
		}
		c.Deviations = filtered
	}
	return domain.SummarizeDeviations(c, f.Open, f.DetectedFrom, f.DetectedTo, s.now().UTC()), nil
}
func deviationTasks(c *domain.SealCase, f DeviationFilter) *DeviationTaskSummary {
	out := &DeviationTaskSummary{Items: []domain.Deviation{}}
	for _, d := range c.Deviations {
		open := d.ClosedAt == nil
		if open {
			out.OpenCount++
			if out.EarliestDetectedAt == nil || d.DetectedAt.Before(*out.EarliestDetectedAt) {
				t := d.DetectedAt
				out.EarliestDetectedAt = &t
			}
		}
		if f.Open != nil && open != *f.Open {
			continue
		}
		if f.Category != "" && d.Category != f.Category {
			continue
		}
		if f.Severity != "" && d.Severity != f.Severity {
			continue
		}
		if f.DetectedFrom != nil && d.DetectedAt.Before(*f.DetectedFrom) {
			continue
		}
		if f.DetectedTo != nil && d.DetectedAt.After(*f.DetectedTo) {
			continue
		}
		out.Items = append(out.Items, d)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		if !out.Items[i].DetectedAt.Equal(out.Items[j].DetectedAt) {
			return out.Items[i].DetectedAt.Before(out.Items[j].DetectedAt)
		}
		return out.Items[i].DeviationID < out.Items[j].DeviationID
	})
	return out
}
func sortedStrings(v []string) []string {
	out := append([]string(nil), v...)
	sort.Strings(out)
	return out
}
func listFilterDigest(f CaseListFilter) string {
	return domain.MustDigest(struct {
		Keyword string       `json:"keyword"`
		State   domain.State `json:"state"`
		From    *time.Time   `json:"from"`
		To      *time.Time   `json:"to"`
	}{strings.TrimSpace(f.Keyword), f.State, f.CreatedFrom, f.CreatedTo})
}
func eString(err error) string {
	if err == nil {
		return "通过"
	}
	return err.Error()
}
