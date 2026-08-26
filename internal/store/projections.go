package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"wellseal/internal/domain"
)

func syncProjections(ctx context.Context, tx *sql.Tx, c *domain.SealCase) error {
	if err := syncPlan(ctx, tx, c); err != nil {
		return err
	}
	if err := replaceCheckpoints(ctx, tx, c); err != nil {
		return err
	}
	if err := replaceDeviations(ctx, tx, c); err != nil {
		return err
	}
	if err := syncRelease(ctx, tx, c); err != nil {
		return err
	}
	return nil
}

func syncPlan(ctx context.Context, tx *sql.Tx, c *domain.SealCase) error {
	if c.Plan == nil {
		return nil
	}
	snapshot, err := json.Marshal(c.Plan)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO seal_plans(plan_id,case_id,digest,locked_at,snapshot) VALUES(?,?,?,?,?)
		ON CONFLICT(case_id) DO UPDATE SET plan_id=excluded.plan_id,digest=excluded.digest,locked_at=excluded.locked_at,snapshot=excluded.snapshot`,
		c.Plan.PlanID, c.CaseID, c.Plan.Digest, c.Plan.LockedAt.Format(time.RFC3339Nano), snapshot)
	if err != nil {
		return fmt.Errorf("写入方案投影失败: %w", err)
	}
	return nil
}

func replaceCheckpoints(ctx context.Context, tx *sql.Tx, c *domain.SealCase) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM construction_checkpoints WHERE case_id=?`, c.CaseID); err != nil {
		return err
	}
	for _, checkpoint := range c.Checkpoints {
		snapshot, err := json.Marshal(checkpoint)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO construction_checkpoints(checkpoint_id,case_id,sequence_no,stage_type,measured_at,snapshot) VALUES(?,?,?,?,?,?)`, checkpoint.CheckpointID, c.CaseID, checkpoint.SequenceNo, checkpoint.StageType, checkpoint.MeasuredAt.Format(time.RFC3339Nano), snapshot)
		if err != nil {
			return fmt.Errorf("写入检查点投影失败: %w", err)
		}
	}
	return nil
}

func replaceDeviations(ctx context.Context, tx *sql.Tx, c *domain.SealCase) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM deviations WHERE case_id=?`, c.CaseID); err != nil {
		return err
	}
	for _, deviation := range c.Deviations {
		snapshot, err := json.Marshal(deviation)
		if err != nil {
			return err
		}
		var closed any
		if deviation.ClosedAt != nil {
			closed = deviation.ClosedAt.Format(time.RFC3339Nano)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO deviations(deviation_id,case_id,category,detected_at,closed_at,snapshot) VALUES(?,?,?,?,?,?)`, deviation.DeviationID, c.CaseID, deviation.Category, deviation.DetectedAt.Format(time.RFC3339Nano), closed, snapshot)
		if err != nil {
			return fmt.Errorf("写入偏差投影失败: %w", err)
		}
	}
	return nil
}

func syncRelease(ctx context.Context, tx *sql.Tx, c *domain.SealCase) error {
	if c.Release == nil {
		return nil
	}
	snapshot, err := json.Marshal(c.Release)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO release_decisions(case_id,decision,witness_id,decided_at,snapshot) VALUES(?,?,?,?,?)
		ON CONFLICT(case_id) DO UPDATE SET decision=excluded.decision,witness_id=excluded.witness_id,decided_at=excluded.decided_at,snapshot=excluded.snapshot`, c.CaseID, c.Release.Decision, c.Release.WitnessID, c.Release.DecidedAt.Format(time.RFC3339Nano), snapshot)
	if err != nil {
		return fmt.Errorf("写入见证决定投影失败: %w", err)
	}
	return nil
}
