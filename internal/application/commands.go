package application

import (
	"time"
	"wellseal/internal/domain"
)

type Meta struct {
	RequestID        string `json:"request_id"`
	ExpectedRevision int64  `json:"expected_revision"`
	Actor            string `json:"actor"`
}
type CreateCaseCommand struct {
	RequestID        string  `json:"request_id"`
	Actor            string  `json:"actor"`
	WellCode         string  `json:"well_code"`
	SiteName         string  `json:"site_name"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	TotalDepthM      float64 `json:"total_depth_m"`
	CasingDiameterMM float64 `json:"casing_diameter_mm"`
	OwnerName        string  `json:"owner_name"`
}
type LockBaselineCommand struct{ Meta }
type BaselinePatchCommand struct {
	Meta
	WellCode         *string  `json:"well_code,omitempty"`
	SiteName         *string  `json:"site_name,omitempty"`
	Latitude         *float64 `json:"latitude,omitempty"`
	Longitude        *float64 `json:"longitude,omitempty"`
	TotalDepthM      *float64 `json:"total_depth_m,omitempty"`
	CasingDiameterMM *float64 `json:"casing_diameter_mm,omitempty"`
	OwnerName        *string  `json:"owner_name,omitempty"`
}
type ReviseBaselineCommand = BaselinePatchCommand
type PlanCommand struct {
	Meta
	LayerSpecs             []domain.LayerSpec `json:"layer_specs"`
	MaterialLots           []string           `json:"material_lots"`
	VolumeTolerancePercent float64            `json:"volume_tolerance_percent"`
	RequiredEvidenceTypes  []string           `json:"required_evidence_types"`
	PreparedBy             string             `json:"prepared_by"`
	ReviewedBy             string             `json:"reviewed_by"`
}
type StartCommand struct{ Meta }
type CheckpointCommand struct {
	Meta
	Items          []CheckpointItem     `json:"items,omitempty"`
	StageType      string               `json:"stage_type,omitempty"`
	DepthFromM     float64              `json:"depth_from_m,omitempty"`
	DepthToM       float64              `json:"depth_to_m,omitempty"`
	MaterialLot    string               `json:"material_lot,omitempty"`
	ActualVolumeL  float64              `json:"actual_volume_l,omitempty"`
	RecordedBy     string               `json:"recorded_by,omitempty"`
	EvidenceDigest string               `json:"evidence_digest,omitempty"`
	EvidenceTypes  []string             `json:"evidence_types,omitempty"`
	Measurements   []domain.Measurement `json:"measurements,omitempty"`
	SequenceNo     int                  `json:"sequence_no,omitempty"`
}

// CheckpointCorrectionCommand 用于在施工阶段替换已记录的单层检查点。
type CheckpointCorrectionCommand struct {
	Meta
	StageType        string               `json:"stage_type"`
	DepthFromM       float64              `json:"depth_from_m"`
	DepthToM         float64              `json:"depth_to_m"`
	MaterialLot      string               `json:"material_lot"`
	ActualVolumeL    float64              `json:"actual_volume_l"`
	RecordedBy       string               `json:"recorded_by"`
	EvidenceDigest   string               `json:"evidence_digest"`
	EvidenceTypes    []string             `json:"evidence_types"`
	Measurements     []domain.Measurement `json:"measurements"`
	SequenceNo       int                  `json:"sequence_no"`
	Reason           string               `json:"reason"`
	CorrectionReason string               `json:"correction_reason,omitempty"`
}
type CheckpointItem struct {
	StageType      string               `json:"stage_type"`
	DepthFromM     float64              `json:"depth_from_m"`
	DepthToM       float64              `json:"depth_to_m"`
	MaterialLot    string               `json:"material_lot"`
	ActualVolumeL  float64              `json:"actual_volume_l"`
	RecordedBy     string               `json:"recorded_by"`
	EvidenceDigest string               `json:"evidence_digest"`
	EvidenceTypes  []string             `json:"evidence_types"`
	Measurements   []domain.Measurement `json:"measurements"`
	SequenceNo     int                  `json:"sequence_no"`
}
type ResolveDeviationCommand struct {
	Meta
	DeviationID      string             `json:"deviation_id"`
	CorrectiveAction string             `json:"corrective_action"`
	RetestResult     string             `json:"retest_result"`
	ClosedBy         string             `json:"closed_by"`
	RetestValues     map[string]float64 `json:"retest_values,omitempty"`
}
type VerifyCommand struct {
	Meta
	PreflightDigest string `json:"preflight_digest,omitempty"`
}
type WitnessCommand struct {
	Meta
	Decision           string   `json:"decision"`
	WitnessID          string   `json:"witness_id"`
	DecisionNote       string   `json:"decision_note"`
	ConfirmedChecklist []string `json:"confirmed_checklist,omitempty"`
	ReturnedItems      []string `json:"returned_items,omitempty"`
	LayerSequenceNo    *int     `json:"layer_sequence_no,omitempty"`
	VerificationDigest string   `json:"verification_digest,omitempty"`
}
type Progress struct {
	Completed int `json:"completed"`
	Total     int `json:"total"`
	Percent   int `json:"percent"`
}
type CaseView struct {
	Case                  *domain.SealCase              `json:"case"`
	Progress              domain.ProgressSummary        `json:"progress"`
	VerificationPreflight *domain.VerificationReport    `json:"verification_preflight,omitempty"`
	WitnessChecklist      []domain.WitnessChecklistItem `json:"witness_checklist,omitempty"`
	DeviationTasks        *DeviationTaskSummary         `json:"deviation_tasks,omitempty"`
	DeviationSummary      *domain.DeviationSummary      `json:"deviation_summary,omitempty"`
	Replayed              bool                          `json:"replayed,omitempty"`
}
type CaseListFilter struct {
	Keyword     string
	State       domain.State
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	Cursor      string
	Limit       int
}

type QualitySummaryFilter struct {
	State       domain.State
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	SiteName    string
	OwnerName   string
}
type CaseListResult struct {
	Items        []domain.SealCase    `json:"items"`
	NextCursor   string               `json:"next_cursor,omitempty"`
	Total        int                  `json:"total"`
	StatusCounts map[domain.State]int `json:"status_counts"`
}
type DeviationFilter struct {
	Open         *bool
	Category     string
	Severity     string
	DetectedFrom *time.Time
	DetectedTo   *time.Time
}
type DeviationTaskSummary struct {
	Items              []domain.Deviation `json:"items"`
	OpenCount          int                `json:"open_count"`
	EarliestDetectedAt *time.Time         `json:"earliest_detected_at,omitempty"`
}

type ArchiveVerification struct {
	Archive       domain.ReleaseArchive `json:"archive"`
	Checks        []ArchiveCheck        `json:"checks"`
	EventCount    int                   `json:"event_count"`
	EvidenceCount int                   `json:"evidence_count"`
	VerifiedAt    time.Time             `json:"verified_at"`
}
type ArchiveCheck struct {
	Code    string `json:"code"`
	Passed  bool   `json:"passed"`
	Message string `json:"message"`
}
