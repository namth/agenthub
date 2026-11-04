package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Button biểu diễn một nút inline trong tin nhắn Telegram.
type Button struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

// Sender là interface giao tiếp gửi tin và phản hồi callback tới Telegram.
type Sender interface {
	SendMessage(ctx context.Context, chatID, text string, buttons [][]Button) error
	AnswerCallback(ctx context.Context, callbackID, text string) error
}

type botAPI struct {
	token      string
	apiBaseURL string
	client     *http.Client
}

// NewBotAPI khởi tạo Sender giao tiếp với Telegram Bot API thật.
// apiURL rỗng sẽ mặc định dùng https://api.telegram.org.
func NewBotAPI(token, apiURL string, hc *http.Client) Sender {
	if apiURL == "" {
		apiURL = "https://api.telegram.org"
	}
	apiURL = strings.TrimRight(apiURL, "/")
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &botAPI{
		token:      token,
		apiBaseURL: apiURL,
		client:     hc,
	}
}

type inlineKeyboard struct {
	InlineKeyboard [][]Button `json:"inline_keyboard"`
}

type sendMessagePayload struct {
	ChatID      string          `json:"chat_id"`
	Text        string          `json:"text"`
	ReplyMarkup *inlineKeyboard `json:"reply_markup,omitempty"`
}

type answerCallbackPayload struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
}

type apiResponse struct {
	Ok          bool            `json:"ok"`
	Result      json.RawMessage `json:"result,omitempty"`
	Description string          `json:"description,omitempty"`
}

func (b *botAPI) postJSON(ctx context.Context, method string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/bot%s/%s", b.apiBaseURL, b.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		// Che bot token nếu có trong URL lỗi để tránh lọt vào log
		sanitizedErr := strings.ReplaceAll(err.Error(), b.token, "***")
		return errors.New(sanitizedErr)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	var apiResp apiResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("telegram %s failed with status %d", method, resp.StatusCode)
		}
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if !apiResp.Ok || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		desc := apiResp.Description
		if desc == "" {
			desc = fmt.Sprintf("status code %d", resp.StatusCode)
		}
		return fmt.Errorf("telegram %s error: %s", method, desc)
	}

	return nil
}

func (b *botAPI) SendMessage(ctx context.Context, chatID, text string, buttons [][]Button) error {
	p := sendMessagePayload{
		ChatID: chatID,
		Text:   text,
	}
	if len(buttons) > 0 {
		p.ReplyMarkup = &inlineKeyboard{InlineKeyboard: buttons}
	}
	return b.postJSON(ctx, "sendMessage", p)
}

func (b *botAPI) AnswerCallback(ctx context.Context, callbackID, text string) error {
	p := answerCallbackPayload{
		CallbackQueryID: callbackID,
		Text:            text,
	}
	return b.postJSON(ctx, "answerCallbackQuery", p)
}
