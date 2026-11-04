package pipeline_test

// Test nghiệm thu TASK-038 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/clarification-and-shared-skills.md §6, §7 (BR-47, BR-48, BR-51, BR-53); TC-75, TC-76, TC-80.

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/pipeline"
)

type hookLog struct {
	mu   sync.Mutex
	need []string
	cap  []string
	rem  []string
}

// newHookedRig dựng lại pipeline với các hook FEAT-07 (cùng DB/dispatcher như newRig).
func newHookedRig(t *testing.T) (*rig, *hookLog) {
	t.Helper()
	r := newRig(t)
	h := &hookLog{}
	r.p = pipeline.New(r.db, r.disp, r.gh, "p1",
		pipeline.WithOnNeedInfo(func(taskID string, round int, qs []pipeline.ClarifyQuestion) {
			h.mu.Lock()
			ids := []string{}
			for _, q := range qs {
				ids = append(ids, q.ID)
			}
			h.need = append(h.need, taskID+"|"+strings.Join(ids, ","))
			h.mu.Unlock()
			_ = round
		}),
		pipeline.WithOnClarifyCapped(func(taskID string) { h.mu.Lock(); h.cap = append(h.cap, taskID); h.mu.Unlock() }),
		pipeline.WithOnClarifyReminder(func(taskID string, round, hours int) {
			h.mu.Lock()
			h.rem = append(h.rem, taskID+"|"+string(rune('0'+round))+"|"+map[int]string{24: "24", 72: "72"}[hours])
			h.mu.Unlock()
		}))
	return r, h
}

func needInfo(qs ...map[string]any) map[string]any {
	return map[string]any{"status": "NEED_INFO", "questions": qs}
}

var qGateway = map[string]any{"id": "payment_gateway", "question": "Cổng thanh toán nào?", "options": []string{"VNPay", "MoMo"}, "recommended": "VNPay", "reason": "nội địa"}
var qRefund = map[string]any{"id": "refund", "question": "Có hoàn tiền tự động?", "options": []string{}, "recommended": "", "reason": ""}

func spec(r *rig) {
	r.t.Helper()
	if _, err := r.disp.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "SPEC", RequiredAgent: "CLAUDE"}); err != nil {
		r.t.Fatal(err)
	}
}

func TestNeedInfoMovesToWaitingUser(t *testing.T) { // TC-75
	r, h := newHookedRig(t)
	spec(r)
	r.run("m/claude", "", needInfo(qGateway, qRefund))
	if s := r.str(`SELECT status FROM tasks WHERE id='t1'`); s != "WAITING_USER" {
		t.Fatalf("task = %s, muốn WAITING_USER", s)
	}
	var round int
	var st, agent, raw string
	r.db.QueryRow(`SELECT round, status, asked_by_agent, questions FROM task_clarifications WHERE task_id='t1'`).Scan(&round, &st, &agent, &raw)
	var qs []map[string]any
	json.Unmarshal([]byte(raw), &qs)
	if round != 1 || st != "WAITING" || agent != "m/claude" || len(qs) != 2 || qs[0]["recommended"] != "VNPay" {
		t.Fatalf("vòng = %d %s %s %v", round, st, agent, qs)
	}
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id='t1' AND status='QUEUED'`); n != 0 {
		t.Fatalf("WAITING_USER không được có job chờ, có %d", n)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.need) != 1 || h.need[0] != "t1|payment_gateway,refund" {
		t.Fatalf("OnNeedInfo = %v", h.need)
	}
	if n, _ := r.p.Reconcile(context.Background()); n != 0 {
		t.Fatalf("Reconcile không được sinh SPEC cho WAITING_USER, sinh %d", n)
	}
}

func TestNeedInfoInvalidIsReplan(t *testing.T) { // TC-76
	nine := []map[string]any{}
	for i := 0; i < 9; i++ {
		nine = append(nine, map[string]any{"id": "q" + string(rune('a'+i)) + "x", "question": "?", "options": []string{}})
	}
	cases := map[string]map[string]any{
		"9 câu":       needInfo(nine...),
		"id trùng":    needInfo(qGateway, qGateway),
		"id sai":      needInfo(map[string]any{"id": "Có Dấu", "question": "?", "options": []string{}}),
		"rỗng":        needInfo(),
		"recommended": needInfo(map[string]any{"id": "x1", "question": "?", "options": []string{"A"}, "recommended": "B"}),
		"7 phương án": needInfo(map[string]any{"id": "x2", "question": "?", "options": []string{"1", "2", "3", "4", "5", "6", "7"}}),
	}
	for name, res := range cases {
		t.Run(name, func(t *testing.T) {
			r, h := newHookedRig(t)
			spec(r)
			r.run("m/claude", "", res)
			if s := r.str(`SELECT status FROM tasks WHERE id='t1'`); s == "WAITING_USER" {
				t.Fatal("NEED_INFO sai schema không được chuyển WAITING_USER")
			}
			if n := r.num(`SELECT COUNT(*) FROM task_clarifications`); n != 0 {
				t.Fatal("không được lưu vòng câu hỏi sai schema")
			}
			if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id='t1' AND kind='REPLAN' AND lead_only=1`); n != 1 {
				t.Fatalf("sai schema → 1 job REPLAN, có %d", n)
			}
			if len(h.need) != 0 {
				t.Fatal("không gọi OnNeedInfo khi sai schema")
			}
		})
	}
}

func seedRounds(r *rig, n int) {
	r.t.Helper()
	for i := 1; i <= n; i++ {
		r.exec(`INSERT INTO task_clarifications (id, task_id, round, questions, answers, status, answered_via) VALUES (?, 't1', ?, ?, ?, 'ANSWERED', 'WEB')`,
			"c"+string(rune('0'+i)), i, `[{"id":"q`+string(rune('0'+i))+`","question":"Câu số `+string(rune('0'+i))+`?","options":["A","B"],"recommended":"A","reason":""}]`, `{"q`+string(rune('0'+i))+`":"B","_notes":"ghi chú `+string(rune('0'+i))+`"}`)
	}
}

func TestRoundCap(t *testing.T) { // BR-48
	r, h := newHookedRig(t)
	seedRounds(r, 3)
	spec(r)
	r.run("m/claude", "", needInfo(qGateway))
	if n := r.num(`SELECT COUNT(*) FROM task_clarifications WHERE round = 4`); n != 0 {
		t.Fatal("không được có vòng 4")
	}
	if n := r.num(`SELECT COUNT(*) FROM jobs WHERE task_id='t1' AND kind='REPLAN' AND lead_only=1`); n != 1 {
		t.Fatalf("vòng 4 → REPLAN cho lead, có %d", n)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.cap) != 1 || len(h.need) != 0 {
		t.Fatalf("cap=%v need=%v", h.cap, h.need)
	}
}

func TestPromptIncludesPreviousAnswers(t *testing.T) { // BR-51
	r, _ := newHookedRig(t)
	seedRounds(r, 2)
	spec(r)
	a, _ := r.disp.RequestJob("m", "m/claude")
	p := r.p.PromptFor(*a)
	data := p[strings.Index(p, "DỮ LIỆU THAM KHẢO"):]
	for _, want := range []string{"Câu số 1?", "Câu số 2?", "B", "ghi chú 2", "NEED_INFO"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt thiếu %q:\n%s", want, p)
		}
	}
	if !strings.Contains(data, "Câu số 1?") {
		t.Fatal("Hỏi–Đáp phải nằm trong khối DỮ LIỆU THAM KHẢO")
	}
	if strings.Contains(p, "Đã hết lượt hỏi") {
		t.Fatal("còn lượt hỏi thì không được báo hết lượt")
	}
	r2, _ := newHookedRig(t)
	seedRounds(r2, 3)
	spec(r2)
	a2, _ := r2.disp.RequestJob("m", "m/claude")
	if !strings.Contains(r2.p.PromptFor(*a2), "Đã hết lượt hỏi") {
		t.Fatal("đã 3 vòng → prompt phải báo hết lượt hỏi")
	}
}

func TestExplicitDoneStillCreatesSubtasks(t *testing.T) {
	r, _ := newHookedRig(t)
	spec(r)
	res := spec3()
	res["status"] = "DONE"
	r.run("m/claude", "", res)
	if s := r.str(`SELECT status FROM tasks WHERE id='t1'`); s != "READY" {
		t.Fatalf("status DONE + subtasks → READY, có %s", s)
	}
	if n := r.num(`SELECT COUNT(*) FROM tasks WHERE parent_id='t1'`); n != 3 {
		t.Fatalf("subtask = %d", n)
	}
}

func TestReminders(t *testing.T) { // BR-53, TC-80
	r, h := newHookedRig(t)
	created := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	r.exec(`UPDATE tasks SET status='WAITING_USER' WHERE id='t1'`)
	r.exec(`INSERT INTO task_clarifications (id, task_id, round, questions, status, created_at) VALUES ('w1', 't1', 1, '[]', 'WAITING', ?)`, created.Format("2006-01-02 15:04:05"))
	ctx := context.Background()
	steps := []struct {
		after time.Duration
		want  int
	}{{23 * time.Hour, 0}, {25 * time.Hour, 1}, {26 * time.Hour, 0}, {73 * time.Hour, 1}, {80 * time.Hour, 0}}
	for _, s := range steps {
		n, err := r.p.RemindClarifications(ctx, created.Add(s.after))
		if err != nil || n != s.want {
			t.Fatalf("sau %v: nhắc %d (%v), muốn %d", s.after, n, err, s.want)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if strings.Join(h.rem, ",") != "t1|1|24,t1|1|72" {
		t.Fatalf("OnClarifyReminder = %v", h.rem)
	}
	r.exec(`UPDATE task_clarifications SET status='ANSWERED' WHERE id='w1'`)
	if n, _ := r.p.RemindClarifications(ctx, created.Add(200*time.Hour)); n != 0 {
		t.Fatal("vòng đã trả lời không được nhắc")
	}
}
