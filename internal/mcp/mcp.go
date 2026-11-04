package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/namth/agenthub/internal/tokens"
)

type Backend interface {
	CreateTask(ctx context.Context, userID, project, title, description string) (map[string]any, error)
	ListTasks(ctx context.Context, userID, project, status string, limit int) ([]map[string]any, error)
	TaskStatus(ctx context.Context, userID, id string) (map[string]any, error)
	AnswerClarification(ctx context.Context, userID, id string, answers map[string]string) (map[string]any, error)
	ApproveTask(ctx context.Context, userID, id string) (map[string]any, error)
	MergeTask(ctx context.Context, userID, id string) (map[string]any, error)
	DecideProposal(ctx context.Context, userID, taskID, proposalID string, accept bool) (map[string]any, error)
	FleetStatus(ctx context.Context, userID string) (map[string]any, error)
}

type rateLimiter struct {
	mu    sync.Mutex
	calls map[string][]time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		calls: make(map[string][]time.Time),
	}
}

func (rl *rateLimiter) allow(token string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := now.Add(-time.Minute)
	list := rl.calls[token]
	valid := list[:0]
	for _, t := range list {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	if len(valid) >= 60 {
		rl.calls[token] = valid
		return false
	}
	rl.calls[token] = append(valid, now)
	return true
}

var reSecretPrefix = regexp.MustCompile(`(ghp_|gho_|sk-|ahp_)\S*`)

func scrubSecrets(s string) string {
	return reSecretPrefix.ReplaceAllString(s, "***")
}

type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

var allTools = []toolDefinition{
	{
		Name:        "answer_clarification",
		Description: "Trả lời câu hỏi làm rõ từ AgentHub",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "ID của task",
				},
				"answers": map[string]any{
					"type":        "object",
					"description": "Bảng câu trả lời theo câu hỏi",
				},
			},
			"required": []string{"id", "answers"},
		},
	},
	{
		Name:        "approve_task",
		Description: "Duyệt yêu cầu để đưa vào lập kế hoạch",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "ID của task",
				},
			},
			"required": []string{"id"},
		},
	},
	{
		Name:        "create_task",
		Description: "Tạo task mới trên AgentHub",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project": map[string]any{
					"type":        "string",
					"description": "Tên hoặc ID dự án",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "Tiêu đề task (6-120 ký tự)",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Mô tả chi tiết task (tối thiểu 20 ký tự)",
				},
			},
			"required": []string{"project", "title", "description"},
		},
	},
	{
		Name:        "decide_proposal",
		Description: "Xác nhận hoặc từ chối đề xuất huỷ từ trưởng phòng AI",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "ID của task",
				},
				"proposal_id": map[string]any{
					"type":        "string",
					"description": "ID của đề xuất",
				},
				"accept": map[string]any{
					"type":        "boolean",
					"description": "Đồng ý (true) hoặc từ chối (false)",
				},
			},
			"required": []string{"task_id", "proposal_id", "accept"},
		},
	},
	{
		Name:        "fleet_status",
		Description: "Xem trạng thái cụm máy và phân bổ công việc",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	},
	{
		Name:        "list_tasks",
		Description: "Danh sách task trên AgentHub kèm nhãn cần làm rõ",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project": map[string]any{
					"type":        "string",
					"description": "Lọc theo dự án",
				},
				"status": map[string]any{
					"type":        "string",
					"description": "Lọc theo trạng thái task",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Số lượng tối đa (tối đa 50)",
				},
			},
		},
	},
	{
		Name:        "merge_task",
		Description: "Merge Pull Request của task khi QA đã pass",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "ID của task",
				},
			},
			"required": []string{"id"},
		},
	},
	{
		Name:        "task_status",
		Description: "Xem chi tiết trạng thái, sub-task, PR và log của task",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{
					"type":        "string",
					"description": "ID của task",
				},
			},
			"required": []string{"id"},
		},
	},
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func writeRPC(w http.ResponseWriter, id any, result any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func writeRPCError(w http.ResponseWriter, id any, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: msg,
		},
	})
}

func Handler(tokensStore *tokens.Store, isOwner func(userID string) bool, b Backend, now func() time.Time) http.Handler {
	if now == nil {
		now = time.Now
	}
	rl := newRateLimiter()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		plainToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		if plainToken == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		userID, err := tokensStore.Authenticate(plainToken)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		if isOwner == nil || !isOwner(userID) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if !rl.allow(plainToken, now()) {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeRPCError(w, nil, -32700, "Parse error")
			return
		}

		switch req.Method {
		case "initialize":
			writeRPC(w, req.ID, map[string]any{
				"protocolVersion": "2025-06-18",
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "agenthub",
					"version": "0.1.0",
				},
			})

		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)

		case "tools/list":
			sorted := make([]toolDefinition, len(allTools))
			copy(sorted, allTools)
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].Name < sorted[j].Name
			})
			writeRPC(w, req.ID, map[string]any{
				"tools": sorted,
			})

		case "tools/call":
			handleToolCall(w, r.Context(), userID, b, req)

		default:
			writeRPCError(w, req.ID, -32601, "Method not found")
		}
	})
}

func handleToolCall(w http.ResponseWriter, ctx context.Context, userID string, b Backend, req rpcRequest) {
	var callParams struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &callParams)
	}

	toolName := callParams.Name
	args := callParams.Arguments
	if args == nil {
		args = make(map[string]any)
	}

	var res map[string]any
	var callErr error

	switch toolName {
	case "create_task":
		project, _ := args["project"].(string)
		title, _ := args["title"].(string)
		desc, _ := args["description"].(string)
		if project == "" || title == "" || desc == "" {
			writeRPCError(w, req.ID, -32602, "Missing required arguments: project, title, description")
			return
		}
		res, callErr = b.CreateTask(ctx, userID, project, title, desc)

	case "list_tasks":
		project, _ := args["project"].(string)
		status, _ := args["status"].(string)
		var limit int
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		} else if l, ok := args["limit"].(int); ok {
			limit = l
		}
		listRes, err := b.ListTasks(ctx, userID, project, status, limit)
		if err != nil {
			callErr = err
		} else {
			raw, _ := json.Marshal(listRes)
			text := scrubSecrets(string(raw))
			writeRPC(w, req.ID, map[string]any{
				"content": []any{
					map[string]any{
						"type": "text",
						"text": text,
					},
				},
			})
			return
		}

	case "task_status":
		id, _ := args["id"].(string)
		if id == "" {
			writeRPCError(w, req.ID, -32602, "Missing required argument: id")
			return
		}
		res, callErr = b.TaskStatus(ctx, userID, id)

	case "answer_clarification":
		id, _ := args["id"].(string)
		answersRaw, ok := args["answers"].(map[string]any)
		if id == "" || !ok {
			writeRPCError(w, req.ID, -32602, "Missing required arguments: id, answers")
			return
		}
		answers := make(map[string]string)
		for k, v := range answersRaw {
			answers[k] = fmt.Sprint(v)
		}
		res, callErr = b.AnswerClarification(ctx, userID, id, answers)

	case "approve_task":
		id, _ := args["id"].(string)
		if id == "" {
			writeRPCError(w, req.ID, -32602, "Missing required argument: id")
			return
		}
		res, callErr = b.ApproveTask(ctx, userID, id)

	case "merge_task":
		id, _ := args["id"].(string)
		if id == "" {
			writeRPCError(w, req.ID, -32602, "Missing required argument: id")
			return
		}
		res, callErr = b.MergeTask(ctx, userID, id)

	case "decide_proposal":
		taskID, _ := args["task_id"].(string)
		proposalID, _ := args["proposal_id"].(string)
		acceptVal, hasAccept := args["accept"]
		if taskID == "" || proposalID == "" || !hasAccept {
			writeRPCError(w, req.ID, -32602, "Missing required arguments: task_id, proposal_id, accept")
			return
		}
		accept, _ := acceptVal.(bool)
		res, callErr = b.DecideProposal(ctx, userID, taskID, proposalID, accept)

	case "fleet_status":
		res, callErr = b.FleetStatus(ctx, userID)

	default:
		writeRPCError(w, req.ID, -32602, fmt.Sprintf("Unknown tool: %s", toolName))
		return
	}

	if callErr != nil {
		writeRPC(w, req.ID, map[string]any{
			"content": []any{
				map[string]any{
					"type": "text",
					"text": callErr.Error(),
				},
			},
			"isError": true,
		})
		return
	}

	raw, _ := json.Marshal(res)
	text := scrubSecrets(string(raw))
	writeRPC(w, req.ID, map[string]any{
		"content": []any{
			map[string]any{
				"type": "text",
				"text": text,
			},
		},
	})
}
