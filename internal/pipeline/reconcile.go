package pipeline

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/namth/agenthub/internal/dispatcher"
)

type missingItem struct {
	taskID    string
	projectID string
	branch    string
}

// Reconcile bù job còn thiếu để dây chuyền không kẹt: task PLANNING không có SPEC/REPLAN
// đang chờ hoặc đang chạy → tạo SPEC; sub-task READY có nhánh mà không có job CODE đang
// chờ/chạy → tạo CODE. Gọi định kỳ từ main; chạy lại nhiều lần không sinh job trùng.
func (p *Pipeline) Reconcile(ctx context.Context) (int, error) {
	created := 0
	isMulti := p.projectID == "" || p.ghFor != nil

	var specQ string
	var specArgs []any
	if isMulti {
		specQ = `
			SELECT t.id, t.project_id, '' FROM tasks t
			JOIN projects pr ON pr.id = t.project_id
			WHERE pr.status = 'ACTIVE' AND t.status = 'PLANNING' AND NOT EXISTS (
				SELECT 1 FROM jobs j WHERE j.task_id = t.id AND j.kind IN ('SPEC', 'REPLAN') AND j.status IN ('QUEUED', 'RUNNING'))`
	} else {
		specQ = `
			SELECT t.id, t.project_id, '' FROM tasks t
			WHERE t.project_id = ? AND t.status = 'PLANNING' AND NOT EXISTS (
				SELECT 1 FROM jobs j WHERE j.task_id = t.id AND j.kind IN ('SPEC', 'REPLAN') AND j.status IN ('QUEUED', 'RUNNING'))`
		specArgs = []any{p.projectID}
	}

	missingSpec, err := p.idsMissingJob(ctx, specQ, specArgs...)
	if err != nil {
		return 0, err
	}
	for _, m := range missingSpec {
		proj := m.projectID
		if proj == "" {
			proj = p.projectID
		}
		if _, err := p.disp.EnqueueJob(dispatcher.NewJob{ProjectID: proj, TaskID: m.taskID, Kind: dispatcher.JobSpec, RequiredAgent: dispatcher.KindClaude}); err != nil {
			return created, fmt.Errorf("reconcile spec %s: %w", m.taskID, err)
		}
		created++
	}

	var codeQ string
	var codeArgs []any
	if isMulti {
		codeQ = `
			SELECT t.id, t.project_id, (SELECT j.branch_name FROM jobs j WHERE j.task_id = t.id AND j.kind = 'CODE' AND j.branch_name IS NOT NULL ORDER BY j.created_at DESC LIMIT 1)
			FROM tasks t
			JOIN projects pr ON pr.id = t.project_id
			WHERE pr.status = 'ACTIVE' AND t.status = 'READY' AND t.parent_id IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM jobs j WHERE j.task_id = t.id AND j.kind = 'CODE' AND j.status IN ('QUEUED', 'RUNNING'))`
	} else {
		codeQ = `
			SELECT t.id, t.project_id, (SELECT j.branch_name FROM jobs j WHERE j.task_id = t.id AND j.kind = 'CODE' AND j.branch_name IS NOT NULL ORDER BY j.created_at DESC LIMIT 1)
			FROM tasks t
			WHERE t.project_id = ? AND t.status = 'READY' AND t.parent_id IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM jobs j WHERE j.task_id = t.id AND j.kind = 'CODE' AND j.status IN ('QUEUED', 'RUNNING'))`
		codeArgs = []any{p.projectID}
	}

	missingCode, err := p.idsMissingJob(ctx, codeQ, codeArgs...)
	if err != nil {
		return created, err
	}
	for _, m := range missingCode {
		if m.branch == "" {
			continue // chưa từng có nhánh: không tự đoán, để lead xử lý
		}
		proj := m.projectID
		if proj == "" {
			proj = p.projectID
		}
		if _, err := p.disp.EnqueueJob(dispatcher.NewJob{ProjectID: proj, TaskID: m.taskID, Kind: dispatcher.JobCode, RequiredAgent: dispatcher.KindAntigravity, BranchName: m.branch}); err != nil {
			return created, fmt.Errorf("reconcile code %s: %w", m.taskID, err)
		}
		created++
	}
	return created, nil
}

func (p *Pipeline) idsMissingJob(ctx context.Context, q string, args ...any) ([]missingItem, error) {
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []missingItem
	for rows.Next() {
		var id, projID string
		var branch sql.NullString
		if err := rows.Scan(&id, &projID, &branch); err != nil {
			return nil, err
		}
		out = append(out, missingItem{taskID: id, projectID: projID, branch: branch.String})
	}
	return out, rows.Err()
}
