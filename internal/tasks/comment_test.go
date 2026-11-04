package tasks_test

// Test nghiệm thu TASK-038 — do trưởng phòng viết, file bị khoá (tests.lock).
// Đặc tả: docs/features/clarification-and-shared-skills.md §3 (comment câu hỏi lên issue GitHub).

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/namth/agenthub/internal/tasks"
)

func TestCommentIssue(t *testing.T) {
	var path, auth, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.Method+" "+r.URL.Path, r.Header.Get("Authorization")
		var m map[string]string
		json.NewDecoder(r.Body).Decode(&m)
		body = m["body"]
		w.WriteHeader(201)
		w.Write([]byte(`{"id":1}`))
	}))
	defer srv.Close()
	gh := tasks.NewGitHubClient(srv.URL, "namth", "agenthub", "tok", nil)
	c, ok := gh.(tasks.Commenter)
	if !ok {
		t.Fatal("GitHub client phải cài tasks.Commenter")
	}
	if err := c.CommentIssue(context.Background(), 105, "Câu hỏi làm rõ"); err != nil {
		t.Fatal(err)
	}
	if path != "POST /repos/namth/agenthub/issues/105/comments" || auth != "Bearer tok" || body != "Câu hỏi làm rõ" {
		t.Fatalf("request = %s %s %q", path, auth, body)
	}
}
