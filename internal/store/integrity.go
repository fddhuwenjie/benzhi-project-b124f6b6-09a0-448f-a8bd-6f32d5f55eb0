package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"wellseal/internal/domain"
)

func (s *Store) CheckConsistency(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT snapshot FROM cases ORDER BY case_id`)
	if err != nil {
		return fmt.Errorf("读取个案完整性快照失败: %w", err)
	}
	cases := []domain.SealCase{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		var c domain.SealCase
		if err = json.Unmarshal(raw, &c); err != nil {
			return fmt.Errorf("个案快照无法解析: %w", err)
		}
		cases = append(cases, c)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for i := range cases {
		c := &cases[i]
		if err = s.checkCaseProjection(ctx, c); err != nil {
			return fmt.Errorf("个案 %s 数据不一致: %w", c.CaseID, err)
		}
		if err = s.checkAuditChain(ctx, c.CaseID, c.Revision); err != nil {
			return fmt.Errorf("个案 %s 审计链不一致: %w", c.CaseID, err)
		}
	}
	return nil
}

func (s *Store) checkCaseProjection(ctx context.Context, c *domain.SealCase) error {
	planCount, err := countRows(ctx, s.db, `SELECT COUNT(*) FROM seal_plans WHERE case_id=?`, c.CaseID)
	if err != nil {
		return err
	}
	if (c.Plan == nil && planCount != 0) || (c.Plan != nil && planCount != 1) {
		return fmt.Errorf("方案投影数量为 %d", planCount)
	}
	if c.Plan != nil {
		var digest string
		var raw []byte
		if err = s.db.QueryRowContext(ctx, `SELECT digest,snapshot FROM seal_plans WHERE case_id=?`, c.CaseID).Scan(&digest, &raw); err != nil {
			return err
		}
		var plan domain.SealPlan
		if err = json.Unmarshal(raw, &plan); err != nil {
			return err
		}
		if digest != c.Plan.Digest || domain.MustDigest(plan) != domain.MustDigest(*c.Plan) {
			return fmt.Errorf("方案投影内容不匹配")
		}
	}
	if err = s.checkCollectionProjection(ctx, c.CaseID, len(c.Checkpoints),
		`SELECT snapshot FROM construction_checkpoints WHERE case_id=?`,
		func(raw []byte) (string, string, error) {
			var cp domain.ConstructionCheckpoint
			if e := json.Unmarshal(raw, &cp); e != nil {
				return "", "", e
			}
			return cp.CheckpointID, domain.MustDigest(cp), nil
		},
		func(id string) (string, bool) {
			for _, cp := range c.Checkpoints {
				if cp.CheckpointID == id {
					return domain.MustDigest(cp), true
				}
			}
			return "", false
		}, "检查点"); err != nil {
		return err
	}
	if err = s.checkCollectionProjection(ctx, c.CaseID, len(c.Deviations),
		`SELECT snapshot FROM deviations WHERE case_id=?`,
		func(raw []byte) (string, string, error) {
			var d domain.Deviation
			if e := json.Unmarshal(raw, &d); e != nil {
				return "", "", e
			}
			return d.DeviationID, domain.MustDigest(d), nil
		},
		func(id string) (string, bool) {
			for _, d := range c.Deviations {
				if d.DeviationID == id {
					return domain.MustDigest(d), true
				}
			}
			return "", false
		}, "偏差"); err != nil {
		return err
	}
	releaseCount, err := countRows(ctx, s.db, `SELECT COUNT(*) FROM release_decisions WHERE case_id=?`, c.CaseID)
	if err != nil {
		return err
	}
	if (c.Release == nil && releaseCount != 0) || (c.Release != nil && releaseCount != 1) {
		return fmt.Errorf("放行决定投影数量为 %d", releaseCount)
	}
	if c.Release != nil {
		var raw []byte
		if err = s.db.QueryRowContext(ctx, `SELECT snapshot FROM release_decisions WHERE case_id=?`, c.CaseID).Scan(&raw); err != nil {
			return err
		}
		var rel domain.ReleaseDecision
		if err = json.Unmarshal(raw, &rel); err != nil {
			return fmt.Errorf("放行决定投影快照无法解析: %w", err)
		}
		if domain.MustDigest(rel) != domain.MustDigest(*c.Release) {
			return fmt.Errorf("放行决定投影内容不匹配")
		}
	}
	archiveCount, err := countRows(ctx, s.db, `SELECT COUNT(*) FROM archives WHERE case_id=?`, c.CaseID)
	if err != nil {
		return err
	}
	if c.State == domain.StateArchived && archiveCount != 1 {
		return fmt.Errorf("归档状态缺少冻结归档")
	}
	if c.State != domain.StateArchived && archiveCount != 0 {
		return fmt.Errorf("非归档状态存在冻结归档")
	}
	return nil
}

// checkCollectionProjection 校验多行投影集合（检查点、偏差）。
// 除行数外，逐行解码 snapshot 并以稳定标识与个案快照中的同标识比对摘要。
// 任何内容篡改（保持行数不变）都会因摘要不一致被报告为不一致。
func (s *Store) checkCollectionProjection(ctx context.Context, caseID string, expected int, query string,
	decode func([]byte) (id, digest string, err error),
	lookup func(id string) (digest string, ok bool), label string) error {
	rows, err := s.db.QueryContext(ctx, query, caseID)
	if err != nil {
		return fmt.Errorf("读取%s投影失败: %w", label, err)
	}
	seen := make(map[string]bool, expected)
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		id, projectedDigest, err := decode(raw)
		if err != nil {
			rows.Close()
			return fmt.Errorf("%s投影快照无法解析: %w", label, err)
		}
		expectedDigest, ok := lookup(id)
		if !ok {
			rows.Close()
			return fmt.Errorf("%s投影行 %s 不在个案快照内", label, id)
		}
		if projectedDigest != expectedDigest {
			rows.Close()
			return fmt.Errorf("%s投影行 %s 内容不匹配", label, id)
		}
		if seen[id] {
			rows.Close()
			return fmt.Errorf("%s投影行 %s 重复", label, id)
		}
		seen[id] = true
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if len(seen) != expected {
		return fmt.Errorf("%s投影去重后数量为 %d，快照为 %d", label, len(seen), expected)
	}
	return nil
}

func (s *Store) checkAuditChain(ctx context.Context, caseID string, revision int64) error {
	events, err := s.Events(ctx, caseID)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return fmt.Errorf("事件链为空")
	}
	previous := ""
	lastRevision := int64(0)
	for _, event := range events {
		if event.PreviousDigest != previous {
			return fmt.Errorf("事件 %d 前序摘要错误", event.Sequence)
		}
		if domain.EventDigest(event) != event.Digest {
			return fmt.Errorf("事件 %d 摘要错误", event.Sequence)
		}
		if event.Revision < lastRevision {
			return fmt.Errorf("事件 %d 修订号倒退", event.Sequence)
		}
		previous = event.Digest
		lastRevision = event.Revision
	}
	if lastRevision != revision {
		return fmt.Errorf("事件修订号 %d 与个案修订号 %d 不一致", lastRevision, revision)
	}
	return nil
}

type rowCounter interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func countRows(ctx context.Context, q rowCounter, query string, args ...any) (int, error) {
	var count int
	err := q.QueryRowContext(ctx, query, args...).Scan(&count)
	return count, err
}
