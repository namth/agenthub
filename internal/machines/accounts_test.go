package machines_test

// Test nghiệm thu TASK-049 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-accounts.md §6 (DELETE /api/my/machines/{id}/agents/{name}, join máy mới ever_ok=0,
// /api/my/machines chỉ trả agent ever_ok và chưa xoá); TC-100 (phần chủ máy).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/namth/agenthub/internal/database"
	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/machines"
)

type acctRig struct {
	*rig
	calls []string
	fail  error
}

func newAcctRig(t *testing.T) *acctRig {
	t.Helper()
	base := newRig(t) // dùng lại DB + dữ liệu người dùng của rig gốc
	a := &acctRig{rig: base}
	db, err := database.Open(filepath.Join(t.TempDir(), "acct.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, q := range []string{
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-owner', '1', 'namth', 'ADMIN')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-a', '2', 'doi-tac-a', 'PARTNER')`,
		`INSERT INTO users (id, github_id, username, role) VALUES ('u-b', '3', 'doi-tac-b', 'PARTNER')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	var url string
	h := machines.Handler(db, machines.Config{
		BaseURL:    func() string { return url },
		RunnersDir: base.runners,
		Now:        func() time.Time { return time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC) },
		IsOwner:    func(userID string) bool { return userID == "u-owner" },
		IsMember:   func(userID, projectID string) bool { return true },
		RemoveAgent: func(agentID string) error {
			a.calls = append(a.calls, agentID)
			if a.fail != nil {
				return a.fail
			}
			_, err := db.Exec(`UPDATE agents SET removed_at = CURRENT_TIMESTAMP WHERE id = ?`, agentID)
			return err
		},
	}, func(req *http.Request) (string, bool) {
		u := req.Header.Get("X-Test-User")
		return u, u != ""
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	url = srv.URL
	a.rig.srv = srv
	a.rig.db = db
	return a
}

func TestJoinNewMachineAgentsNotEverOK(t *testing.T) {
	r := newAcctRig(t)
	r.joinAs("u-a", "may-a")
	var n int
	r.db.QueryRow(`SELECT COUNT(*) FROM agents WHERE worker_id='may-a' AND ever_ok = 0`).Scan(&n)
	if n != 2 {
		t.Fatalf("join máy mới: agent phải ever_ok=0 cho tới HELLO đăng nhập OK, có %d/2", n)
	}
	_, e := r.do("GET", "/api/my/machines", "u-a", nil)
	var ms []struct {
		ID     string `json:"id"`
		Agents []struct {
			ID string `json:"id"`
		} `json:"agents"`
	}
	json.Unmarshal(e.Data, &ms)
	if len(ms) != 1 || len(ms[0].Agents) != 0 {
		t.Fatalf("máy mới chưa đăng nhập: danh sách agent phải rỗng, có %s", e.Data)
	}
}

func TestOwnerOfMachineDeletesAgent(t *testing.T) { // TC-100 (chủ máy)
	r := newAcctRig(t)
	r.joinAs("u-a", "may-a")
	r.joinAs("u-b", "may-b")
	r.db.Exec(`UPDATE agents SET ever_ok = 1`)
	r.db.Exec(`INSERT INTO agents (id, worker_id, kind, name, ever_ok) VALUES ('may-a/claude-2', 'may-a', 'CLAUDE', 'claude-2', 1)`)

	if code, _ := r.do("DELETE", "/api/my/machines/may-b/agents/claude", "u-a", nil); code != 403 {
		t.Fatalf("xoá agent máy người khác = %d, muốn 403", code)
	}
	if code, _ := r.do("DELETE", "/api/my/machines/may-a/agents/khong-co", "u-a", nil); code != 404 {
		t.Fatalf("agent không có = %d, muốn 404", code)
	}
	if len(r.calls) != 0 {
		t.Fatalf("chưa qua kiểm tra quyền/tồn tại thì không gọi RemoveAgent: %v", r.calls)
	}

	r.fail = dispatcher.ErrConflict
	if code, e := r.do("DELETE", "/api/my/machines/may-a/agents/claude-2", "u-a", nil); code != 409 || e.Error == nil || e.Error.Code != "CONFLICT" {
		t.Fatalf("RemoveAgent báo đang chạy việc → 409 CONFLICT, có %d %+v", code, e)
	}
	r.fail = nil

	code, e := r.do("DELETE", "/api/my/machines/may-a/agents/claude-2", "u-a", nil)
	if code != 200 || !e.Success {
		t.Fatalf("chủ máy xoá = %d %+v", code, e)
	}
	if last := r.calls[len(r.calls)-1]; last != "may-a/claude-2" {
		t.Fatalf("RemoveAgent nhận %q, muốn may-a/claude-2", last)
	}
	_, e = r.do("GET", "/api/my/machines", "u-a", nil)
	var ms []struct {
		Agents []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"agents"`
	}
	json.Unmarshal(e.Data, &ms)
	for _, ag := range ms[0].Agents {
		if ag.ID == "may-a/claude-2" {
			t.Fatal("agent đã xoá vẫn hiện ở Máy của tôi")
		}
		if ag.Name == "" {
			t.Fatalf("agent thiếu name: %s", e.Data)
		}
	}

	// Owner xoá được agent máy người khác ở đây.
	if code, _ := r.do("DELETE", "/api/my/machines/may-b/agents/claude", "u-owner", nil); code != 200 {
		t.Fatalf("owner xoá agent máy người khác = %d, muốn 200", code)
	}
}
