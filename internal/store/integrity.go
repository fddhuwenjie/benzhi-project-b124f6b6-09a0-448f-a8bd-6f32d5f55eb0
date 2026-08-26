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
	cpCount, err := countRows(ctx, s.db, `SELECT COUNT(*) FROM construction_checkpoints WHERE case_id=?`, c.CaseID)
	if err != nil {
		return err
	}
	if cpCount != len(c.Checkpoints) {
		return fmt.Errorf("检查点投影数量为 %d，快照为 %d", cpCount, len(c.Checkpoints))
	}
	devCount, err := countRows(ctx, s.db, `SELECT COUNT(*) FROM deviations WHERE case_id=?`, c.CaseID)
	if err != nil {
		return err
	}
	if devCount != len(c.Deviations) {
		return fmt.Errorf("偏差投影数量为 %d，快照为 %d", devCount, len(c.Deviations))
	}
	releaseCount, err := countRows(ctx, s.db, `SELECT COUNT(*) FROM release_decisions WHERE case_id=?`, c.CaseID)
	if err != nil {
		return err
	}
	if (c.Release == nil && releaseCount != 0) || (c.Release != nil && releaseCount != 1) {
		return fmt.Errorf("放行决定投影数量为 %d", releaseCount)
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
