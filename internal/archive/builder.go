package archive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"wellseal/internal/domain"
)

type Builder struct{ now func() time.Time }

func NewBuilder() *Builder                                 { return &Builder{now: time.Now} }
func (b *Builder) WithClock(now func() time.Time) *Builder { b.now = now; return b }

type EvidenceItem struct {
	CheckpointID   string   `json:"checkpoint_id"`
	SequenceNo     int      `json:"sequence_no"`
	EvidenceDigest string   `json:"evidence_digest"`
	EvidenceTypes  []string `json:"evidence_types"`
}
type Manifest struct {
	FormatVersion      string         `json:"format_version"`
	CaseID             string         `json:"case_id"`
	State              domain.State   `json:"state"`
	WellCode           string         `json:"well_code"`
	SiteName           string         `json:"site_name"`
	PlanDigest         string         `json:"plan_digest"`
	VerificationDigest string         `json:"verification_digest"`
	Decision           string         `json:"decision"`
	WitnessID          string         `json:"witness_id"`
	Evidence           []EvidenceItem `json:"evidence"`
	EventCount         int            `json:"event_count"`
	EventChainHead     string         `json:"event_chain_head"`
	ChecklistDigest    string         `json:"checklist_digest"`
	GeneratedAt        time.Time      `json:"generated_at"`
}
type Payload struct {
	Manifest Manifest            `json:"manifest"`
	Case     domain.SealCase     `json:"case"`
	Events   []domain.AuditEvent `json:"events"`
}

func ValidateChain(events []domain.AuditEvent) error {
	if len(events) == 0 {
		return domain.Gate("审计事件链为空")
	}
	previous := ""
	for i, e := range events {
		if e.PreviousDigest != previous {
			return fmt.Errorf("审计事件 %d 的前序摘要不连续", i+1)
		}
		if domain.EventDigest(e) != e.Digest {
			return fmt.Errorf("审计事件 %d 摘要不匹配", i+1)
		}
		previous = e.Digest
	}
	return nil
}
func (b *Builder) Build(c *domain.SealCase, events []domain.AuditEvent) (domain.ReleaseArchive, error) {
	if c.State != domain.StateReleased {
		return domain.ReleaseArchive{}, domain.Gate("仅已放行个案可生成归档")
	}
	if c.Verification == nil || !c.Verification.Passed {
		return domain.ReleaseArchive{}, domain.Gate("完整性验证未通过")
	}
	if c.Release == nil || c.Release.Decision != "release" {
		return domain.ReleaseArchive{}, domain.Gate("缺少独立放行决定")
	}
	if err := ValidateChain(events); err != nil {
		return domain.ReleaseArchive{}, err
	}
	canonicalCase := *c
	canonicalCase.Checkpoints = domain.NormalizedCheckpoints(c.Checkpoints)
	canonicalCase.Deviations = domain.NormalizedDeviations(c.Deviations)
	events = domain.NormalizedEvents(events)
	evidence := make([]EvidenceItem, 0, len(canonicalCase.Checkpoints))
	for _, cp := range canonicalCase.Checkpoints {
		types := append([]string(nil), cp.EvidenceTypes...)
		sort.Strings(types)
		evidence = append(evidence, EvidenceItem{cp.CheckpointID, cp.SequenceNo, cp.EvidenceDigest, types})
	}
	sort.Slice(evidence, func(i, j int) bool { return evidence[i].SequenceNo < evidence[j].SequenceNo })
	generated := b.now().UTC()
	head := events[len(events)-1].Digest
	m := Manifest{FormatVersion: "wellseal-archive-v1", CaseID: c.CaseID, State: c.State, WellCode: c.WellCode, SiteName: c.SiteName, PlanDigest: c.Plan.Digest, VerificationDigest: c.Verification.Digest, Decision: c.Release.Decision, WitnessID: c.Release.WitnessID, Evidence: evidence, EventCount: len(events), EventChainHead: head, ChecklistDigest: c.Release.ChecklistDigest, GeneratedAt: generated}
	manifestDigest := domain.MustDigest(m)
	p := Payload{Manifest: m, Case: canonicalCase, Events: events}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return domain.ReleaseArchive{}, err
	}
	raw = append(raw, '\n')
	return domain.ReleaseArchive{ArchiveID: "arc-" + c.CaseID, CaseID: c.CaseID, Decision: c.Release.Decision, WitnessID: c.Release.WitnessID, DecisionNote: c.Release.DecisionNote, VerificationDigest: c.Verification.Digest, ManifestDigest: manifestDigest, EventChainHead: head, ChecklistDigest: c.Release.ChecklistDigest, GeneratedAt: generated, Payload: raw}, nil
}

func RebuildAndVerify(c *domain.SealCase, events []domain.AuditEvent, saved domain.ReleaseArchive) error {
	builder := NewBuilder().WithClock(func() time.Time { return saved.GeneratedAt })
	rebuilt, err := builder.Build(c, events)
	if err != nil {
		return err
	}
	if rebuilt.ManifestDigest != saved.ManifestDigest {
		return fmt.Errorf("重建清单摘要与冻结归档不一致")
	}
	if !bytes.Equal(rebuilt.Payload, saved.Payload) {
		return fmt.Errorf("重建归档载荷与冻结归档不一致")
	}
	return nil
}

func VerifyPayload(saved domain.ReleaseArchive) error {
	var payload Payload
	if err := json.Unmarshal(saved.Payload, &payload); err != nil {
		return fmt.Errorf("归档 JSON 无法解析: %w", err)
	}
	if payload.Manifest.CaseID != saved.CaseID || payload.Case.CaseID != saved.CaseID {
		return fmt.Errorf("归档个案标识不一致")
	}
	if payload.Manifest.VerificationDigest != saved.VerificationDigest {
		return fmt.Errorf("归档验证摘要不一致")
	}
	if payload.Case.Verification == nil || payload.Case.Verification.Digest != saved.VerificationDigest {
		return fmt.Errorf("归档内验证结论不一致")
	}
	if payload.Case.Plan == nil {
		return fmt.Errorf("归档内方案摘要不一致")
	}
	if payload.Case.Plan.PlanID != "" && domain.PlanDigest(*payload.Case.Plan) != payload.Manifest.PlanDigest {
		return fmt.Errorf("归档内方案摘要无法重算")
	}
	if payload.Case.Verification.SourceRevision > 0 {
		verificationCase := payload.Case
		verificationCase.Revision = payload.Case.Verification.SourceRevision
		recomputed := domain.BuildVerificationReport(&verificationCase)
		if recomputed.Digest != payload.Case.Verification.Digest || recomputed.PlanDigest != payload.Case.Verification.PlanDigest {
			return fmt.Errorf("归档内逐层验证报告无法重算")
		}
	}
	if payload.Case.Release == nil || payload.Case.Release.Decision != "release" || payload.Case.Release.WitnessID != saved.WitnessID {
		return fmt.Errorf("归档内见证决定不一致")
	}
	if payload.Manifest.ChecklistDigest != saved.ChecklistDigest || payload.Case.Release.ChecklistDigest != saved.ChecklistDigest {
		return fmt.Errorf("归档见证清单摘要不一致")
	}
	expectedEvidence := make([]EvidenceItem, 0, len(payload.Case.Checkpoints))
	for _, cp := range domain.NormalizedCheckpoints(payload.Case.Checkpoints) {
		types := append([]string(nil), cp.EvidenceTypes...)
		sort.Strings(types)
		expectedEvidence = append(expectedEvidence, EvidenceItem{CheckpointID: cp.CheckpointID, SequenceNo: cp.SequenceNo, EvidenceDigest: cp.EvidenceDigest, EvidenceTypes: types})
	}
	if domain.MustDigest(expectedEvidence) != domain.MustDigest(payload.Manifest.Evidence) {
		return fmt.Errorf("归档证据清单与施工事实不一致")
	}
	if err := ValidateChain(payload.Events); err != nil {
		return err
	}
	if payload.Events[len(payload.Events)-1].Digest != saved.EventChainHead || payload.Manifest.EventChainHead != saved.EventChainHead {
		return fmt.Errorf("归档事件链头不一致")
	}
	if domain.MustDigest(payload.Manifest) != saved.ManifestDigest {
		return fmt.Errorf("归档清单摘要不一致")
	}
	return nil
}
