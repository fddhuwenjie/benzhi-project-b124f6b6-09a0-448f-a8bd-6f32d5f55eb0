package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

func CanonicalDigest(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func MustDigest(v any) string {
	d, err := CanonicalDigest(v)
	if err != nil {
		panic(err)
	}
	return d
}
func EventDigest(e AuditEvent) string {
	return MustDigest(struct {
		Sequence       int64     `json:"sequence"`
		CaseID         string    `json:"case_id"`
		Type           string    `json:"type"`
		ChangeSummary  string    `json:"change_summary,omitempty"`
		Actor          string    `json:"actor"`
		OccurredAt     time.Time `json:"occurred_at"`
		Revision       int64     `json:"revision"`
		DataDigest     string    `json:"data_digest"`
		PreviousDigest string    `json:"previous_digest"`
	}{e.Sequence, e.CaseID, e.Type, e.ChangeSummary, e.Actor, e.OccurredAt, e.Revision, e.DataDigest, e.PreviousDigest})
}
