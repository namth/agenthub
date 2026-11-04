package pipeline

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/namth/agenthub/internal/dispatcher"
)

// Reconcile bù job còn thiếu để dây chuyền không kẹt: task PLANNING không có SPEC/REPLAN
// đang chờ hoặc đang chạy → tạo SPEC; sub-task READY có nhánh mà không có job CODE đang
// chờ/chạy → tạo CODE. Gọi định kỳ từ main; chạy lại nhiều lần không sinh job trùng.
func (p *Pipeline) Reconcile(ctx context.Context) (int, error) {
	created := 0
	missingSpec, err := p.idsMissingJob(ctx, `
		SELECT t.id, '' FROM tasks t
		WHERE t.project_id = ? AND t.status = 'PLANNING' AND NOT EXISTS (
			SELECT 1 FROM jobs j WHERE j.task_id = t.id AND j.kind IN ('SPEC', 'REPLAN') AND j.status IN ('QUEUED', 'RUNNING'))`)
	if err != nil {
		return 0, err
	}
	for _, m := range missingSpec {
		if _, err := p.disp.EnqueueJob(dispatcher.NewJob{ProjectID: p.projectID, TaskID: m[0], Kind: dispatcher.JobSpec, RequiredAgent: dispatcher.KindClaude}); err != nil {
			return created, fmt.Errorf("reconcile spec %s: %w", m[0], err)
		}
		created++
	}
	missingCode, err := p.idsMissingJob(ctx, `
		SELECT t.id, (SELECT j.branch_name FROM jobs j WHERE j.task_id = t.id AND j.kind = 'CODE' AND j.branch_name IS NOT NULL ORDER BY j.created_at DESC LIMIT 1)
		FROM tasks t
		WHERE t.project_id = ? AND t.status = 'READY' AND t.parent_id IS NOT NULL AND NOT EXISTS (
			SELECT 1 FROM jobs j WHERE j.task_id = t.id AND j.kind = 'CODE' AND j.status IN ('QUEUED', 'RUNNING'))`)
	if err != nil {
		return created, err
	}
	for _, m := range missingCode {
		if m[1] == "" {
			continue // chưa từng có nhánh: không tự đoán, để lead xử lý
		}
		if _, err := p.disp.EnqueueJob(dispatcher.NewJob{ProjectID: p.projectID, TaskID: m[0], Kind: dispatcher.JobCode, RequiredAgent: dispatcher.KindAntigravity, BranchName: m[1]}); err != nil {
			return created, fmt.Errorf("reconcile code %s: %w", m[0], err)
		}
		created++
	}
	return created, nil
}

func (p *Pipeline) idsMissingJob(ctx context.Context, q string) ([][2]string, error) {
	rows, err := p.db.QueryContext(ctx, q, p.projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var id string
		var branch sql.NullString
		if err := rows.Scan(&id, &branch); err != nil {
			return nil, err
		}
		out = append(out, [2]string{id, branch.String})
	}
	return out, rows.Err()
}
