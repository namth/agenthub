package office_test

// Test nghiệm thu TASK-049 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/agent-accounts.md §6 (snapshot văn phòng chỉ có agent ever_ok và chưa xoá, kèm name); DEC-44.

import (
	"encoding/json"
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
	"github.com/namth/agenthub/internal/office"
)

func TestSnapshotOnlyRealAgents(t *testing.T) {
	db := seedUsers(t)
	for _, q := range []string{
		`INSERT INTO workers (id, name, os, token_hash, conn_status) VALUES ('m', 'm', 'darwin', 'h', 'ONLINE')`,
		`INSERT INTO workers (id, name, os, token_hash, conn_status) VALUES ('rong', 'rong', 'darwin', 'h2', 'ONLINE')`,
		`INSERT INTO agents (id, worker_id, kind, name, ever_ok) VALUES ('m/claude', 'm', 'CLAUDE', 'claude', 1)`,
		`INSERT INTO agents (id, worker_id, kind, name, ever_ok) VALUES ('m/claude-2', 'm', 'CLAUDE', 'claude-2', 1)`,
		`INSERT INTO agents (id, worker_id, kind, name, ever_ok, auth_status) VALUES ('m/antigravity', 'm', 'ANTIGRAVITY', 'antigravity', 0, 'NEEDS_AUTH')`,
		`INSERT INTO agents (id, worker_id, kind, name, ever_ok, removed_at) VALUES ('m/claude-3', 'm', 'CLAUDE', 'claude-3', 1, CURRENT_TIMESTAMP)`,
		`INSERT INTO agents (id, worker_id, kind, name, ever_ok) VALUES ('rong/claude', 'rong', 'CLAUDE', 'claude', 0)`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	snap, err := office.BuildSnapshot(db, dispatcher.Lease{State: "VACANT"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(snap)
	var s struct {
		Workers []struct {
			ID     string `json:"id"`
			Agents []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"agents"`
		} `json:"workers"`
	}
	json.Unmarshal(b, &s)
	got := map[string][]string{}
	for _, w := range s.Workers {
		for _, a := range w.Agents {
			got[w.ID] = append(got[w.ID], a.ID+"="+a.Name)
		}
	}
	if m := got["m"]; len(m) != 2 || m[0] != "m/claude=claude" || m[1] != "m/claude-2=claude-2" {
		t.Fatalf("máy m chỉ có 2 agent thật (kèm name), có %v", m)
	}
	if len(got["rong"]) != 0 {
		t.Fatalf("máy chưa có agent đăng nhập: không agent nào, có %v", got["rong"])
	}
}
