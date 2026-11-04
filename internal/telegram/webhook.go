package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Approver thực hiện phê duyệt hoặc từ chối task.
type Approver interface {
	Approve(ctx context.Context, taskID string) error
	Reject(ctx context.Context, taskID, reason string) error
}

type updatePayload struct {
	UpdateID      int64          `json:"update_id"`
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
	Chat *chatPayload `json:"chat"`
}

type chatPayload struct {
	ID int64 `json:"id"`
}

// WebhookHandler xử lý webhook callback từ Telegram Bot API.
func WebhookHandler(s Sender, a Approver, adminChatID, secret string) http.Handler {
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

		cb := upd.CallbackQuery
		if cb == nil {
			w.WriteHeader(http.StatusOK)
			return
		}

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
		}

		if taskID == "" || action == "" {
			w.WriteHeader(http.StatusOK)
			return
		}

		ctx := r.Context()
		if action == "approve" {
			err := a.Approve(ctx, taskID)
			if err != nil {
				_ = s.AnswerCallback(ctx, cb.ID, fmt.Sprintf("Không thực hiện được: %v", err))
			} else {
				_ = s.AnswerCallback(ctx, cb.ID, "Đã duyệt")
			}
		} else {
			err := a.Reject(ctx, taskID, "Từ chối qua Telegram")
			if err != nil {
				_ = s.AnswerCallback(ctx, cb.ID, fmt.Sprintf("Không thực hiện được: %v", err))
			} else {
				_ = s.AnswerCallback(ctx, cb.ID, "Đã từ chối")
			}
		}

		w.WriteHeader(http.StatusOK)
	})
}
