package archive_deviation_alias_test

import (
	"encoding/json"
	"testing"
	"time"

	"wellseal/internal/archive"
	"wellseal/internal/domain"
)

func TestArchiveBuildDoesNotMutateDeviationHistory(t *testing.T) {
	earlier := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	later := earlier.Add(time.Hour)
	c := &domain.SealCase{
		CaseID: "case-alias", State: domain.StateReleased, Revision: 8,
		Plan:         &domain.SealPlan{Digest: "plan-digest"},
		Verification: &domain.VerificationResult{Passed: true, Digest: "verification-digest"},
		Release:      &domain.ReleaseDecision{Decision: "release", WitnessID: "witness"},
		Deviations: []domain.Deviation{{
			DeviationID:  "dev-1",
			RetestValues: map[string]float64{" final-reading ": 2},
			Dispositions: []domain.DeviationDisposition{
				{ClosedBy: "later", OccurredAt: later, RetestValues: map[string]float64{" later-reading ": 2}},
				{ClosedBy: "earlier", OccurredAt: earlier, RetestValues: map[string]float64{" earlier-reading ": 1}},
			},
		}},
	}
	event := domain.AuditEvent{Sequence: 1, CaseID: c.CaseID, Type: "witness_decided", Actor: "witness", OccurredAt: later, Revision: c.Revision, DataDigest: "case-digest"}
	event.Digest = domain.EventDigest(event)

	built, err := archive.NewBuilder().WithClock(func() time.Time { return later }).Build(c, []domain.AuditEvent{event})
	if err != nil {
		t.Fatal(err)
	}
	var payload archive.Payload
	if err = json.Unmarshal(built.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Case.Deviations[0].Dispositions[0].ClosedBy != "earlier" {
		t.Fatal("归档载荷未按发生时间规范化偏差处置")
	}
	if _, ok := payload.Case.Deviations[0].Dispositions[0].RetestValues["earlier-reading"]; !ok {
		t.Fatal("归档载荷未规范化复验指标键")
	}

	got := c.Deviations[0]
	if got.Dispositions[0].ClosedBy != "later" {
		t.Fatalf("归档构建改写了源处置顺序: first=%s", got.Dispositions[0].ClosedBy)
	}
	if _, ok := got.RetestValues[" final-reading "]; !ok {
		t.Fatal("归档构建改写了源偏差复验指标")
	}
	if _, ok := got.Dispositions[0].RetestValues[" later-reading "]; !ok {
		t.Fatal("归档构建改写了源处置复验指标")
	}
}
