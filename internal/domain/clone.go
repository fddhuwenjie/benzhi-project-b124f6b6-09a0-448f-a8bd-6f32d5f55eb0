package domain

// Clone 生成 SealCase 的深拷贝，使调用方对返回值的修改不会影响缓存或数据库快照。
func (c *SealCase) Clone() *SealCase {
	if c == nil {
		return nil
	}
	out := *c
	if c.ArchivedAt != nil {
		t := *c.ArchivedAt
		out.ArchivedAt = &t
	}
	out.Plan = c.Plan.clone()
	out.Checkpoints = cloneCheckpoints(c.Checkpoints)
	out.Deviations = cloneDeviations(c.Deviations)
	out.Verification = c.Verification.clone()
	out.Release = c.Release.clone()
	return &out
}

func (p *SealPlan) clone() *SealPlan {
	if p == nil {
		return nil
	}
	out := *p
	out.LayerSpecs = cloneLayerSpecs(p.LayerSpecs)
	out.MaterialLots = cloneStrings(p.MaterialLots)
	out.RequiredEvidenceTypes = cloneStrings(p.RequiredEvidenceTypes)
	return &out
}

func cloneLayerSpecs(in []LayerSpec) []LayerSpec {
	if in == nil {
		return nil
	}
	out := make([]LayerSpec, len(in))
	for i, l := range in {
		out[i] = l
		out[i].RequiredEvidenceTypes = cloneStrings(l.RequiredEvidenceTypes)
	}
	return out
}

func cloneCheckpoints(in []ConstructionCheckpoint) []ConstructionCheckpoint {
	if in == nil {
		return nil
	}
	out := make([]ConstructionCheckpoint, len(in))
	for i, cp := range in {
		out[i] = cp
		out[i].EvidenceTypes = cloneStrings(cp.EvidenceTypes)
		out[i].Measurements = cloneMeasurements(cp.Measurements)
	}
	return out
}

func cloneMeasurements(in []Measurement) []Measurement {
	if in == nil {
		return nil
	}
	out := make([]Measurement, len(in))
	copy(out, in)
	return out
}

func cloneDeviations(in []Deviation) []Deviation {
	if in == nil {
		return nil
	}
	out := make([]Deviation, len(in))
	for i, d := range in {
		out[i] = d
		if d.ClosedAt != nil {
			t := *d.ClosedAt
			out[i].ClosedAt = &t
		}
		if d.LayerSequenceNo != nil {
			n := *d.LayerSequenceNo
			out[i].LayerSequenceNo = &n
		}
		if d.RetestValues != nil {
			out[i].RetestValues = cloneFloatMap(d.RetestValues)
		}
		out[i].Dispositions = cloneDispositions(d.Dispositions)
	}
	return out
}

func cloneDispositions(in []DeviationDisposition) []DeviationDisposition {
	if in == nil {
		return nil
	}
	out := make([]DeviationDisposition, len(in))
	for i, d := range in {
		out[i] = d
		if d.RetestValues != nil {
			out[i].RetestValues = cloneFloatMap(d.RetestValues)
		}
	}
	return out
}

func cloneFloatMap(in map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (v *VerificationResult) clone() *VerificationResult {
	if v == nil {
		return nil
	}
	out := *v
	out.Findings = cloneStrings(v.Findings)
	if v.Checks != nil {
		out.Checks = make([]VerificationCheck, len(v.Checks))
		copy(out.Checks, v.Checks)
	}
	return &out
}

func (r *ReleaseDecision) clone() *ReleaseDecision {
	if r == nil {
		return nil
	}
	out := *r
	out.ConfirmedChecklist = cloneStrings(r.ConfirmedChecklist)
	out.ReturnedItems = cloneStrings(r.ReturnedItems)
	if r.LayerSequenceNo != nil {
		n := *r.LayerSequenceNo
		out.LayerSequenceNo = &n
	}
	return &out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
