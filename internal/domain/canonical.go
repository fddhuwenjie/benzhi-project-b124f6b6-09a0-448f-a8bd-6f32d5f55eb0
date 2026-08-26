package domain

import (
	"sort"
	"strings"
)

func NormalizedPlan(plan SealPlan) SealPlan {
	out := plan
	out.LayerSpecs = append([]LayerSpec(nil), plan.LayerSpecs...)
	for i := range out.LayerSpecs {
		out.LayerSpecs[i].MaterialLot = strings.TrimSpace(out.LayerSpecs[i].MaterialLot)
		out.LayerSpecs[i].StageType = strings.TrimSpace(out.LayerSpecs[i].StageType)
		out.LayerSpecs[i].RequiredEvidenceTypes = sortedTrimmedUnique(out.LayerSpecs[i].RequiredEvidenceTypes)
	}
	sort.Slice(out.LayerSpecs, func(i, j int) bool {
		if out.LayerSpecs[i].SequenceNo != out.LayerSpecs[j].SequenceNo {
			return out.LayerSpecs[i].SequenceNo < out.LayerSpecs[j].SequenceNo
		}
		if out.LayerSpecs[i].DepthFromM != out.LayerSpecs[j].DepthFromM {
			return out.LayerSpecs[i].DepthFromM < out.LayerSpecs[j].DepthFromM
		}
		return out.LayerSpecs[i].DepthToM < out.LayerSpecs[j].DepthToM
	})
	out.MaterialLots = sortedTrimmedUnique(plan.MaterialLots)
	out.RequiredEvidenceTypes = sortedTrimmedUnique(plan.RequiredEvidenceTypes)
	out.PreparedBy = strings.TrimSpace(plan.PreparedBy)
	out.ReviewedBy = strings.TrimSpace(plan.ReviewedBy)
	return out
}

func PlanDigest(plan SealPlan) string {
	n := NormalizedPlan(plan)
	return MustDigest(struct {
		CaseID     string      `json:"case_id"`
		Layers     []LayerSpec `json:"layers"`
		Lots       []string    `json:"lots"`
		Tolerance  float64     `json:"volume_tolerance_percent"`
		Evidence   []string    `json:"required_evidence_types"`
		PreparedBy string      `json:"prepared_by"`
		ReviewedBy string      `json:"reviewed_by"`
	}{n.CaseID, n.LayerSpecs, n.MaterialLots, n.VolumeTolerancePercent, n.RequiredEvidenceTypes, n.PreparedBy, n.ReviewedBy})
}

func NormalizedCheckpoints(in []ConstructionCheckpoint) []ConstructionCheckpoint {
	out := append([]ConstructionCheckpoint(nil), in...)
	for i := range out {
		out[i].EvidenceTypes = sortedUnique(out[i].EvidenceTypes)
		out[i].Measurements = append([]Measurement(nil), out[i].Measurements...)
		sort.Slice(out[i].Measurements, func(a, b int) bool {
			if out[i].Measurements[a].Name != out[i].Measurements[b].Name {
				return out[i].Measurements[a].Name < out[i].Measurements[b].Name
			}
			if out[i].Measurements[a].Unit != out[i].Measurements[b].Unit {
				return out[i].Measurements[a].Unit < out[i].Measurements[b].Unit
			}
			return out[i].Measurements[a].Value < out[i].Measurements[b].Value
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SequenceNo != out[j].SequenceNo {
			return out[i].SequenceNo < out[j].SequenceNo
		}
		return out[i].CheckpointID < out[j].CheckpointID
	})
	return out
}

func NormalizedDeviations(in []Deviation) []Deviation {
	out := append([]Deviation(nil), in...)
	for i := range out {
		out[i].RetestValues = normalizedRetestValues(out[i].RetestValues)
		sort.Slice(out[i].Dispositions, func(a, b int) bool {
			if !out[i].Dispositions[a].OccurredAt.Equal(out[i].Dispositions[b].OccurredAt) {
				return out[i].Dispositions[a].OccurredAt.Before(out[i].Dispositions[b].OccurredAt)
			}
			return out[i].Dispositions[a].ClosedBy < out[i].Dispositions[b].ClosedBy
		})
		for j := range out[i].Dispositions {
			out[i].Dispositions[j].RetestValues = normalizedRetestValues(out[i].Dispositions[j].RetestValues)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].DetectedAt.Equal(out[j].DetectedAt) {
			return out[i].DetectedAt.Before(out[j].DetectedAt)
		}
		return out[i].DeviationID < out[j].DeviationID
	})
	return out
}

func normalizedRetestValues(values map[string]float64) map[string]float64 {
	for key, value := range values {
		normalized := strings.TrimSpace(key)
		if normalized == key {
			continue
		}
		delete(values, key)
		values[normalized] = value
	}
	return values
}
func NormalizedEvents(in []AuditEvent) []AuditEvent {
	out := append([]AuditEvent(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out
}
func sortedUnique(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
func sortedTrimmedUnique(in []string) []string {
	trimmed := make([]string, 0, len(in))
	for _, value := range in {
		trimmed = append(trimmed, strings.TrimSpace(value))
	}
	return sortedUnique(trimmed)
}
