package tasks_test

// Test nghiệm thu TASK-039 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/clarification-and-shared-skills.md §2, §6, §7 (BR-49, BR-52 → BR-55); TC-77 → TC-79.

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/namth/agenthub/internal/tasks"
)

const qJSON = `[{"id":"payment_gateway","question":"Cổng nào?","options":["VNPay","MoMo"],"recommended":"VNPay","reason":"nội địa"},{"id":"refund","question":"Hoàn tiền?","options":[],"recommended":"","reason":""}]`

// seedClarify: task t-c do partner1 tạo, đang WAITING_USER, vòng 1 đã trả lời, vòng 2 đang chờ; partner2 là partner khác.
func seedClarify(r *rig) {
	r.t.Helper()
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('partner2', 'g3', 'doi-tac-2', 'PARTNER')`,
		`INSERT INTO tasks (id, project_id, github_issue_id, title, description, status, created_by) VALUES ('t-c', 'p1', 105, 'Tích hợp thanh toán', 'Khách muốn thanh toán online', 'WAITING_USER', 'partner1')`,
		`INSERT INTO task_clarifications (id, task_id, round, questions, answers, status, answered_by, answered_via, answered_at) VALUES ('c1', 't-c', 1, '[{"id":"who","question":"Ai trả?","options":["Lẻ"],"recommended":"Lẻ","reason":""}]', '{"who":"Lẻ"}', 'ANSWERED', 'partner1', 'GITHUB', '2026-11-04 10:00:00')`,
		`INSERT INTO task_clarifications (id, task_id, round, questions, status) VALUES ('c2', 't-c', 2, '` + qJSON + `', 'WAITING')`,
	} {
		if _, err := r.db.Exec(q); err != nil {
			r.t.Fatalf("%s: %v", q, err)
		}
	}
}

type view struct {
	Rounds []struct {
		Round       int               `json:"round"`
		Status      string            `json:"status"`
		Questions   []map[string]any  `json:"questions"`
		Answers     map[string]string `json:"answers"`
		AnsweredBy  string            `json:"answered_by"`
		AnsweredVia string            `json:"answered_via"`
	} `json:"rounds"`
	CanAnswer bool `json:"can_answer"`
	CanAuto   bool `json:"can_auto"`
}

func (r *rig) clarView(user string) (int, view) {
	r.t.Helper()
	code, e := r.do("GET", "/api/tasks/t-c/clarifications", user, nil)
	var v view
	json.Unmarshal(e.Data, &v)
	return code, v
}

func TestGetClarifications(t *testing.T) {
	r := newRig(t)
	seedClarify(r)
	code, v := r.clarView("partner1:PARTNER")
	if code != 200 || len(v.Rounds) != 2 {
		t.Fatalf("GET = %d %+v", code, v)
	}
	if v.Rounds[0].Round != 1 || v.Rounds[0].Status != "ANSWERED" || v.Rounds[0].AnsweredBy != "doi-tac" || v.Rounds[0].Answers["who"] != "Lẻ" {
		t.Fatalf("vòng 1 = %+v (answered_by phải là login)", v.Rounds[0])
	}
	if v.Rounds[1].Status != "WAITING" || len(v.Rounds[1].Questions) != 2 || v.Rounds[1].Questions[0]["recommended"] != "VNPay" {
		t.Fatalf("vòng 2 = %+v", v.Rounds[1])
	}
	if !v.CanAnswer || v.CanAuto {
		t.Fatalf("người tạo: can_answer=true, can_auto=false; có %v %v", v.CanAnswer, v.CanAuto)
	}
	if _, va := r.clarView("admin1:ADMIN"); !va.CanAnswer || !va.CanAuto {
		t.Fatalf("admin: can_answer & can_auto = %v %v", va.CanAnswer, va.CanAuto)
	}
	if _, vp := r.clarView("partner2:PARTNER"); vp.CanAnswer || vp.CanAuto {
		t.Fatalf("partner khác: chỉ xem; có %v %v", vp.CanAnswer, vp.CanAuto)
	}
}

func TestClarifyAnswerFlow(t *testing.T) { // TC-77, TC-78
	r := newRig(t)
	seedClarify(r)
	body := map[string]any{"answers": map[string]string{"payment_gateway": "MoMo", "refund": "Có", "_notes": "ưu tiên mobile", "la_gi": "bỏ qua"}}
	if code, e := r.do("POST", "/api/tasks/t-c/clarify", "partner2:PARTNER", body); code != 403 || e.Error.Code != "FORBIDDEN" {
		t.Fatalf("partner khác trả lời: %d %+v, muốn 403", code, e.Error)
	}
	if code, e := r.do("POST", "/api/tasks/t-c/clarify", "partner1:PARTNER", map[string]any{"answers": map[string]string{"payment_gateway": "MoMo"}}); code != 400 || e.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("thiếu câu: %d %+v, muốn 400", code, e.Error)
	}
	code, e := r.do("POST", "/api/tasks/t-c/clarify", "partner1:PARTNER", body)
	if code != 200 {
		t.Fatalf("trả lời = %d %+v", code, e.Error)
	}
	var d map[string]any
	json.Unmarshal(e.Data, &d)
	if d["status"] != "PLANNING" || r.status("t-c") != "PLANNING" {
		t.Fatalf("task phải về PLANNING: %v / %s", d, r.status("t-c"))
	}
	var st, by, via, raw string
	r.db.QueryRow(`SELECT status, answered_by, answered_via, answers FROM task_clarifications WHERE id='c2'`).Scan(&st, &by, &via, &raw)
	var ans map[string]string
	json.Unmarshal([]byte(raw), &ans)
	want := map[string]string{"payment_gateway": "MoMo", "refund": "Có", "_notes": "ưu tiên mobile"}
	if st != "ANSWERED" || by != "partner1" || via != "WEB" || !reflect.DeepEqual(ans, want) {
		t.Fatalf("vòng 2 = %s %s %s %v", st, by, via, ans)
	}
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE task_id='t-c' AND kind='SPEC' AND required_agent='CLAUDE' AND lead_only=0 AND status='QUEUED'`).Scan(&n)
	if n != 1 {
		t.Fatalf("phải có đúng 1 job SPEC mới (agent Claude bất kỳ — BR-52), có %d", n)
	}
	if code, e := r.do("POST", "/api/tasks/t-c/clarify", "admin1:ADMIN", body); code != 409 || e.Error.Code != "INVALID_TRANSITION" {
		t.Fatalf("trả lời lần 2: %d %+v, muốn 409 INVALID_TRANSITION", code, e.Error)
	}
}

func TestClarifyAuto(t *testing.T) {
	r := newRig(t)
	seedClarify(r)
	if code, _ := r.do("POST", "/api/tasks/t-c/clarify/auto", "partner1:PARTNER", nil); code != 403 {
		t.Fatalf("người tạo (partner) tự quyết: %d, muốn 403", code)
	}
	if code, e := r.do("POST", "/api/tasks/t-c/clarify/auto", "admin1:ADMIN", nil); code != 200 {
		t.Fatalf("admin tự quyết: %d %+v", code, e.Error)
	}
	var st, via, raw string
	r.db.QueryRow(`SELECT status, answered_via, answers FROM task_clarifications WHERE id='c2'`).Scan(&st, &via, &raw)
	var ans map[string]string
	json.Unmarshal([]byte(raw), &ans)
	if st != "AUTO_DECIDED" || via != "AUTO" || ans["payment_gateway"] != "VNPay" || ans["refund"] == "" {
		t.Fatalf("tự quyết = %s %s %v", st, via, ans)
	}
	if r.status("t-c") != "PLANNING" {
		t.Fatal("task phải về PLANNING")
	}
}

func TestKanbanWaitingUser(t *testing.T) {
	r := newRig(t)
	seedClarify(r)
	_, e := r.do("GET", "/api/tasks", "admin1:ADMIN", nil)
	var b struct {
		Columns []struct {
			Key   string           `json:"key"`
			Tasks []map[string]any `json:"tasks"`
		} `json:"columns"`
	}
	json.Unmarshal(e.Data, &b)
	for _, c := range b.Columns {
		for _, tk := range c.Tasks {
			if tk["id"] == "t-c" {
				if c.Key != "planning" || tk["clarify_pending"] != float64(2) {
					t.Fatalf("WAITING_USER phải ở cột planning, clarify_pending=2; có cột %s %v", c.Key, tk)
				}
				return
			}
		}
	}
	t.Fatalf("không thấy task WAITING_USER trên Kanban: %s", e.Data)
}

func TestParseAnswerComment(t *testing.T) {
	got, ok := tasks.ParseAnswerComment("Cảm ơn!\n/answer payment_gateway: VNPay\n/answer refund:  Có, trong 7 ngày \n/answer _notes: ưu tiên mobile\nhết")
	want := map[string]string{"payment_gateway": "VNPay", "refund": "Có, trong 7 ngày", "_notes": "ưu tiên mobile"}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse = %v %v", got, ok)
	}
	if _, ok := tasks.ParseAnswerComment("Không có lệnh nào"); ok {
		t.Fatal("không có /answer → ok=false")
	}
}

func TestAnswerByLoginAndHook(t *testing.T) { // TC-79
	r := newRig(t)
	seedClarify(r)
	var mu sync.Mutex
	var hooks []string
	svc := tasks.NewService(r.db, r.gh, r.disp, "p1", tasks.WithOnClarified(func(taskID string, round int, by, via string) {
		mu.Lock()
		hooks = append(hooks, taskID+"|"+by+"|"+via)
		mu.Unlock()
	}))
	ctx := context.Background()
	if err := svc.AnswerByLogin(ctx, "nguoi-la", "t-c", map[string]string{"payment_gateway": "VNPay", "refund": "Không"}); !errors.Is(err, tasks.ErrForbidden) {
		t.Fatalf("login lạ: %v, muốn ErrForbidden", err)
	}
	if err := svc.AnswerByLogin(ctx, "doi-tac", "t-c", map[string]string{"payment_gateway": "VNPay", "refund": "Không"}); err != nil {
		t.Fatal(err)
	}
	var via string
	r.db.QueryRow(`SELECT answered_via FROM task_clarifications WHERE id='c2'`).Scan(&via)
	mu.Lock()
	defer mu.Unlock()
	if via != "GITHUB" || len(hooks) != 1 || hooks[0] != "t-c|doi-tac|GITHUB" {
		t.Fatalf("via=%s hooks=%v", via, hooks)
	}
}

func TestPickOptionTelegram(t *testing.T) {
	r := newRig(t)
	seedClarify(r)
	r.db.Exec(`UPDATE task_clarifications SET questions = '[{"id":"payment_gateway","question":"Cổng nào?","options":["VNPay","MoMo"],"recommended":"VNPay","reason":""},{"id":"scope","question":"Phạm vi?","options":["Web","App"],"recommended":"Web","reason":""}]' WHERE id='c2'`)
	svc := tasks.NewService(r.db, r.gh, r.disp, "p1")
	ctx := context.Background()
	done, err := svc.PickOption(ctx, "admin1", "t-c", 2, "payment_gateway", 1)
	if err != nil || done {
		t.Fatalf("chọn câu 1: done=%v err=%v", done, err)
	}
	if r.status("t-c") != "WAITING_USER" {
		t.Fatal("chưa đủ câu thì vẫn WAITING_USER")
	}
	if _, err := svc.PickOption(ctx, "admin1", "t-c", 2, "scope", 9); !errors.Is(err, tasks.ErrValidation) {
		t.Fatalf("chỉ số phương án sai: %v", err)
	}
	done, err = svc.PickOption(ctx, "admin1", "t-c", 2, "scope", 0)
	if err != nil || !done {
		t.Fatalf("chọn câu 2: done=%v err=%v", done, err)
	}
	var st, via, raw string
	r.db.QueryRow(`SELECT status, answered_via, answers FROM task_clarifications WHERE id='c2'`).Scan(&st, &via, &raw)
	var ans map[string]string
	json.Unmarshal([]byte(raw), &ans)
	if st != "ANSWERED" || via != "TELEGRAM" || ans["payment_gateway"] != "MoMo" || ans["scope"] != "Web" || r.status("t-c") != "PLANNING" {
		t.Fatalf("telegram chốt vòng = %s %s %v task=%s", st, via, ans, r.status("t-c"))
	}
}

func TestIssueCommentWebhook(t *testing.T) {
	r := newRig(t)
	var mu sync.Mutex
	var got []string
	r.wh = tasks.WebhookHandler(r.db, secret, "p1", tasks.WithOnIssueComment(func(projectID string, issue int, login, body string) {
		mu.Lock()
		got = append(got, projectID+"|"+login+"|"+body)
		mu.Unlock()
		_ = issue
	}))
	payload := map[string]any{"action": "created", "issue": map[string]any{"number": 105, "title": "x"},
		"comment": map[string]any{"body": "/answer payment_gateway: VNPay", "user": map[string]any{"login": "doi-tac"}}}
	if code := r.hook("issue_comment", payload, ""); code != 200 {
		t.Fatalf("issue_comment = %d", code)
	}
	if code := r.hook("issue_comment", payload, "sha256=deadbeef"); code != 401 {
		t.Fatalf("chữ ký sai: %d", code)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0] != "p1|doi-tac|/answer payment_gateway: VNPay" {
		t.Fatalf("OnIssueComment = %v", got)
	}
}
