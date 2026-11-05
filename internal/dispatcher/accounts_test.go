package dispatcher_test

// Test nghiệm thu TASK-049 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-accounts.md §6, §7 (BR-66 → BR-70); TC-98, TC-99, TC-100 (phần dispatcher).

import (
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/dispatcher"
)

func yes() *bool { b := true; return &b }
func no() *bool  { b := false; return &b }

func ids(as []dispatcher.Agent) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, a.ID)
	}
	sort.Strings(out)
	return out
}

func (e *env) bareWorker(id string) {
	e.t.Helper()
	e.exec(`INSERT INTO workers (id, name, os, token_hash) VALUES (?, ?, 'darwin', 'h')`, id, id)
}

func TestHelloTwoClaudeAccounts(t *testing.T) { // TC-98
	e := newEnv(t)
	e.bareWorker("m")
	got, err := e.svc.ConnectAuth("m", "", []dispatcher.AgentHello{
		{Kind: dispatcher.KindClaude, AuthStatus: dispatcher.AuthOK, EverOK: yes()},
		{Kind: dispatcher.KindClaude, Name: "claude-2", AuthStatus: dispatcher.AuthOK, EverOK: yes()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if g := ids(got); len(g) != 2 || g[0] != "m/claude" || g[1] != "m/claude-2" {
		t.Fatalf("ConnectAuth = %v, muốn [m/claude m/claude-2]", g)
	}
	if n := e.str(`SELECT name FROM agents WHERE id='m/claude-2'`); n != "claude-2" {
		t.Fatalf("name = %q", n)
	}
	if n := e.num(`SELECT ever_ok FROM agents WHERE id='m/claude-2'`); n != 1 {
		t.Fatal("ever_ok phải = 1")
	}

	// Hai slot độc lập: mỗi tài khoản nhận một job.
	j1 := e.job("t1", dispatcher.JobCode, dispatcher.KindClaude)
	j2 := e.job("t2", dispatcher.JobCode, dispatcher.KindClaude)
	a1 := e.request("m", "m/claude")
	a2 := e.request("m", "m/claude-2")
	if a1 == nil || a2 == nil || a1.JobID == a2.JobID {
		t.Fatalf("hai tài khoản cùng máy phải nhận 2 job khác nhau: %+v %+v", a1, a2)
	}
	_ = j1
	_ = j2

	// COOLDOWN của claude không chặn claude-2 (BR-70).
	if err := e.svc.Submit("m", a1.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Submit("m", a2.ID); err != nil {
		t.Fatal(err)
	}
	until := e.clk.Now().Add(2 * time.Hour)
	if err := e.svc.Exhausted("m", "m/claude", "", &until, "EXHAUSTED", "", ""); err != nil {
		t.Fatal(err)
	}
	e.job("t3", dispatcher.JobCode, dispatcher.KindClaude)
	if a := e.request("m", "m/claude"); a != nil {
		t.Fatalf("agent COOLDOWN không được nhận việc: %+v", a)
	}
	if a := e.request("m", "m/claude-2"); a == nil {
		t.Fatal("claude-2 vẫn phải nhận việc khi claude đang COOLDOWN")
	}
}

func TestHelloDefaultNameKeepsLegacyID(t *testing.T) {
	e := newEnv(t)
	e.bareWorker("m")
	// Runner cũ (trước FEAT-11) không gửi name/ever_ok.
	if _, err := e.svc.ConnectAuth("m", "", []dispatcher.AgentHello{{Kind: dispatcher.KindAntigravity, AuthStatus: dispatcher.AuthOK}}); err != nil {
		t.Fatal(err)
	}
	if n := e.str(`SELECT name FROM agents WHERE id='m/antigravity'`); n != "antigravity" {
		t.Fatalf("name mặc định = kind viết thường, có %q", n)
	}
	if n := e.num(`SELECT ever_ok FROM agents WHERE id='m/antigravity'`); n != 1 {
		t.Fatal("runner cũ báo OK → ever_ok 1")
	}
}

func TestHelloRejectsBadName(t *testing.T) {
	e := newEnv(t)
	e.bareWorker("m")
	_, err := e.svc.ConnectAuth("m", "", []dispatcher.AgentHello{{Kind: dispatcher.KindClaude, Name: "../Hack", AuthStatus: dispatcher.AuthOK, EverOK: yes()}})
	if err == nil {
		if n := e.num(`SELECT COUNT(*) FROM agents WHERE worker_id='m'`); n != 0 {
			t.Fatal("name sai định dạng không được tạo agent")
		}
	}
}

func TestNeverOKHiddenAndNotDispatched(t *testing.T) { // TC-99 (phần ever_ok)
	e := newEnv(t)
	e.bareWorker("m")
	got, err := e.svc.ConnectAuth("m", "", []dispatcher.AgentHello{
		{Kind: dispatcher.KindClaude, AuthStatus: dispatcher.AuthNeeds, AuthDetail: "Not logged in", EverOK: no()},
		{Kind: dispatcher.KindAntigravity, AuthStatus: dispatcher.AuthOK, EverOK: yes()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if g := ids(got); len(g) != 1 || g[0] != "m/antigravity" {
		t.Fatalf("ConnectAuth chỉ trả agent ever_ok: %v", g)
	}
	e.job("t1", dispatcher.JobSpec, dispatcher.KindClaude)
	if a, _ := e.svc.RequestJob("m", "m/claude"); a != nil {
		t.Fatalf("agent chưa từng OK không được nhận việc: %+v", a)
	}

	// Sau khi đăng nhập OK lần đầu → ever_ok 1; mất phiên sau đó KHÔNG về 0 (BR-68).
	if _, err := e.svc.ConnectAuth("m", "", []dispatcher.AgentHello{{Kind: dispatcher.KindClaude, AuthStatus: dispatcher.AuthOK, EverOK: yes()}}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetAuth("m", "m/claude", dispatcher.AuthNeeds, "OAuth session expired"); err != nil {
		t.Fatal(err)
	}
	if n := e.num(`SELECT ever_ok FROM agents WHERE id='m/claude'`); n != 1 {
		t.Fatal("ever_ok không được về 0 khi mất phiên")
	}
}

func TestRemoveAgent(t *testing.T) { // TC-100 (phần dispatcher), TC-99 (tombstone)
	e := newEnv(t)
	e.bareWorker("m")
	if _, err := e.svc.ConnectAuth("m", "", []dispatcher.AgentHello{
		{Kind: dispatcher.KindClaude, AuthStatus: dispatcher.AuthOK, EverOK: yes()},
		{Kind: dispatcher.KindClaude, Name: "claude-2", AuthStatus: dispatcher.AuthOK, EverOK: yes()},
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.svc.RemoveAgent("m/khong-co"); !errors.Is(err, dispatcher.ErrNotFound) {
		t.Fatalf("xoá agent không tồn tại: %v, muốn ErrNotFound", err)
	}

	// Đang chạy việc → ErrConflict.
	e.job("t1", dispatcher.JobCode, dispatcher.KindClaude)
	a := e.request("m", "m/claude-2")
	if a == nil {
		t.Fatal("cần assignment")
	}
	if err := e.svc.RemoveAgent("m/claude-2"); !errors.Is(err, dispatcher.ErrConflict) {
		t.Fatalf("agent đang chạy việc: %v, muốn ErrConflict", err)
	}
	if err := e.svc.Submit("m", a.ID); err != nil {
		t.Fatal(err)
	}

	e.rec.mu.Lock()
	before := len(e.rec.evs)
	e.rec.mu.Unlock()
	if err := e.svc.RemoveAgent("m/claude-2"); err != nil {
		t.Fatalf("RemoveAgent: %v", err)
	}
	if e.str(`SELECT removed_at FROM agents WHERE id='m/claude-2'`) == "" {
		t.Fatal("phải đặt removed_at (tombstone)")
	}
	found := false
	e.rec.mu.Lock()
	evs := append([]dispatcher.Event(nil), e.rec.evs[before:]...)
	e.rec.mu.Unlock()
	for _, ev := range evs {
		if ev.Type == dispatcher.EvAgentRemoved && ev.AgentID == "m/claude-2" && ev.WorkerID == "m" {
			found = true
		}
	}
	if !found {
		t.Fatalf("thiếu sự kiện EvAgentRemoved: %+v", evs)
	}

	// HELLO sau đó còn báo claude-2 → không hồi sinh, và nằm trong danh sách cần AGENT_REMOVE.
	got, err := e.svc.ConnectAuth("m", "", []dispatcher.AgentHello{
		{Kind: dispatcher.KindClaude, AuthStatus: dispatcher.AuthOK, EverOK: yes()},
		{Kind: dispatcher.KindClaude, Name: "claude-2", AuthStatus: dispatcher.AuthOK, EverOK: yes()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if g := ids(got); len(g) != 1 || g[0] != "m/claude" {
		t.Fatalf("agent đã xoá bị hồi sinh: %v", g)
	}
	if e.str(`SELECT removed_at FROM agents WHERE id='m/claude-2'`) == "" {
		t.Fatal("HELLO không được xoá tombstone")
	}
	if tomb := e.svc.Tombstoned("m", []string{"m/claude", "m/claude-2"}); len(tomb) != 1 || tomb[0] != "m/claude-2" {
		t.Fatalf("Tombstoned = %v, muốn [m/claude-2]", tomb)
	}
	e.job("t2", dispatcher.JobCode, dispatcher.KindClaude)
	if a, _ := e.svc.RequestJob("m", "m/claude-2"); a != nil {
		t.Fatal("agent đã xoá không được nhận việc")
	}
}

func TestRemoveLeadReleasesLease(t *testing.T) {
	e := newEnv(t)
	e.bareWorker("m")
	if _, err := e.svc.ConnectAuth("m", "", []dispatcher.AgentHello{{Kind: dispatcher.KindClaude, AuthStatus: dispatcher.AuthOK, EverOK: yes()}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AdminAssign("m/claude"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RemoveAgent("m/claude"); err != nil {
		t.Fatalf("RemoveAgent lead: %v", err)
	}
	if l := e.lead(); l.AgentID == "m/claude" {
		t.Fatalf("lead đã xoá vẫn giữ lease: %+v", l)
	}
}
