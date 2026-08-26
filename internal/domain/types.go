package domain

import "time"

type State string

const (
	StateDraft          State = "draft"
	StateBaselineLocked State = "baseline_locked"
	StateSealing        State = "sealing"
	StateHeld           State = "held"
	StateVerification   State = "verification"
	StateReleased       State = "released"
	StateArchived       State = "archived"
)

type SealCase struct {
	CaseID           string                   `json:"case_id"`
	WellCode         string                   `json:"well_code"`
	SiteName         string                   `json:"site_name"`
	Latitude         float64                  `json:"latitude"`
	Longitude        float64                  `json:"longitude"`
	TotalDepthM      float64                  `json:"total_depth_m"`
	CasingDiameterMM float64                  `json:"casing_diameter_mm"`
	OwnerName        string                   `json:"owner_name"`
	State            State                    `json:"state"`
	Revision         int64                    `json:"revision"`
	CreatedAt        time.Time                `json:"created_at"`
	ArchivedAt       *time.Time               `json:"archived_at,omitempty"`
	Plan             *SealPlan                `json:"plan,omitempty"`
	Checkpoints      []ConstructionCheckpoint `json:"checkpoints"`
	Deviations       []Deviation              `json:"deviations"`
	Verification     *VerificationResult      `json:"verification,omitempty"`
	Release          *ReleaseDecision         `json:"release,omitempty"`
}
type LayerSpec struct {
	SequenceNo            int      `json:"sequence_no"`
	DepthFromM            float64  `json:"depth_from_m"`
	DepthToM              float64  `json:"depth_to_m"`
	MaterialLot           string   `json:"material_lot"`
	TargetVolumeL         float64  `json:"target_volume_l"`
	StageType             string   `json:"stage_type"`
	RequiredEvidenceTypes []string `json:"required_evidence_types,omitempty"`
}
type SealPlan struct {
	PlanID                 string      `json:"plan_id"`
	CaseID                 string      `json:"case_id"`
	LayerSpecs             []LayerSpec `json:"layer_specs"`
	MaterialLots           []string    `json:"material_lots"`
	VolumeTolerancePercent float64     `json:"volume_tolerance_percent"`
	RequiredEvidenceTypes  []string    `json:"required_evidence_types"`
	PreparedBy             string      `json:"prepared_by"`
	ReviewedBy             string      `json:"reviewed_by"`
	LockedAt               time.Time   `json:"locked_at"`
	Digest                 string      `json:"digest"`
}
type Measurement struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}
type ConstructionCheckpoint struct {
	CheckpointID   string        `json:"checkpoint_id"`
	CaseID         string        `json:"case_id"`
	StageType      string        `json:"stage_type"`
	DepthFromM     float64       `json:"depth_from_m"`
	DepthToM       float64       `json:"depth_to_m"`
	MaterialLot    string        `json:"material_lot"`
	ActualVolumeL  float64       `json:"actual_volume_l"`
	MeasuredAt     time.Time     `json:"measured_at"`
	RecordedBy     string        `json:"recorded_by"`
	EvidenceDigest string        `json:"evidence_digest"`
	EvidenceTypes  []string      `json:"evidence_types"`
	Measurements   []Measurement `json:"measurements"`
	SequenceNo     int           `json:"sequence_no"`
}
type Deviation struct {
	DeviationID      string                 `json:"deviation_id"`
	CaseID           string                 `json:"case_id"`
	Category         string                 `json:"category"`
	Description      string                 `json:"description"`
	DetectedAt       time.Time              `json:"detected_at"`
	CorrectiveAction string                 `json:"corrective_action,omitempty"`
	RetestResult     string                 `json:"retest_result,omitempty"`
	ClosedBy         string                 `json:"closed_by,omitempty"`
	ClosedAt         *time.Time             `json:"closed_at,omitempty"`
	Severity         string                 `json:"severity"`
	LayerSequenceNo  *int                   `json:"layer_sequence_no,omitempty"`
	Source           string                 `json:"source,omitempty"`
	RetestValues     map[string]float64     `json:"retest_values,omitempty"`
	Dispositions     []DeviationDisposition `json:"dispositions,omitempty"`
}
type DeviationDisposition struct {
	CorrectiveAction string             `json:"corrective_action"`
	RetestResult     string             `json:"retest_result"`
	ClosedBy         string             `json:"closed_by"`
	RetestValues     map[string]float64 `json:"retest_values,omitempty"`
	OccurredAt       time.Time          `json:"occurred_at"`
}

type DeviationSummaryItem struct {
	DeviationID              string     `json:"deviation_id"`
	Category                 string     `json:"category"`
	Severity                 string     `json:"severity"`
	DetectedAt               time.Time  `json:"detected_at"`
	ClosedAt                 *time.Time `json:"closed_at,omitempty"`
	Open                     bool       `json:"open"`
	DispositionHours         float64    `json:"disposition_hours"`
	DispositionDurationHours float64    `json:"disposition_duration_hours"`
	FailedRetests            int        `json:"failed_retests"`
	PassedRetests            int        `json:"passed_retests"`
	LatestRetest             string     `json:"latest_retest,omitempty"`
	LatestRetestResult       string     `json:"latest_retest_result,omitempty"`
}

type DeviationSummaryGroup struct {
	Key                     string                 `json:"key"`
	Category                string                 `json:"category"`
	Severity                string                 `json:"severity"`
	Total                   int                    `json:"total"`
	Open                    int                    `json:"open"`
	Closed                  int                    `json:"closed"`
	OpenCount               int                    `json:"open_count"`
	ClosedCount             int                    `json:"closed_count"`
	AverageDispositionHours float64                `json:"average_disposition_hours"`
	EarliestDetectedAt      *time.Time             `json:"earliest_detected_at,omitempty"`
	LatestDetectedAt        *time.Time             `json:"latest_detected_at,omitempty"`
	Items                   []DeviationSummaryItem `json:"items"`
}

type QualityVerificationCounts struct {
	Passed     int `json:"passed"`
	Failed     int `json:"failed"`
	Unverified int `json:"unverified"`
	Archived   int `json:"archived"`
}

type QualitySummary struct {
	TotalCases             int                       `json:"total_cases"`
	StatusCounts           map[State]int             `json:"status_counts"`
	VerificationCounts     QualityVerificationCounts `json:"verification_counts"`
	DeviationGroups        []DeviationSummaryGroup   `json:"deviation_groups"`
	QueriedAt              time.Time                 `json:"queried_at"`
	DataRevisionUpperBound int64                     `json:"data_revision_upper_bound"`
}

type DeviationSummary struct {
	Total       int                     `json:"total"`
	Open        int                     `json:"open"`
	Closed      int                     `json:"closed"`
	OpenCount   int                     `json:"open_count"`
	ClosedCount int                     `json:"closed_count"`
	Groups      []DeviationSummaryGroup `json:"groups"`
}
type VerificationCheck struct {
	Group      string `json:"group"`
	Code       string `json:"code"`
	Passed     bool   `json:"passed"`
	SequenceNo int    `json:"sequence_no,omitempty"`
	Message    string `json:"message"`
}
type VerificationResult struct {
	Passed         bool                `json:"passed"`
	Findings       []string            `json:"findings"`
	Digest         string              `json:"digest"`
	VerifiedAt     time.Time           `json:"verified_at"`
	Checks         []VerificationCheck `json:"checks"`
	SourceRevision int64               `json:"source_revision"`
	PlanDigest     string              `json:"plan_digest"`
}
type WitnessChecklistItem struct {
	Code         string `json:"code"`
	Required     bool   `json:"required"`
	SourceDigest string `json:"source_digest"`
	Description  string `json:"description"`
}
type ReleaseDecision struct {
	Decision           string    `json:"decision"`
	WitnessID          string    `json:"witness_id"`
	DecisionNote       string    `json:"decision_note"`
	DecidedAt          time.Time `json:"decided_at"`
	ConfirmedChecklist []string  `json:"confirmed_checklist"`
	ChecklistDigest    string    `json:"checklist_digest"`
	ReturnedItems      []string  `json:"returned_items,omitempty"`
	LayerSequenceNo    *int      `json:"layer_sequence_no,omitempty"`
}
type ReleaseArchive struct {
	ArchiveID          string    `json:"archive_id"`
	CaseID             string    `json:"case_id"`
	Decision           string    `json:"decision"`
	WitnessID          string    `json:"witness_id"`
	DecisionNote       string    `json:"decision_note"`
	VerificationDigest string    `json:"verification_digest"`
	ManifestDigest     string    `json:"manifest_digest"`
	EventChainHead     string    `json:"event_chain_head"`
	GeneratedAt        time.Time `json:"generated_at"`
	Payload            []byte    `json:"-"`
	ChecklistDigest    string    `json:"checklist_digest"`
}
type AuditEvent struct {
	Sequence       int64     `json:"sequence"`
	CaseID         string    `json:"case_id"`
	Type           string    `json:"type"`
	ChangeSummary  string    `json:"change_summary,omitempty"`
	Actor          string    `json:"actor"`
	OccurredAt     time.Time `json:"occurred_at"`
	Revision       int64     `json:"revision"`
	DataDigest     string    `json:"data_digest"`
	PreviousDigest string    `json:"previous_digest"`
	Digest         string    `json:"digest"`
}
type Page[T any] struct {
	Items      []T   `json:"items"`
	NextCursor int64 `json:"next_cursor,omitempty"`
}
