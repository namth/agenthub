package tasks_test

// Test nghiệm thu TASK-018 — do trưởng phòng viết, file bị khoá (tests.lock).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/namth/agenthub/internal/tasks"
)

type fakeMerger struct {
	merged []int
	err    error
}

func (m *fakeMerger) MergePR(_ context.Context, number int) error {
	m.merged = append(m.merged, number)
	return m.err
}

func TestMergeEndpoint(t *testing.T) {
	r := newRig(t)
	m := &fakeMerger{}
	var done []string
	svc := tasks.NewService(r.db, r.gh, r.disp, "p1", tasks.WithMerger(m), tasks.WithOnMerged(func(prURL string) { done = append(done, prURL) }))
	r.h = tasks.HandlerWithService(svc, authz)
	r.db.Exec(`INSERT INTO tasks (id, project_id, title, description, created_by, status, pr_url) VALUES ('s1', 'p1', 'A', 'B', 'admin1', 'QA_PASSED', 'https://github.com/namth/agenthub/pull/15')`)
	r.db.Exec(`INSERT INTO tasks (id, project_id, title, description, created_by, status) VALUES ('s2', 'p1', 'C', 'D', 'admin1', 'WAITING_QA')`)

	if code, e := r.do("POST", "/api/tasks/s1/merge", "partner1:PARTNER", nil); code != 403 || e.Error.Code != "FORBIDDEN" {
		t.Fatalf("partner merge: %d", code)
	}
	if code, e := r.do("POST", "/api/tasks/s2/merge", "admin1:ADMIN", nil); code != 409 || e.Error.Code != "INVALID_TRANSITION" {
		t.Fatalf("chưa QA_PASSED: %d %+v, muốn 409", code, e.Error)
	}
	code, e := r.do("POST", "/api/tasks/s1/merge", "admin1:ADMIN", nil)
	if code != 200 || !e.Success {
		t.Fatalf("merge = %d %+v", code, e.Error)
	}
	if len(m.merged) != 1 || m.merged[0] != 15 {
		t.Fatalf("MergePR = %v, muốn số PR 15 lấy từ pr_url", m.merged)
	}
	if r.status("s1") != "DONE" || len(done) != 1 || done[0] != "https://github.com/namth/agenthub/pull/15" {
		t.Fatalf("sau merge: %s, OnMerged %v", r.status("s1"), done)
	}
}

func TestMergeErrors(t *testing.T) {
	r := newRig(t)
	m := &fakeMerger{err: errors.New("405 not mergeable")}
	svc := tasks.NewService(r.db, r.gh, r.disp, "p1", tasks.WithMerger(m))
	r.h = tasks.HandlerWithService(svc, authz)
	r.db.Exec(`INSERT INTO tasks (id, project_id, title, description, created_by, status, pr_url) VALUES ('s1', 'p1', 'A', 'B', 'admin1', 'QA_PASSED', 'https://github.com/namth/agenthub/pull/15')`)
	if code, e := r.do("POST", "/api/tasks/s1/merge", "admin1:ADMIN", nil); code != 502 || e.Error.Code != "GITHUB_ERROR" {
		t.Fatalf("GitHub từ chối merge: %d %+v", code, e.Error)
	}
	if r.status("s1") != "QA_PASSED" {
		t.Fatal("merge lỗi giữ QA_PASSED")
	}
	r2 := newRig(t)
	r2.h = tasks.HandlerWithService(tasks.NewService(r2.db, r2.gh, r2.disp, "p1"), authz)
	r2.db.Exec(`INSERT INTO tasks (id, project_id, title, description, created_by, status, pr_url) VALUES ('s1', 'p1', 'A', 'B', 'admin1', 'QA_PASSED', 'https://x/pull/1')`)
	if code, e := r2.do("POST", "/api/tasks/s1/merge", "admin1:ADMIN", nil); code != 501 || e.Error.Code != "NOT_CONFIGURED" {
		t.Fatalf("không cấu hình merger: %d %+v, muốn 501", code, e.Error)
	}
}

func TestWebhookForwardsCIAndMerge(t *testing.T) {
	r := newRig(t)
	var ci, merged []string
	wh := tasks.WebhookHandler(r.db, secret, "p1",
		tasks.OnWorkflowRun(func(branch, conclusion, url string) { ci = append(ci, branch+"|"+conclusion+"|"+url) }),
		tasks.OnPullRequestMerged(func(prURL string) { merged = append(merged, prURL) }))
	post := func(event string, payload any) {
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest("POST", "/api/webhooks/github", bytes.NewReader(body))
		req.Header.Set("X-GitHub-Event", event)
		req.Header.Set("X-Hub-Signature-256", sign(body))
		wh.ServeHTTP(httptest.NewRecorder(), req)
	}
	post("workflow_run", map[string]any{"action": "completed", "workflow_run": map[string]any{"head_branch": "feat/task-201", "conclusion": "success", "html_url": "https://ci/1"}})
	post("workflow_run", map[string]any{"action": "requested", "workflow_run": map[string]any{"head_branch": "feat/task-201", "conclusion": nil, "html_url": "https://ci/2"}})
	post("pull_request", map[string]any{"action": "closed", "pull_request": map[string]any{"merged": true, "html_url": "https://github.com/namth/agenthub/pull/15"}})
	post("pull_request", map[string]any{"action": "closed", "pull_request": map[string]any{"merged": false, "html_url": "https://github.com/namth/agenthub/pull/16"}})
	if len(ci) != 1 || ci[0] != "feat/task-201|success|https://ci/1" {
		t.Fatalf("workflow_run chỉ chuyển khi completed: %v", ci)
	}
	if len(merged) != 1 || merged[0] != "https://github.com/namth/agenthub/pull/15" {
		t.Fatalf("pull_request chỉ chuyển khi closed + merged: %v", merged)
	}
	// issues vẫn đồng bộ như cũ khi có option.
	post("issues", issueEvent("opened", 700, "Từ GitHub"))
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE github_issue_id=700`).Scan(&n)
	if n != 1 {
		t.Fatal("issues opened vẫn phải đồng bộ")
	}
}

func TestGitHubMergerRequest(t *testing.T) {
	var method, path, auth string
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&body)
		w.Write([]byte(`{"merged":true}`))
	}))
	defer srv.Close()
	m := tasks.NewGitHubMerger(srv.URL, "namth", "agenthub", "pat", nil)
	if err := m.MergePR(context.Background(), 15); err != nil {
		t.Fatal(err)
	}
	if method != "PUT" || path != "/repos/namth/agenthub/pulls/15/merge" || auth != "Bearer pat" || body["merge_method"] != "squash" {
		t.Fatalf("request = %s %s %s %v (squash merge theo docs/09)", method, path, auth, body)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(405) }))
	defer bad.Close()
	if err := tasks.NewGitHubMerger(bad.URL, "o", "r", "t", nil).MergePR(context.Background(), 1); err == nil {
		t.Fatal("HTTP 405 phải trả lỗi")
	}
}
