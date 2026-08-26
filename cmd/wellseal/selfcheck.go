package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"wellseal/internal/application"
)

func runSelfCheck(addr string, log *slog.Logger) error {
	dir, err := os.MkdirTemp("", "wellseal-self-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	parts, err := assemble(filepath.Join(dir, "selfcheck.db"), log)
	if err != nil {
		return err
	}
	defer parts.store.Close()
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("自检监听失败: %w", err)
	}
	srv := newHTTPServer(addr, parts.handler)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		<-done
	}()
	client := &http.Client{Timeout: 5 * time.Second}
	base := "http://" + addr
	if res, err := client.Get(base + "/"); err != nil {
		return err
	} else {
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			return fmt.Errorf("工作台返回 %s", res.Status)
		}
	}
	var v application.CaseView
	if err = postJSON(client, base+"/api/cases", map[string]any{"request_id": "sc-create", "actor": "工程师甲", "well_code": "SC-WELL-001", "site_name": "自检场地", "latitude": 31.2, "longitude": 121.4, "total_depth_m": 20, "casing_diameter_mm": 100, "owner_name": "场地责任人"}, &v); err != nil {
		return err
	}
	id := v.Case.CaseID
	if err = postJSON(client, base+"/api/cases/"+id+"/baseline-lock", map[string]any{"request_id": "sc-lock", "expected_revision": v.Case.Revision, "actor": "工程师甲"}, &v); err != nil {
		return err
	}
	plan := map[string]any{"request_id": "sc-plan", "expected_revision": v.Case.Revision, "actor": "复核乙", "layer_specs": []map[string]any{{"sequence_no": 1, "depth_from_m": 0, "depth_to_m": 20, "material_lot": "LOT-SC", "target_volume_l": 100, "stage_type": "placement"}}, "material_lots": []string{"LOT-SC"}, "volume_tolerance_percent": 10, "required_evidence_types": []string{"photo", "field_log"}, "prepared_by": "工程师甲", "reviewed_by": "复核乙"}
	if err = postJSON(client, base+"/api/cases/"+id+"/plan", plan, &v); err != nil {
		return err
	}
	if err = postJSON(client, base+"/api/cases/"+id+"/start", map[string]any{"request_id": "sc-start", "expected_revision": v.Case.Revision, "actor": "工程师甲"}, &v); err != nil {
		return err
	}
	bad := map[string]any{"request_id": "sc-bad-cp", "expected_revision": v.Case.Revision, "actor": "施工员丙", "stage_type": "placement", "depth_from_m": 0, "depth_to_m": 20, "material_lot": "WRONG", "actual_volume_l": 100, "recorded_by": "施工员丙", "evidence_digest": "bad-evidence", "evidence_types": []string{"photo", "field_log"}, "measurements": []map[string]any{{"name": "terminal_elevation", "value": 0, "unit": "m"}}, "sequence_no": 1}
	if err = postJSON(client, base+"/api/cases/"+id+"/checkpoints", bad, &v); err != nil {
		return err
	}
	if v.Case.State != "held" || len(v.Case.Deviations) != 1 {
		return fmt.Errorf("不合格检查点未触发暂停")
	}
	dev := v.Case.Deviations[0].DeviationID
	resolve := map[string]any{"request_id": "sc-resolve", "expected_revision": v.Case.Revision, "actor": "复核乙", "deviation_id": dev, "corrective_action": "更换为方案指定批次并复核标签", "retest_result": "passed", "closed_by": "复核乙", "retest_values": map[string]float64{"material_lot_verified": 1}}
	if err = postJSON(client, base+"/api/cases/"+id+"/deviations/resolve", resolve, &v); err != nil {
		return err
	}
	good := map[string]any{"request_id": "sc-good-cp", "expected_revision": v.Case.Revision, "actor": "施工员丙", "stage_type": "placement", "depth_from_m": 0, "depth_to_m": 20, "material_lot": "LOT-SC", "actual_volume_l": 100, "recorded_by": "施工员丙", "evidence_digest": "sha256-selfcheck-evidence", "evidence_types": []string{"photo", "field_log"}, "measurements": []map[string]any{{"name": "terminal_elevation", "value": 0, "unit": "m"}}, "sequence_no": 1}
	if err = postJSON(client, base+"/api/cases/"+id+"/checkpoints", good, &v); err != nil {
		return err
	}
	if err = postJSON(client, base+"/api/cases/"+id+"/verify", map[string]any{"request_id": "sc-verify", "expected_revision": v.Case.Revision, "actor": "工程师甲"}, &v); err != nil {
		return err
	}
	if v.Case.Verification == nil || !v.Case.Verification.Passed {
		return fmt.Errorf("完整性验证未通过")
	}
	if err = postJSON(client, base+"/api/cases/"+id+"/witness", map[string]any{"request_id": "sc-release", "expected_revision": v.Case.Revision, "actor": "见证人丁", "decision": "release", "witness_id": "见证人丁", "decision_note": "证据与事件链完整，同意放行", "confirmed_checklist": []string{"plan", "verification", "deviations", "recorders", "event_chain"}, "verification_digest": v.Case.Verification.Digest}, &v); err != nil {
		return err
	}
	if v.Case.State != "archived" {
		return fmt.Errorf("最终状态为 %s，期望 archived", v.Case.State)
	}
	res, err := client.Get(base + "/api/cases/" + id + "/archive")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("ETag") == "" {
		return fmt.Errorf("归档下载校验失败: %s", res.Status)
	}
	etag := res.Header.Get("ETag")
	res.Body.Close()
	eventsBefore, err := parts.store.Events(context.Background(), id)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodGet, base+"/api/cases/"+id+"/archive", nil)
	if err != nil {
		return err
	}
	req.Header.Set("If-None-Match", etag)
	conditional, err := client.Do(req)
	if err != nil {
		return err
	}
	conditional.Body.Close()
	if conditional.StatusCode != http.StatusNotModified {
		return fmt.Errorf("归档条件下载返回 %s，期望 304", conditional.Status)
	}
	eventsAfter, err := parts.store.Events(context.Background(), id)
	if err != nil || len(eventsAfter) != len(eventsBefore) {
		return fmt.Errorf("归档只读请求改变了审计事件数量")
	}
	if err := parts.store.CheckConsistency(context.Background()); err != nil {
		return fmt.Errorf("自检数据库一致性失败: %w", err)
	}
	return nil
}
func postJSON(client *http.Client, url string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	res, err := client.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		var failure any
		json.NewDecoder(res.Body).Decode(&failure)
		return fmt.Errorf("POST %s 返回 %s: %v", url, res.Status, failure)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
