package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// TaskBrief chứa thông tin tóm tắt của một task.
type TaskBrief struct {
	ID       string
	Title    string
	Status   string
	Project  string
	PRURL    string
	Subtasks []TaskBrief
}

// Commander thực hiện các lệnh quản trị từ Telegram.
type Commander interface {
	CreateTask(ctx context.Context, project, title, description string) (string, error)
	ListTasks(ctx context.Context, project string, limit int) ([]TaskBrief, error)
	TaskStatus(ctx context.Context, id string) (TaskBrief, error)
	Merge(ctx context.Context, id string) error
}

// ErrNotFound trả về khi không tìm thấy dự án hoặc task.
var ErrNotFound = errors.New("NOT_FOUND")

// Approver thực hiện phê duyệt hoặc từ chối task.
type Approver interface {
	Approve(ctx context.Context, taskID string) error
	Reject(ctx context.Context, taskID, reason string) error
}

type updatePayload struct {
	UpdateID      int64          `json:"update_id"`
	Message       *msgPayload    `json:"message"`
	CallbackQuery *callbackQuery `json:"callback_query"`
}

type callbackQuery struct {
	ID      string       `json:"id"`
	Data    string       `json:"data"`
	From    *userPayload `json:"from"`
	Message *msgPayload  `json:"message"`
}

type userPayload struct {
	ID int64 `json:"id"`
}

type msgPayload struct {
	MessageID int64        `json:"message_id"`
	Chat      *chatPayload `json:"chat"`
	From      *userPayload `json:"from"`
	Text      string       `json:"text"`
}

type chatPayload struct {
	ID int64 `json:"id"`
}

// WebhookHandler xử lý webhook callback từ Telegram Bot API với Approver.
func WebhookHandler(s Sender, a Approver, adminChatID, secret string) http.Handler {
	return WebhookHandlerWith(s, a, nil, adminChatID, secret)
}

// WebhookHandlerWith xử lý webhook từ Telegram với Sender, Approver, và Commander.
func WebhookHandlerWith(s Sender, a Approver, c Commander, adminChatID, secret string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSecret := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if subtle.ConstantTimeCompare([]byte(gotSecret), []byte(secret)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var upd updatePayload
		if err := json.NewDecoder(r.Body).Decode(&upd); err != nil {
			w.WriteHeader(http.StatusOK)
			return
		}

		if cb := upd.CallbackQuery; cb != nil {
			var chatID string
			if cb.Message != nil && cb.Message.Chat != nil {
				chatID = strconv.FormatInt(cb.Message.Chat.ID, 10)
			} else if cb.From != nil {
				chatID = strconv.FormatInt(cb.From.ID, 10)
			}

			if chatID != adminChatID {
				w.WriteHeader(http.StatusOK)
				return
			}

			var action, taskID string
			if strings.HasPrefix(cb.Data, "task_approve:") {
				action = "approve"
				taskID = strings.TrimPrefix(cb.Data, "task_approve:")
			} else if strings.HasPrefix(cb.Data, "task_reject:") {
				action = "reject"
				taskID = strings.TrimPrefix(cb.Data, "task_reject:")
			} else if strings.HasPrefix(cb.Data, "task_merge:") {
				action = "merge"
				taskID = strings.TrimPrefix(cb.Data, "task_merge:")
			}

			if taskID == "" || action == "" {
				w.WriteHeader(http.StatusOK)
				return
			}

			ctx := r.Context()
			switch action {
			case "approve":
				if a != nil {
					err := a.Approve(ctx, taskID)
					if err != nil {
						_ = s.AnswerCallback(ctx, cb.ID, fmt.Sprintf("Không thực hiện được: %v", err))
					} else {
						_ = s.AnswerCallback(ctx, cb.ID, "Đã duyệt")
					}
				}
			case "reject":
				if a != nil {
					err := a.Reject(ctx, taskID, "Từ chối qua Telegram")
					if err != nil {
						_ = s.AnswerCallback(ctx, cb.ID, fmt.Sprintf("Không thực hiện được: %v", err))
					} else {
						_ = s.AnswerCallback(ctx, cb.ID, "Đã từ chối")
					}
				}
			case "merge":
				if c != nil {
					err := c.Merge(ctx, taskID)
					if err != nil {
						_ = s.AnswerCallback(ctx, cb.ID, fmt.Sprintf("Không thực hiện được: %v", err))
					} else {
						_ = s.AnswerCallback(ctx, cb.ID, "Đã merge")
					}
				}
			}

			w.WriteHeader(http.StatusOK)
			return
		}

		if msg := upd.Message; msg != nil {
			var chatID string
			if msg.Chat != nil {
				chatID = strconv.FormatInt(msg.Chat.ID, 10)
			} else if msg.From != nil {
				chatID = strconv.FormatInt(msg.From.ID, 10)
			}

			if chatID != adminChatID {
				w.WriteHeader(http.StatusOK)
				return
			}

			text := strings.TrimSpace(msg.Text)
			if text == "" {
				w.WriteHeader(http.StatusOK)
				return
			}

			ctx := r.Context()
			if text == "/new" || strings.HasPrefix(text, "/new ") {
				raw := strings.TrimSpace(strings.TrimPrefix(text, "/new"))
				parts := strings.SplitN(raw, "|", 3)
				var project, title, desc string
				if len(parts) == 3 {
					project = strings.TrimSpace(parts[0])
					title = strings.TrimSpace(parts[1])
					desc = strings.TrimSpace(parts[2])
				}

				titleRunes := []rune(title)
				descRunes := []rune(desc)
				if len(parts) < 3 || project == "" || len(titleRunes) < 6 || len(titleRunes) > 120 || len(descRunes) < 20 {
					_ = s.SendMessage(ctx, chatID, "Cú pháp: /new <dự án> | <tiêu đề> | <mô tả>", nil)
					w.WriteHeader(http.StatusOK)
					return
				}

				if c != nil {
					taskID, err := c.CreateTask(ctx, project, title, desc)
					if err != nil {
						if errors.Is(err, ErrNotFound) {
							_ = s.SendMessage(ctx, chatID, fmt.Sprintf("Không tìm thấy dự án %s", project), nil)
						} else {
							_ = s.SendMessage(ctx, chatID, fmt.Sprintf("Không thực hiện được: %v", err), nil)
						}
					} else {
						_ = s.SendMessage(ctx, chatID, fmt.Sprintf("✅ Đã tạo task %s: %s", taskID, title), nil)
					}
				}
			} else if text == "/tasks" || strings.HasPrefix(text, "/tasks ") {
				project := strings.TrimSpace(strings.TrimPrefix(text, "/tasks"))
				if c != nil {
					taskList, err := c.ListTasks(ctx, project, 10)
					if err != nil {
						_ = s.SendMessage(ctx, chatID, fmt.Sprintf("Không thực hiện được: %v", err), nil)
					} else if len(taskList) == 0 {
						_ = s.SendMessage(ctx, chatID, "Không có task nào", nil)
					} else {
						lines := make([]string, 0, len(taskList))
						for _, t := range taskList {
							lines = append(lines, fmt.Sprintf("%s · %s · %s", t.ID, t.Status, t.Title))
						}
						_ = s.SendMessage(ctx, chatID, strings.Join(lines, "\n"), nil)
					}
				}
			} else if text == "/status" || strings.HasPrefix(text, "/status ") {
				id := strings.TrimSpace(strings.TrimPrefix(text, "/status"))
				if id == "" {
					_ = s.SendMessage(ctx, chatID, "Cú pháp: /status <id>", nil)
					w.WriteHeader(http.StatusOK)
					return
				}
				if c != nil {
					task, err := c.TaskStatus(ctx, id)
					if err != nil {
						if errors.Is(err, ErrNotFound) {
							_ = s.SendMessage(ctx, chatID, fmt.Sprintf("Không tìm thấy task %s", id), nil)
						} else {
							_ = s.SendMessage(ctx, chatID, fmt.Sprintf("Không thực hiện được: %v", err), nil)
						}
					} else {
						var sb strings.Builder
						sb.WriteString(fmt.Sprintf("%s · %s\nTrạng thái: %s", task.ID, task.Title, task.Status))
						if task.PRURL != "" {
							sb.WriteString(fmt.Sprintf("\nPR: %s", task.PRURL))
						}
						if len(task.Subtasks) > 0 {
							sb.WriteString("\nSub-tasks:")
							for _, st := range task.Subtasks {
								sb.WriteString(fmt.Sprintf("\n- %s · %s · %s", st.ID, st.Status, st.Title))
							}
						}
						var buttons [][]Button
						if task.Status == "QA_PASSED" && task.PRURL != "" {
							buttons = [][]Button{
								{
									{Text: "🔀 Merge", CallbackData: "task_merge:" + task.ID},
								},
							}
						}
						_ = s.SendMessage(ctx, chatID, sb.String(), buttons)
					}
				}
			} else if strings.HasPrefix(text, "/") {
				help := "Các lệnh hỗ trợ:\n" +
					"- /new <dự án> | <tiêu đề> | <mô tả>\n" +
					"- /tasks [dự án]\n" +
					"- /status <id>"
				_ = s.SendMessage(ctx, chatID, help, nil)
			}

			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusOK)
	})
}
