package office

import (
	"fmt"
	"sync"
	"time"

	"github.com/namth/agenthub/internal/dispatcher"
)

// Bus quản lý bộ đệm vòng các sự kiện văn phòng và phân phối qua channel cho các client SSE.
type Bus struct {
	mu          sync.RWMutex
	size        int
	now         func() time.Time
	loc         *time.Location
	nextID      int64
	events      []Event
	subs        map[int64]chan Event
	nextSubID   int64
	currentLead string
	revokedLead string
}

// NewBus khởi tạo Bus với kích thước bộ đệm size, hàm lấy giờ now và múi giờ loc.
func NewBus(size int, now func() time.Time, loc *time.Location) *Bus {
	if size <= 0 {
		size = 400
	}
	if now == nil {
		now = time.Now
	}
	if loc == nil {
		loc = time.Local
	}
	return &Bus{
		size: size,
		now:  now,
		loc:  loc,
		subs: make(map[int64]chan Event),
	}
}

// Notify thoả mãn interface dispatcher.Notifier, chuyển sự kiện miền thành sự kiện văn phòng.
// Hàm này không được chặn.
func (b *Bus) Notify(e dispatcher.Event) {
	switch e.Type {
	case dispatcher.EvLeaseGranted:
		b.mu.Lock()
		who := "system"
		if b.revokedLead != "" {
			who = b.revokedLead
			b.revokedLead = ""
		}
		b.currentLead = e.AgentID
		b.mu.Unlock()

		b.Emit(Event{
			Who:  who,
			Type: "handover",
			To:   e.AgentID,
			Text: fmt.Sprintf("epoch %d", e.Epoch),
		})

	case dispatcher.EvLeaseRevoked:
		b.mu.Lock()
		b.revokedLead = e.AgentID
		b.mu.Unlock()

	case dispatcher.EvJobAssigned:
		b.mu.RLock()
		who := b.currentLead
		b.mu.RUnlock()
		if who == "" {
			who = "system"
		}
		b.Emit(Event{
			Who:  who,
			Type: "assign",
			To:   e.AgentID,
			Task: e.TaskID,
		})

	case dispatcher.EvJobSubmitted:
		b.Emit(Event{
			Who:  e.AgentID,
			Type: "submit",
			Task: e.TaskID,
		})

	case dispatcher.EvAgentCooldown:
		text := ""
		if e.Until != nil {
			text = fmt.Sprintf("đến %s", e.Until.In(b.loc).Format("15:04"))
		}
		b.Emit(Event{
			Who:  e.AgentID,
			Type: "cooldown",
			Text: text,
		})

	case dispatcher.EvAgentIdle:
		b.Emit(Event{
			Who:  e.AgentID,
			Type: "idle",
		})

	case dispatcher.EvLeadVacant:
		b.mu.Lock()
		b.currentLead = ""
		b.revokedLead = ""
		b.mu.Unlock()

		text := "chưa rõ"
		if e.Until != nil {
			text = fmt.Sprintf("dự kiến %s", e.Until.In(b.loc).Format("15:04"))
		}
		b.Emit(Event{
			Who:  "system",
			Type: "vacant",
			Text: text,
		})

	case dispatcher.EvLeadRestored:
		// bỏ qua (LEASE_GRANTED đã vẽ)

	case dispatcher.EvWorkerOffline:
		b.Emit(Event{
			Who:  e.WorkerID,
			Type: "offline",
		})

	case dispatcher.EvWorkerOnline:
		b.Emit(Event{
			Who:  e.WorkerID,
			Type: "online",
		})

	case dispatcher.EvJobFailed:
		b.Emit(Event{
			Who:  "system",
			Type: "error",
			Task: e.TaskID,
		})

	default:
		// còn lại bỏ qua
	}
}

// Emit gán ID tăng dần từ 1, TS = now(), làm sạch Text, lưu vào bộ đệm vòng và phát cho người nghe.
func (b *Bus) Emit(e Event) {
	e.TS = b.now()
	e.Text = cleanText(e.Text)

	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	e.ID = b.nextID

	if len(b.events) < b.size {
		b.events = append(b.events, e)
	} else {
		copy(b.events, b.events[1:])
		b.events[len(b.events)-1] = e
	}

	for _, ch := range b.subs {
		select {
		case ch <- e:
		default:
			// người nghe chậm bị bỏ tin, không chặn Emit
		}
	}
}

// Since trả về các sự kiện có ID > id. ok=false nếu sự kiện id+1 đã rơi khỏi bộ đệm vòng.
func (b *Bus) Since(id int64) ([]Event, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if len(b.events) == 0 {
		return []Event{}, true
	}

	oldestID := b.events[0].ID
	if id+1 < oldestID {
		res := make([]Event, len(b.events))
		copy(res, b.events)
		return res, false
	}

	idx := len(b.events)
	for i, ev := range b.events {
		if ev.ID > id {
			idx = i
			break
		}
	}

	sub := b.events[idx:]
	res := make([]Event, len(sub))
	copy(res, sub)
	return res, true
}

// Subscribe trả về channel nhận sự kiện mới và hàm huỷ đăng ký.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 64)
	subID := b.nextSubID
	b.nextSubID++
	b.subs[subID] = ch

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, subID)
			close(ch)
			b.mu.Unlock()
		})
	}

	return ch, unsub
}
