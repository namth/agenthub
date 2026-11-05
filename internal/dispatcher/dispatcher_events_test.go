package dispatcher_test

// Test bổ sung khi nghiệm thu TASK-002 (lỗ hổng của đặc tả, không phải lỗi agent).
// Do trưởng phòng viết, file bị khoá (tests.lock).

import (
	"testing"
	"time"

	"github.com/namth/agenthub/internal/dispatcher"
)

// Notifier gọi ngược vào API ngay trong Notify — phải không deadlock.
type callbackNotifier struct {
	svc  *dispatcher.Service
	seen chan string
}

func (c *callbackNotifier) Notify(e dispatcher.Event) {
	if c.svc != nil {
		_, _ = c.svc.Lead()
	}
	select {
	case c.seen <- e.Type:
	default:
	}
}

func TestNotifyCalledWithoutHoldingLock(t *testing.T) {
	base := newEnv(t)
	cb := &callbackNotifier{seen: make(chan string, 64)}
	svc := dispatcher.New(base.db, base.clk, base.cfg, cb)
	cb.svc = svc
	base.exec(`INSERT INTO workers (id, name, os, token_hash) VALUES ('a', 'a', 'darwin', 'h')`)

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.Connect("a", []string{dispatcher.KindClaude, dispatcher.KindAntigravity}) // LEASE_GRANTED
		_, _ = svc.EnqueueJob(dispatcher.NewJob{ProjectID: "p1", TaskID: "t1", Kind: "CODE", RequiredAgent: "ANTIGRAVITY"})
		_ = svc.Exhausted("a", "a/claude", "", nil, "limit", "", "") // LEASE_REVOKED, LEAD_VACANT, AGENT_COOLDOWN
		_ = svc.Tick()
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("deadlock: Notify đang được gọi khi Service còn giữ khoá")
	}
}

func TestRequeuedJobsAnnounceAvailability(t *testing.T) {
	e := newEnv(t)
	e.machine("L", dispatcher.KindClaude)
	e.machine("A", dispatcher.KindAntigravity)
	e.machine("B", dispatcher.KindAntigravity)
	e.machine("C", dispatcher.KindAntigravity)

	e.job("t1", dispatcher.JobCode, dispatcher.KindAntigravity)
	n := e.rec.count(dispatcher.EvJobAvailable)
	if n < 1 {
		t.Fatal("tạo job phải phát JOB_AVAILABLE")
	}

	// Hết limit → job về hàng đợi → phải báo.
	a1 := e.request("A", "A/antigravity")
	if err := e.svc.Exhausted("A", "A/antigravity", a1.ID, nil, "limit", "", ""); err != nil {
		t.Fatal(err)
	}
	if got := e.rec.count(dispatcher.EvJobAvailable); got <= n {
		t.Fatal("job trả về hàng đợi vì EXHAUSTED phải phát JOB_AVAILABLE")
	}
	n = e.rec.count(dispatcher.EvJobAvailable)

	// Mất kết nối quá lâu → thu hồi → phải báo.
	e.request("B", "B/antigravity")
	e.advance(180*time.Second, "L", "C")
	if got := e.rec.count(dispatcher.EvJobAvailable); got <= n {
		t.Fatal("job bị RECLAIMED phải phát JOB_AVAILABLE")
	}
	n = e.rec.count(dispatcher.EvJobAvailable)

	// Lần thử thứ 3 thất bại → sinh REPLAN → phải báo.
	a3 := e.request("C", "C/antigravity")
	if a3 == nil || a3.Attempt != 3 {
		t.Fatalf("lần 3: %+v", a3)
	}
	if err := e.svc.Exhausted("C", "C/antigravity", a3.ID, nil, "limit", "", ""); err != nil {
		t.Fatal(err)
	}
	if got := e.rec.count(dispatcher.EvJobAvailable); got <= n {
		t.Fatal("tạo job REPLAN phải phát JOB_AVAILABLE")
	}
	ev, _ := e.rec.last(dispatcher.EvJobAvailable)
	if ev.JobID == "" {
		t.Fatal("JOB_AVAILABLE phải mang JobID của job vừa sẵn sàng")
	}
}
