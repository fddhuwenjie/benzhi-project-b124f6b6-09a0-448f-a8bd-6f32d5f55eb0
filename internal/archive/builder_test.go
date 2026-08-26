package archive

import (
	"testing"
	"time"

	"wellseal/internal/domain"
)

func TestBuildIsDeterministicAndVerifiable(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	c := &domain.SealCase{
		CaseID: "case-1", WellCode: "W", SiteName: "S", State: domain.StateReleased, Revision: 5,
		Plan:         &domain.SealPlan{Digest: "plan-digest"},
		Verification: &domain.VerificationResult{Passed: true, Digest: "verification-digest"},
		Release:      &domain.ReleaseDecision{Decision: "release", WitnessID: "见证人", DecisionNote: "同意"},
		Checkpoints: []domain.ConstructionCheckpoint{
			{CheckpointID: "cp-2", SequenceNo: 2, EvidenceDigest: "b", EvidenceTypes: []string{"photo"}},
			{CheckpointID: "cp-1", SequenceNo: 1, EvidenceDigest: "a", EvidenceTypes: []string{"log", "photo"}},
		},
	}
	e := domain.AuditEvent{Sequence: 1, CaseID: c.CaseID, Type: "released", Actor: "见证人", OccurredAt: now, Revision: 5, DataDigest: domain.MustDigest(c)}
	e.Digest = domain.EventDigest(e)
	b := NewBuilder().WithClock(func() time.Time { return now })
	one, err := b.Build(c, []domain.AuditEvent{e})
	if err != nil {
		t.Fatal(err)
	}
	two, err := b.Build(c, []domain.AuditEvent{e})
	if err != nil {
		t.Fatal(err)
	}
	if string(one.Payload) != string(two.Payload) || one.ManifestDigest != two.ManifestDigest {
		t.Fatal("相同快照生成的归档不确定")
	}
	if err = VerifyPayload(one); err != nil {
		t.Fatalf("归档自校验失败: %v", err)
	}
	one.Payload[20] ^= 1
	if err = VerifyPayload(one); err == nil {
		t.Fatal("篡改归档未被识别")
	}
}
