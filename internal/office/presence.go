package office

import (
	"database/sql"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// UserView chứa thông tin người dùng hiển thị ở khu chỉ huy (FEAT-02, BR-44).
type UserView struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
	IsOwner   bool   `json:"is_owner"`
	Online    bool   `json:"online"`
}

// Presence theo dõi trạng thái online của người dùng và phát sự kiện presence (BR-44).
type Presence struct {
	db          *sql.DB
	now         func() time.Time
	bus         *Bus
	isOwner     func(userID string) bool
	mu          sync.Mutex
	lastTouch   map[string]time.Time
	lastSeen    map[string]time.Time
	openStreams map[string]int
	prevOnline  map[string]bool
}

// NewPresence tạo Presence mới.
func NewPresence(db *sql.DB, now func() time.Time, bus *Bus, isOwner func(userID string) bool) *Presence {
	if now == nil {
		now = time.Now
	}
	if isOwner == nil {
		isOwner = func(string) bool { return false }
	}
	return &Presence{
		db:          db,
		now:         now,
		bus:         bus,
		isOwner:     isOwner,
		lastTouch:   make(map[string]time.Time),
		lastSeen:    make(map[string]time.Time),
		openStreams: make(map[string]int),
		prevOnline:  make(map[string]bool),
	}
}

func parseDBTime(s string) (time.Time, bool) {
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// Touch ghi users.last_seen_at tối đa 1 lần / 60s / người (so trong RAM).
func (p *Presence) Touch(userID string) {
	if userID == "" {
		return
	}
	p.mu.Lock()
	now := p.now()
	last, ok := p.lastTouch[userID]
	if ok && now.Sub(last) < 60*time.Second {
		p.mu.Unlock()
		return
	}
	p.lastTouch[userID] = now
	p.lastSeen[userID] = now
	p.mu.Unlock()

	nowStr := now.UTC().Format("2006-01-02 15:04:05")
	_, _ = p.db.Exec(`UPDATE users SET last_seen_at = ? WHERE id = ?`, nowStr, userID)
}

// StreamOpened ghi nhận người dùng mở 1 kết nối SSE.
func (p *Presence) StreamOpened(userID string) {
	if userID == "" {
		return
	}
	p.mu.Lock()
	p.openStreams[userID]++
	p.mu.Unlock()
}

// StreamClosed ghi nhận người dùng đóng 1 kết nối SSE.
func (p *Presence) StreamClosed(userID string) {
	if userID == "" {
		return
	}
	p.mu.Lock()
	if p.openStreams[userID] > 0 {
		p.openStreams[userID]--
	}
	p.mu.Unlock()
}

// Online kiểm tra người dùng online: last_seen >= now - 5 phút hoặc đang mở >= 1 stream.
func (p *Presence) Online(userID string) bool {
	if userID == "" {
		return false
	}
	p.mu.Lock()
	streams := p.openStreams[userID]
	seen := p.lastSeen[userID]
	now := p.now()
	p.mu.Unlock()

	if streams > 0 {
		return true
	}
	cutoff := now.Add(-5 * time.Minute)
	if !seen.IsZero() {
		return !seen.Before(cutoff)
	}

	var s sql.NullString
	err := p.db.QueryRow(`SELECT last_seen_at FROM users WHERE id = ?`, userID).Scan(&s)
	if err == nil && s.Valid && s.String != "" {
		if t, ok := parseDBTime(s.String); ok {
			p.mu.Lock()
			if p.lastSeen[userID].Before(t) {
				p.lastSeen[userID] = t
			}
			p.mu.Unlock()
			return !t.Before(cutoff)
		}
	}
	return false
}

// Tick so trạng thái online với lần trước, phát Event{Who:"user:"+username, Type:"presence", Text:"online"|"offline"} khi đổi.
func (p *Presence) Tick() {
	rows, err := p.db.Query(`SELECT id, username, last_seen_at FROM users`)
	if err != nil {
		return
	}
	defer rows.Close()

	type uInfo struct {
		id         string
		username   string
		lastSeenAt sql.NullString
	}
	var users []uInfo
	for rows.Next() {
		var u uInfo
		if err := rows.Scan(&u.id, &u.username, &u.lastSeenAt); err == nil {
			users = append(users, u)
		}
	}

	p.mu.Lock()
	now := p.now()
	cutoff := now.Add(-5 * time.Minute)

	type presenceEvent struct {
		who  string
		text string
	}
	var evs []presenceEvent

	for _, u := range users {
		isOnline := false
		if p.openStreams[u.id] > 0 {
			isOnline = true
		} else {
			seen := p.lastSeen[u.id]
			if u.lastSeenAt.Valid && u.lastSeenAt.String != "" {
				if t, ok := parseDBTime(u.lastSeenAt.String); ok {
					if t.After(seen) {
						seen = t
						p.lastSeen[u.id] = t
					}
				}
			}
			if !seen.IsZero() && !seen.Before(cutoff) {
				isOnline = true
			}
		}

		prev, hadPrev := p.prevOnline[u.id]
		if !hadPrev {
			p.prevOnline[u.id] = isOnline
			if isOnline {
				evs = append(evs, presenceEvent{
					who:  "user:" + u.username,
					text: "online",
				})
			}
		} else if isOnline != prev {
			p.prevOnline[u.id] = isOnline
			text := "offline"
			if isOnline {
				text = "online"
			}
			evs = append(evs, presenceEvent{
				who:  "user:" + u.username,
				text: text,
			})
		}
	}
	p.mu.Unlock()

	if p.bus != nil {
		for _, e := range evs {
			p.bus.Emit(Event{
				Who:  e.who,
				Type: "presence",
				Text: e.text,
			})
		}
	}
}

// Users trả về danh sách người dùng theo thứ tự BR-44: owner -> partner online (theo username) -> partner vắng (theo username).
func (p *Presence) Users() ([]UserView, error) {
	rows, err := p.db.Query(`SELECT id, username, avatar_url, last_seen_at FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type uRow struct {
		id         string
		username   string
		avatarURL  string
		lastSeenAt sql.NullString
	}
	var uRows []uRow
	for rows.Next() {
		var ur uRow
		var av sql.NullString
		if err := rows.Scan(&ur.id, &ur.username, &av, &ur.lastSeenAt); err != nil {
			return nil, err
		}
		if av.Valid {
			ur.avatarURL = av.String
		}
		uRows = append(uRows, ur)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	p.mu.Lock()
	now := p.now()
	cutoff := now.Add(-5 * time.Minute)
	openStreamsCopy := make(map[string]int, len(p.openStreams))
	for k, v := range p.openStreams {
		openStreamsCopy[k] = v
	}
	lastSeenCopy := make(map[string]time.Time, len(p.lastSeen))
	for k, v := range p.lastSeen {
		lastSeenCopy[k] = v
	}
	p.mu.Unlock()

	res := make([]UserView, 0, len(uRows))
	for _, ur := range uRows {
		online := false
		if openStreamsCopy[ur.id] > 0 {
			online = true
		} else {
			var seen time.Time
			if t, ok := lastSeenCopy[ur.id]; ok {
				seen = t
			}
			if ur.lastSeenAt.Valid && ur.lastSeenAt.String != "" {
				if t, ok := parseDBTime(ur.lastSeenAt.String); ok {
					if t.After(seen) {
						seen = t
					}
				}
			}
			if !seen.IsZero() && !seen.Before(cutoff) {
				online = true
			}
		}

		res = append(res, UserView{
			Login:     ur.username,
			AvatarURL: ur.avatarURL,
			IsOwner:   p.isOwner(ur.id),
			Online:    online,
		})
	}

	sort.SliceStable(res, func(i, j int) bool {
		ui, uj := res[i], res[j]
		if ui.IsOwner != uj.IsOwner {
			return ui.IsOwner
		}
		if !ui.IsOwner {
			if ui.Online != uj.Online {
				return ui.Online
			}
		}
		return strings.ToLower(ui.Login) < strings.ToLower(uj.Login)
	})

	return res, nil
}

// Middleware bọc router, gọi Touch(userID) với mỗi request hợp lệ rồi chuyển tiếp.
func (p *Presence) Middleware(userOf func(*http.Request) (string, bool)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if userOf != nil {
				if uid, ok := userOf(r); ok && uid != "" {
					p.Touch(uid)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
