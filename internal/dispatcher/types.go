// Package dispatcher là bộ điều phối của AgentHub: hàng đợi job, trạng thái agent,
// lease trưởng phòng và fencing (FEAT-01, docs/features/lead-rotation.md).
//
// File này là hợp đồng chung do trưởng phòng viết: gateway và các package khác chỉ
// phụ thuộc vào interface API cùng các kiểu dưới đây, không phụ thuộc cài đặt.
package dispatcher

import (
	"errors"
	"time"
)

// Lỗi nghiệp vụ — gateway ánh xạ sang ERROR event (docs/07 §2.3), REST sang mã HTTP.
var (
	ErrStaleAssignment  = errors.New("STALE_ASSIGNMENT")   // BR-04
	ErrStaleEpoch       = errors.New("STALE_EPOCH")        // BR-03
	ErrAgentNotOwned    = errors.New("AGENT_NOT_OWNED")    // BR-05
	ErrNotLead          = errors.New("NOT_LEAD")           // agent không giữ lease
	ErrAgentNotEligible = errors.New("AGENT_NOT_ELIGIBLE") // BR-06 (Admin trao vai)
	ErrNoLead           = errors.New("NO_LEAD")            // Admin thu hồi khi VACANT
	ErrNotFound         = errors.New("NOT_FOUND")
	ErrInvalidAuth      = errors.New("VALIDATION_ERROR")
	ErrConflict         = errors.New("CONFLICT")
)

// Clock cho phép test dùng đồng hồ giả.
type Clock interface{ Now() time.Time }

// Config là các ngưỡng thời gian/lần thử (docs/10 mục LEAD ROTATION & FAILOVER).
type Config struct {
	LeaseTTL        time.Duration // LEAD_LEASE_TTL_SEC, mặc định 90s
	OfflineAfter    time.Duration // HEARTBEAT_OFFLINE_SEC, mặc định 45s
	ReclaimAfter    time.Duration // RECLAIM_AFTER_SEC, mặc định 120s
	MaxAttempts     int           // MAX_ATTEMPTS, mặc định 3
	LogIdleWarn     time.Duration // LOG_IDLE_WARN_MIN, mặc định 15m
	DefaultCooldown time.Duration // DEFAULT_COOLDOWN_MIN, mặc định 60m
	AdminRevokeBan  time.Duration // loại trừ agent vừa bị Admin thu hồi, mặc định 10m (BR-06)
}

func DefaultConfig() Config {
	return Config{
		LeaseTTL:        90 * time.Second,
		OfflineAfter:    45 * time.Second,
		ReclaimAfter:    120 * time.Second,
		MaxAttempts:     3,
		LogIdleWarn:     15 * time.Minute,
		DefaultCooldown: 60 * time.Minute,
		AdminRevokeBan:  10 * time.Minute,
	}
}

// Loại agent và loại job khớp CHECK trong schema.sql.
const (
	KindClaude      = "CLAUDE"
	KindAntigravity = "ANTIGRAVITY"

	JobSpec     = "SPEC"
	JobDesign   = "DESIGN"
	JobTestW    = "TEST_WRITE"
	JobCode     = "CODE"
	JobQAReview = "QA_REVIEW"
	JobReplan   = "REPLAN"
	JobUnblock  = "UNBLOCK"
)

// Lý do thu hồi lease (LEASE_REVOKED.reason).
const (
	RevokeExhausted = "EXHAUSTED"
	RevokeExpired   = "EXPIRED"
	RevokeOffline   = "OFFLINE"
	RevokeAdmin     = "ADMIN"
	RevokeReleased  = "RELEASED"
	RevokeAuth      = "AUTH"
)

const (
	AuthOK      = "OK"
	AuthNeeds   = "NEEDS_AUTH"
	AuthUnknown = "UNKNOWN"
)

type AgentHello struct {
	Kind       string
	Name       string
	AuthStatus string
	AuthDetail string
	EverOK     *bool
}

type Agent struct {
	ID            string // "<worker_id>/<kind thường>", vd "mac-mini/claude"
	Name          string
	WorkerID      string
	Kind          string
	Status        string // IDLE | BUSY | COOLDOWN
	CooldownUntil *time.Time
	AuthStatus    string
}

type NewJob struct {
	ProjectID     string
	TaskID        string
	Kind          string
	RequiredAgent string
	LeadOnly      bool
	Priority      int // 0 → mặc định 100
	BranchName    string
}

// Assignment là một lần giao job cho agent — nội dung của ASSIGN_TASK.
type Assignment struct {
	ID          string // assignment_id (mã fencing)
	JobID       string
	TaskID      string
	AgentID     string
	JobKind     string
	Attempt     int
	BranchName  string
	HandoffNote string // ghi chú của lần thử trước (nếu có)
}

type Lease struct {
	State     string // HELD | VACANT
	AgentID   string // rỗng khi VACANT
	Epoch     int64
	ExpiresAt *time.Time
}

// Event là sự kiện miền dispatcher phát ra cho gateway / Telegram / văn phòng ảo.
type Event struct {
	Type           string // xem hằng Ev* bên dưới
	At             time.Time
	WorkerID       string
	AgentID        string
	JobID          string
	TaskID         string
	AssignmentID   string
	Epoch          int64
	Reason         string
	Until          *time.Time // COOLDOWN tới / giờ dự kiến hết VACANT
	AuthStatus     string
	PrevAuthStatus string
	Detail         string
}

const (
	EvLeaseGranted  = "LEASE_GRANTED"  // AgentID = lead mới, Epoch mới, Until = expires_at
	EvLeaseRevoked  = "LEASE_REVOKED"  // AgentID = lead cũ, Epoch cũ, Reason
	EvLeadVacant    = "LEAD_VACANT"    // Until = MIN(cooldown_until) của agent CLAUDE nếu có
	EvLeadRestored  = "LEAD_RESTORED"  // thoát VACANT
	EvAgentCooldown = "AGENT_COOLDOWN" // Until, Reason = EXHAUSTED | SOFT_LIMIT
	EvAgentIdle     = "AGENT_IDLE"     // hết cooldown
	EvWorkerOffline = "WORKER_OFFLINE" // WorkerID
	EvWorkerOnline  = "WORKER_ONLINE"  // WorkerID
	EvJobReclaimed  = "JOB_RECLAIMED"  // AssignmentID cũ, JobID
	EvJobFailed     = "JOB_FAILED"     // vượt MaxAttempts → REPLAN
	EvAbortTask     = "ABORT_TASK"     // gateway gửi ABORT_TASK tới AgentID
	EvJobAvailable  = "JOB_AVAILABLE"  // có job mới có thể giao
	EvJobAssigned   = "JOB_ASSIGNED"   // AgentID, AssignmentID, JobID, TaskID, Reason = job kind (văn phòng ảo)
	EvJobSubmitted  = "JOB_SUBMITTED"  // AgentID, AssignmentID, JobID
	EvAgentAuth     = "AGENT_AUTH"
	EvWorkerAuth    = "WORKER_AUTH"
	EvAgentRemoved  = "AGENT_REMOVED" // WorkerID, AgentID
)

// Notifier nhận sự kiện; cài đặt không được chặn lâu (gateway tự đệm).
type Notifier interface{ Notify(Event) }

// AuthAPI là interface tuỳ chọn cho tính năng đăng nhập agent (FEAT-06).
type AuthAPI interface {
	ConnectAuth(workerID, ghAuth string, agents []AgentHello) ([]Agent, error) // ghAuth "" = giữ cũ
	SetAuth(workerID, agentID, status, detail string) error
	SetGhAuth(workerID, status, detail string) error
	AuthRequired(workerID, agentID, assignmentID, detail, handoffNote, wipCommit string) error
}

// API là toàn bộ thao tác dispatcher cung cấp. Mọi thời điểm lấy từ Clock.
type API interface {
	// Fleet
	Connect(workerID string, kinds []string) ([]Agent, error) // máy ONLINE, upsert agent theo kind
	Heartbeat(workerID string) error
	Disconnect(workerID string) error // WebSocket đóng: máy OFFLINE ngay, offline_since = now
	Tick() error                      // chạy mọi bộ hẹn giờ: offline, thu hồi, hết hạn lease, hết cooldown, log idle, hạn mềm

	// Jobs
	EnqueueJob(j NewJob) (string, error)
	RequestJob(workerID, agentID string) (*Assignment, error) // nil, nil nếu không có job phù hợp
	Log(workerID, assignmentID string) error                  // cập nhật last_log_at
	Submit(workerID, assignmentID string) error
	Exhausted(workerID, agentID, assignmentID string, resetsAt *time.Time, reason, handoffNote, wipCommit string) error

	// Lead
	Lead() (Lease, error)
	Renew(workerID, agentID string, epoch int64) (Lease, error)
	Release(workerID, agentID string, epoch int64) error
	LeadNote(workerID, agentID string, epoch int64, taskID, entry string) error
	AdminAssign(agentID string) (Lease, error)
	AdminRevoke() (Lease, error)
}
