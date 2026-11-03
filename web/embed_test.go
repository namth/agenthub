package web_test

// Test nghiệm thu TASK-001 — do trưởng phòng viết, file bị khoá (tests.lock).

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/namth/agenthub/web"
)

func TestStaticHasIndexAtRoot(t *testing.T) {
	b, err := fs.ReadFile(web.Static(), "index.html")
	if err != nil {
		t.Fatalf("web.Static() phải chứa index.html ở gốc (không tiền tố dist/): %v", err)
	}
	if !strings.Contains(string(b), "AgentHub") {
		t.Fatalf("index.html phải có chữ AgentHub, nhận %q", string(b))
	}
}
