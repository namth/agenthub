package mcp_test

// Test nghiệm thu TASK-046 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/remote-command.md §6 (MCP tools), BR-63; TC-93.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/mcp"
	"github.com/namth/agenthub/internal/tokens"
)

type fakeBackend struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeBackend) rec(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }
func (f *fakeBackend) CreateTask(_ context.Context, u, p, title, d string) (map[string]any, error) {
	f.rec("create " + u + " " + p + " " + title)
	return map[string]any{"id": "t9", "status": "PENDING_APPROVAL"}, nil
}
func (f *fakeBackend) ListTasks(_ context.Context, u, p, st string, limit int) ([]map[string]any, error) {
	f.rec("list " + p + " " + st)
	return []map[string]any{{"id": "t1", "status": st}}, nil
}
func (f *fakeBackend) TaskStatus(_ context.Context, u, id string) (map[string]any, error) {
	if id == "khong-co" {
		return nil, errors.New("NOT_FOUND")
	}
	return map[string]any{"id": id, "status": "IN_PROGRESS"}, nil
}
func (f *fakeBackend) AnswerClarification(_ context.Context, u, id string, a map[string]string) (map[string]any, error) {
	f.rec("answer " + id + " " + a["gateway"])
	return map[string]any{"status": "PLANNING"}, nil
}
func (f *fakeBackend) ApproveTask(_ context.Context, u, id string) (map[string]any, error) {
	f.rec("approve " + id)
	return map[string]any{"status": "PLANNING"}, nil
}
func (f *fakeBackend) MergeTask(_ context.Context, u, id string) (map[string]any, error) {
	f.rec("merge " + id)
	return map[string]any{"status": "DONE"}, nil
}
func (f *fakeBackend) DecideProposal(_ context.Context, u, tid, pid string, accept bool) (map[string]any, error) {
	return map[string]any{"ok": accept}, nil
}
func (f *fakeBackend) FleetStatus(_ context.Context, u string) (map[string]any, error) {
	return map[string]any{"workers": []any{}}, nil
}

type rig struct {
	srv          *httptest.Server
	b            *fakeBackend
	owner, other string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	db.Exec(`INSERT INTO users (id, github_id, username, role) VALUES ('u-owner', '1', 'namth', 'ADMIN')`)
	db.Exec(`INSERT INTO users (id, github_id, username, role) VALUES ('u-p', '2', 'doi-tac', 'PARTNER')`)
	now := func() time.Time { return time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC) }
	st := tokens.NewStore(db, now)
	owner, _, err := st.Create("u-owner", "mcp", 30)
	if err != nil {
		t.Fatal(err)
	}
	// Token của partner tạo thẳng qua store (HTTP sẽ chặn) để kiểm /mcp từ chối.
	other, _, err := st.Create("u-p", "lau", 30)
	if err != nil {
		t.Fatal(err)
	}
	b := &fakeBackend{}
	h := mcp.Handler(st, func(u string) bool { return u == "u-owner" }, b, now)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &rig{srv: srv, b: b, owner: owner, other: other}
}

func (r *rig) rpc(t *testing.T, token string, body any) (int, map[string]any) {
	t.Helper()
	var raw []byte
	if s, ok := body.(string); ok {
		raw = []byte(s)
	} else {
		raw, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest("POST", r.srv.URL+"/mcp", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var m map[string]any
	json.NewDecoder(res.Body).Decode(&m)
	return res.StatusCode, m
}

func call(id int, name string, args map[string]any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}}
}

func TestAuth(t *testing.T) {
	r := newRig(t)
	init := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18"}}
	if code, _ := r.rpc(t, "", init); code != 401 {
		t.Fatalf("thiếu token: %d", code)
	}
	if code, _ := r.rpc(t, "ahp_sai", init); code != 401 {
		t.Fatalf("token sai: %d", code)
	}
	if code, _ := r.rpc(t, r.other, init); code != 403 {
		t.Fatalf("token partner: %d, muốn 403 (DEC-39)", code)
	}
	code, m := r.rpc(t, r.owner, init)
	res, _ := m["result"].(map[string]any)
	if code != 200 || res["protocolVersion"] == nil || res["serverInfo"].(map[string]any)["name"] != "agenthub" {
		t.Fatalf("initialize = %d %v", code, m)
	}
	if _, ok := res["capabilities"].(map[string]any)["tools"]; !ok {
		t.Fatalf("capabilities.tools thiếu: %v", res)
	}
	if code, _ := r.rpc(t, r.owner, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"}); code != 202 {
		t.Fatalf("notification: %d, muốn 202", code)
	}
}

func TestToolsList(t *testing.T) {
	r := newRig(t)
	_, m := r.rpc(t, r.owner, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	tools, _ := m["result"].(map[string]any)["tools"].([]any)
	names := []string{}
	for _, x := range tools {
		tm := x.(map[string]any)
		names = append(names, tm["name"].(string))
		if tm["description"] == "" || tm["inputSchema"] == nil {
			t.Fatalf("tool thiếu description/inputSchema: %v", tm)
		}
	}
	sort.Strings(names)
	want := []string{"answer_clarification", "approve_task", "create_task", "decide_proposal", "fleet_status", "list_tasks", "merge_task", "task_status"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tools = %v, muốn %v", names, want)
	}
}

func TestToolsCall(t *testing.T) {
	r := newRig(t)
	_, m := r.rpc(t, r.owner, call(3, "create_task", map[string]any{"project": "agenthub", "title": "Xuất báo cáo Excel", "description": "Khách cần xuất bảng tiến độ dạng Excel"}))
	res, _ := m["result"].(map[string]any)
	content, _ := res["content"].([]any)
	if len(content) == 0 || !strings.Contains(content[0].(map[string]any)["text"].(string), "t9") || res["isError"] == true {
		t.Fatalf("create_task = %v", m)
	}
	r.rpc(t, r.owner, call(4, "answer_clarification", map[string]any{"id": "t9", "answers": map[string]any{"gateway": "VNPay"}}))
	r.rpc(t, r.owner, call(5, "merge_task", map[string]any{"id": "t9"}))
	r.rpc(t, r.owner, call(6, "list_tasks", map[string]any{"project": "agenthub", "status": "WAITING_USER"}))
	r.b.mu.Lock()
	got := strings.Join(r.b.calls, "|")
	r.b.mu.Unlock()
	for _, want := range []string{"create u-owner agenthub Xuất báo cáo Excel", "answer t9 VNPay", "merge t9", "list agenthub WAITING_USER"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Backend thiếu lời gọi %q: %s", want, got)
		}
	}
	_, m = r.rpc(t, r.owner, call(7, "task_status", map[string]any{"id": "khong-co"}))
	if res, _ := m["result"].(map[string]any); res["isError"] != true {
		t.Fatalf("lỗi Backend phải isError=true: %v", m)
	}
	_, m = r.rpc(t, r.owner, call(8, "create_task", map[string]any{"title": "thiếu project"}))
	if e, _ := m["error"].(map[string]any); e == nil || e["code"].(float64) != -32602 {
		t.Fatalf("thiếu tham số → -32602: %v", m)
	}
	_, m = r.rpc(t, r.owner, call(9, "xoa_het", nil))
	if e, _ := m["error"].(map[string]any); e == nil || e["code"].(float64) != -32602 {
		t.Fatalf("tool lạ → -32602: %v", m)
	}
	_, m = r.rpc(t, r.owner, map[string]any{"jsonrpc": "2.0", "id": 10, "method": "la/gi"})
	if e, _ := m["error"].(map[string]any); e == nil || e["code"].(float64) != -32601 {
		t.Fatalf("phương thức lạ → -32601: %v", m)
	}
	_, m = r.rpc(t, r.owner, "{không phải json")
	if e, _ := m["error"].(map[string]any); e == nil || e["code"].(float64) != -32700 {
		t.Fatalf("JSON hỏng → -32700: %v", m)
	}
}

func TestRateLimit(t *testing.T) {
	r := newRig(t)
	body := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"}
	for i := 0; i < 60; i++ {
		if code, _ := r.rpc(t, r.owner, body); code != 200 {
			t.Fatalf("lời gọi %d: %d", i+1, code)
		}
	}
	if code, _ := r.rpc(t, r.owner, body); code != 429 {
		t.Fatalf("lời gọi 61 trong 1 phút: %d, muốn 429", code)
	}
}
