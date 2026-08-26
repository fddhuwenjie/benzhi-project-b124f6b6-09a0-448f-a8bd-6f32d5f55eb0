package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wellseal/internal/application"
	"wellseal/internal/domain"
)

func (h *Handler) Routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /{$}", h.Index)
	m.Handle("GET /static/", http.StripPrefix("/static/", h.static))
	m.HandleFunc("GET /api/cases", h.ListCases)
	m.HandleFunc("POST /api/cases", h.CreateCase)
	m.HandleFunc("GET /api/cases/{id}", h.GetCase)
	m.HandleFunc("POST /api/cases/{id}/baseline-lock", h.LockBaseline)
	m.HandleFunc("PATCH /api/cases/{id}/baseline", h.ReviseBaseline)
	m.HandleFunc("POST /api/cases/{id}/plan", h.SetPlan)
	m.HandleFunc("POST /api/cases/{id}/plan/preflight", h.PreflightPlan)
	m.HandleFunc("POST /api/cases/{id}/start", h.StartSealing)
	m.HandleFunc("POST /api/cases/{id}/checkpoints", h.RecordCheckpoint)
	m.HandleFunc("PATCH /api/cases/{id}/checkpoints/{sequence_no}", h.CorrectCheckpoint)
	m.HandleFunc("POST /api/cases/{id}/deviations/resolve", h.ResolveDeviation)
	m.HandleFunc("GET /api/cases/{id}/deviations", h.ListDeviations)
	m.HandleFunc("GET /api/cases/{id}/deviations/summary", h.DeviationSummary)
	m.HandleFunc("GET /api/cases/{id}/verify/preflight", h.VerificationPreflight)
	m.HandleFunc("POST /api/cases/{id}/verify", h.Verify)
	m.HandleFunc("POST /api/cases/{id}/witness", h.Witness)
	m.HandleFunc("GET /api/cases/{id}/timeline", h.Timeline)
	m.HandleFunc("GET /api/cases/{id}/archive", h.DownloadArchive)
	m.HandleFunc("/", h.NotFound)
	return h.middleware(m)
}
func (h *Handler) ListCases(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("view") == "quality_summary" {
		from, err := parseTimeField(q.Get("created_from"), "created_from")
		if err != nil {
			h.fail(w, err)
			return
		}
		to, err := parseTimeField(q.Get("created_to"), "created_to")
		if err != nil {
			h.fail(w, err)
			return
		}
		out, err := h.app.QualitySummary(r.Context(), application.QualitySummaryFilter{State: domain.State(q.Get("state")), CreatedFrom: from, CreatedTo: to, SiteName: q.Get("site_name"), OwnerName: q.Get("owner_name")})
		if err != nil {
			h.fail(w, err)
			return
		}
		h.write(w, http.StatusOK, out)
		return
	}
	from, err := parseTimeField(q.Get("created_from"), "created_from")
	if err != nil {
		h.fail(w, err)
		return
	}
	to, err := parseTimeField(q.Get("created_to"), "created_to")
	if err != nil {
		h.fail(w, err)
		return
	}
	limit := 25
	if q.Get("limit") != "" {
		limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil || limit < 1 || limit > 100 {
			h.fail(w, domain.Invalid("分页大小无效", map[string]string{"limit": "必须为 1 到 100"}))
			return
		}
	}
	items, err := h.app.List(r.Context(), application.CaseListFilter{Keyword: q.Get("keyword"), State: domain.State(q.Get("state")), CreatedFrom: from, CreatedTo: to, Cursor: q.Get("cursor"), Limit: limit})
	if err != nil {
		h.fail(w, err)
		return
	}
	h.write(w, http.StatusOK, items)
}
func (h *Handler) PreflightPlan(w http.ResponseWriter, r *http.Request) {
	var cmd application.PlanCommand
	if !h.decode(w, r, &cmd) {
		return
	}
	out, err := h.app.PreflightPlan(r.Context(), r.PathValue("id"), cmd)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.write(w, http.StatusOK, out)
}
func (h *Handler) CreateCase(w http.ResponseWriter, r *http.Request) {
	var cmd application.CreateCaseCommand
	if !h.decode(w, r, &cmd) {
		return
	}
	out, err := h.app.Create(r.Context(), cmd)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.write(w, http.StatusCreated, out)
}
func (h *Handler) GetCase(w http.ResponseWriter, r *http.Request) {
	out, err := h.app.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err)
		return
	}
	h.write(w, http.StatusOK, out)
}
func (h *Handler) LockBaseline(w http.ResponseWriter, r *http.Request) {
	var cmd application.LockBaselineCommand
	if !h.decode(w, r, &cmd) {
		return
	}
	out, err := h.app.LockBaseline(r.Context(), r.PathValue("id"), cmd)
	h.result(w, out, err)
}
func (h *Handler) ReviseBaseline(w http.ResponseWriter, r *http.Request) {
	var cmd application.BaselinePatchCommand
	if !h.decode(w, r, &cmd) {
		return
	}
	out, err := h.app.ReviseBaseline(r.Context(), r.PathValue("id"), cmd)
	h.result(w, out, err)
}
func (h *Handler) SetPlan(w http.ResponseWriter, r *http.Request) {
	var cmd application.PlanCommand
	if !h.decode(w, r, &cmd) {
		return
	}
	out, err := h.app.SetPlan(r.Context(), r.PathValue("id"), cmd)
	h.result(w, out, err)
}
func (h *Handler) StartSealing(w http.ResponseWriter, r *http.Request) {
	var cmd application.StartCommand
	if !h.decode(w, r, &cmd) {
		return
	}
	out, err := h.app.Start(r.Context(), r.PathValue("id"), cmd)
	h.result(w, out, err)
}
func (h *Handler) RecordCheckpoint(w http.ResponseWriter, r *http.Request) {
	var cmd application.CheckpointCommand
	if !h.decode(w, r, &cmd) {
		return
	}
	out, err := h.app.Checkpoint(r.Context(), r.PathValue("id"), cmd)
	h.result(w, out, err)
}
func (h *Handler) CorrectCheckpoint(w http.ResponseWriter, r *http.Request) {
	var cmd application.CheckpointCorrectionCommand
	if !h.decode(w, r, &cmd) {
		return
	}
	seq, err := strconv.Atoi(r.PathValue("sequence_no"))
	if err != nil || seq < 1 {
		h.fail(w, domain.Invalid("检查点序号无效", map[string]string{"sequence_no": "必须为正整数"}))
		return
	}
	if cmd.SequenceNo != 0 && cmd.SequenceNo != seq {
		h.fail(w, domain.Invalid("检查点序号不一致", map[string]string{"sequence_no": "必须与路径序号一致"}))
		return
	}
	cmd.SequenceNo = seq
	out, err := h.app.CorrectCheckpoint(r.Context(), r.PathValue("id"), cmd)
	h.result(w, out, err)
}
func (h *Handler) ResolveDeviation(w http.ResponseWriter, r *http.Request) {
	var cmd application.ResolveDeviationCommand
	if !h.decode(w, r, &cmd) {
		return
	}
	out, err := h.app.ResolveDeviation(r.Context(), r.PathValue("id"), cmd)
	h.result(w, out, err)
}
func (h *Handler) ListDeviations(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var open *bool
	if q.Get("open") != "" {
		v, err := strconv.ParseBool(q.Get("open"))
		if err != nil {
			h.fail(w, domain.Invalid("偏差状态筛选无效", map[string]string{"open": "必须为 true 或 false"}))
			return
		}
		open = &v
	}
	from, err := parseTimeField(q.Get("detected_from"), "detected_from")
	if err != nil {
		h.fail(w, err)
		return
	}
	to, err := parseTimeField(q.Get("detected_to"), "detected_to")
	if err != nil {
		h.fail(w, err)
		return
	}
	if err = domain.ValidateDetectedTimeRange(from, to); err != nil {
		h.fail(w, err)
		return
	}
	out, err := h.app.Deviations(r.Context(), r.PathValue("id"), application.DeviationFilter{Open: open, Category: q.Get("category"), Severity: q.Get("severity"), DetectedFrom: from, DetectedTo: to})
	if err != nil {
		h.fail(w, err)
		return
	}
	h.write(w, http.StatusOK, out)
}
func (h *Handler) DeviationSummary(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var open *bool
	if q.Get("open") != "" {
		v, err := strconv.ParseBool(q.Get("open"))
		if err != nil {
			h.fail(w, domain.Invalid("偏差状态筛选无效", map[string]string{"open": "必须为 true 或 false"}))
			return
		}
		open = &v
	}
	from, err := parseTimeField(q.Get("detected_from"), "detected_from")
	if err != nil {
		h.fail(w, err)
		return
	}
	to, err := parseTimeField(q.Get("detected_to"), "detected_to")
	if err != nil {
		h.fail(w, err)
		return
	}
	if err = domain.ValidateDetectedTimeRange(from, to); err != nil {
		h.fail(w, err)
		return
	}
	out, err := h.app.DeviationSummary(r.Context(), r.PathValue("id"), application.DeviationFilter{Open: open, Category: q.Get("category"), Severity: q.Get("severity"), DetectedFrom: from, DetectedTo: to})
	if err != nil {
		h.fail(w, err)
		return
	}
	h.write(w, http.StatusOK, out)
}
func (h *Handler) VerificationPreflight(w http.ResponseWriter, r *http.Request) {
	out, err := h.app.VerificationPreflight(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err)
		return
	}
	h.write(w, http.StatusOK, out)
}
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	var cmd application.VerifyCommand
	if !h.decode(w, r, &cmd) {
		return
	}
	out, err := h.app.Verify(r.Context(), r.PathValue("id"), cmd)
	h.result(w, out, err)
}
func (h *Handler) Witness(w http.ResponseWriter, r *http.Request) {
	var cmd application.WitnessCommand
	if !h.decode(w, r, &cmd) {
		return
	}
	out, err := h.app.Witness(r.Context(), r.PathValue("id"), cmd)
	h.result(w, out, err)
}
func (h *Handler) Timeline(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	out, err := h.app.Timeline(r.Context(), r.PathValue("id"), after)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.write(w, http.StatusOK, out)
}
func (h *Handler) DownloadArchive(w http.ResponseWriter, r *http.Request) {
	verified, err := h.app.Archive(r.Context(), r.PathValue("id"))
	if err != nil {
		h.fail(w, err)
		return
	}
	a := verified.Archive
	if r.URL.Query().Get("view") == "summary" {
		h.write(w, http.StatusOK, map[string]any{"archive_id": a.ArchiveID, "case_id": a.CaseID, "generated_at": a.GeneratedAt, "manifest_digest": a.ManifestDigest, "verification_digest": a.VerificationDigest, "checklist_digest": a.ChecklistDigest, "event_chain_head": a.EventChainHead, "event_count": verified.EventCount, "evidence_count": verified.EvidenceCount, "checks": verified.Checks, "verified_at": verified.VerifiedAt})
		return
	}
	etag := `"` + a.ManifestDigest + `"`
	w.Header().Set("ETag", etag)
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+a.CaseID+`.json"`)
	w.Write(a.Payload)
}
func (h *Handler) NotFound(w http.ResponseWriter, r *http.Request) {
	h.write(w, http.StatusNotFound, map[string]any{"error": domain.BusinessError{Code: domain.CodeNotFound, Message: "路由不存在"}})
}
func (h *Handler) result(w http.ResponseWriter, out application.CaseView, err error) {
	if err != nil {
		h.fail(w, err)
		return
	}
	h.write(w, http.StatusOK, out)
}
func (h *Handler) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		msg := "请求 JSON 无效"
		var max *http.MaxBytesError
		if errors.As(err, &max) {
			msg = "请求体超过 1 MiB"
		}
		h.write(w, http.StatusBadRequest, map[string]any{"error": domain.BusinessError{Code: domain.CodeInvalid, Message: msg}})
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		h.write(w, http.StatusBadRequest, map[string]any{"error": domain.BusinessError{Code: domain.CodeInvalid, Message: "请求只能包含一个 JSON 对象"}})
		return false
	}
	return true
}
func (h *Handler) fail(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	be := &domain.BusinessError{Code: "internal", Message: "服务内部错误"}
	var business *domain.BusinessError
	if errors.As(err, &business) {
		be = business
		switch business.Code {
		case domain.CodeInvalid:
			status = http.StatusBadRequest
		case domain.CodeNotFound:
			status = http.StatusNotFound
		case domain.CodeConflict, domain.CodeIdempotency, domain.CodeIntegrity:
			status = http.StatusConflict
		case domain.CodeGate, domain.CodeFrozen:
			status = http.StatusUnprocessableEntity
		}
	} else {
		h.log.Error("请求处理失败", "error", err)
	}
	h.write(w, status, map[string]any{"error": be})
}
func (h *Handler) write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func parseTimeField(raw, field string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	v, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, domain.Invalid("时间参数无效", map[string]string{field: "必须为 RFC3339 时间"})
	}
	v = v.UTC()
	return &v, nil
}
func etagMatches(header, current string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		if strings.HasPrefix(candidate, "W/") {
			candidate = strings.TrimSpace(strings.TrimPrefix(candidate, "W/"))
		}
		if candidate == current {
			return true
		}
	}
	return false
}
