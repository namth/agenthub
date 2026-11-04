package telegram

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/namth/agenthub/internal/dispatcher"
)

type notifyItem struct {
	chatID  string
	text    string
	buttons [][]Button
}

type revokedLead struct {
	agentID string
	reason  string
}

// Notifier gửi thông báo tới quản trị viên qua Telegram.
type Notifier struct {
	sender      Sender
	adminChatID string
	loc         *time.Location

	queue chan notifyItem
	done  chan struct{}
	wg    sync.WaitGroup
	once  sync.Once

	mu          sync.Mutex
	lastRevoked *revokedLead
}

// NewNotifier khởi tạo Notifier với hàng đợi bất đồng bộ có giới hạn.
// Khi hàng đợi đầy, tin mới sẽ bị bỏ qua và không làm chặn luồng gọi.
func NewNotifier(s Sender, adminChatID string, loc *time.Location) *Notifier {
	if loc == nil {
		loc = time.UTC
	}
	n := &Notifier{
		sender:      s,
		adminChatID: adminChatID,
		loc:         loc,
		queue:       make(chan notifyItem, 256),
		done:        make(chan struct{}),
	}

	n.wg.Add(1)
	go n.worker()

	return n
}

func (n *Notifier) worker() {
	defer n.wg.Done()
	for {
		select {
		case <-n.done:
			return
		case item := <-n.queue:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = n.sender.SendMessage(ctx, item.chatID, item.text, item.buttons)
			cancel()
		}
	}
}

func (n *Notifier) enqueue(item notifyItem) {
	select {
	case <-n.done:
		return
	case n.queue <- item:
	default:
		// Hàng đợi đầy: bỏ tin để tránh nghẽn luồng gọi
	}
}

// Close dừng goroutine gửi thông báo.
func (n *Notifier) Close() {
	n.once.Do(func() {
		close(n.done)
		n.wg.Wait()
	})
}

// Notify nhận sự kiện dispatcher và gửi thông báo tương ứng.
func (n *Notifier) Notify(e dispatcher.Event) {
	switch e.Type {
	case dispatcher.EvLeaseRevoked:
		n.mu.Lock()
		n.lastRevoked = &revokedLead{
			agentID: e.AgentID,
			reason:  e.Reason,
		}
		n.mu.Unlock()

	case dispatcher.EvLeaseGranted:
		n.mu.Lock()
		prev := n.lastRevoked
		n.lastRevoked = nil
		n.mu.Unlock()

		if prev != nil {
			reasonStr := formatRevokeReason(prev.reason)
			text := fmt.Sprintf("👑 ĐỔI TRƯỞNG PHÒNG (epoch %d)\n%s ➡️ %s · Lý do: %s",
				e.Epoch, prev.agentID, e.AgentID, reasonStr)
			n.enqueue(notifyItem{
				chatID: n.adminChatID,
				text:   text,
			})
		}

	case dispatcher.EvLeadVacant:
		var eta string
		if e.Until != nil && !e.Until.IsZero() {
			eta = e.Until.In(n.loc).Format("15:04")
		} else {
			eta = "chưa rõ"
		}
		text := fmt.Sprintf("⚠️ CHƯA CÓ TRƯỞNG PHÒNG\nViệc code vẫn chạy; việc lập kế hoạch/review đang chờ.\nDự kiến có trưởng phòng lại lúc %s.", eta)
		n.enqueue(notifyItem{
			chatID: n.adminChatID,
			text:   text,
		})

	case dispatcher.EvLeadRestored:
		n.enqueue(notifyItem{
			chatID: n.adminChatID,
			text:   "✅ Đã có trưởng phòng trở lại",
		})

	case dispatcher.EvJobFailed:
		text := fmt.Sprintf("🔁 TASK %s THẤT BẠI NHIỀU LẦN\nĐã chuyển về lập lại kế hoạch (REPLAN)", e.TaskID)
		n.enqueue(notifyItem{
			chatID: n.adminChatID,
			text:   text,
		})

	default:
		// Các sự kiện khác bỏ qua theo đặc tả
	}
}

func formatRevokeReason(reason string) string {
	switch reason {
	case dispatcher.RevokeExhausted:
		return "hết limit"
	case dispatcher.RevokeExpired:
		return "hết hạn lease"
	case dispatcher.RevokeOffline:
		return "mất kết nối"
	case dispatcher.RevokeAdmin:
		return "Admin thu hồi"
	case dispatcher.RevokeReleased:
		return "tự nhường vai"
	default:
		return reason
	}
}

// NotifyNewTask gửi thông báo yêu cầu tính năng mới kèm nút duyệt nhanh.
func (n *Notifier) NotifyNewTask(taskID string, issue int, title, requester, description string) {
	descRunes := []rune(description)
	desc := description
	if len(descRunes) > 300 {
		desc = string(descRunes[:300]) + "…"
	}

	text := fmt.Sprintf("🔔 YÊU CẦU TÍNH NĂNG MỚI #%d\nNgười yêu cầu: @%s\nTiêu đề: %s\nMô tả: %s",
		issue, requester, title, desc)

	buttons := [][]Button{
		{
			{Text: "✅ Phê duyệt & Lập kế hoạch", CallbackData: "task_approve:" + taskID},
			{Text: "❌ Từ chối", CallbackData: "task_reject:" + taskID},
		},
	}

	n.enqueue(notifyItem{
		chatID:  n.adminChatID,
		text:    text,
		buttons: buttons,
	})
}
