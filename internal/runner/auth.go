package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

const (
	AuthOK      = "OK"
	AuthNeeds   = "NEEDS_AUTH"
	AuthUnknown = "UNKNOWN"
	AuthKindGH  = "GH"
)

func DefaultAuthChecks() map[string][]string {
	return map[string][]string{
		"CLAUDE": {"claude", "auth", "status"},
		"GH":     {"gh", "auth", "status"},
	}
}

func DefaultAuthPatterns() []string {
	return []string{
		"not logged in",
		"please run /login",
		"authentication required",
		"invalid api key",
		"gh auth login",
		"failed to authenticate",
		"oauth",
		"session expired",
		"token expired",
		"could not be refreshed",
	}
}

func DefaultAuthProbes() map[string][]string {
	return map[string][]string{
		"CLAUDE":      {"claude", "-p", "Trả lời đúng một từ: OK"},
		"ANTIGRAVITY": {"agy", "-p", "Trả lời đúng một từ: OK"},
	}
}

func ParseProbeResult(exit int, output string, err error) (status, detail string) {
	if err != nil {
		d := err.Error()
		if f := firstNonEmptyLine(output); f != "" {
			d = f
		}
		return AuthUnknown, sanitizeDetail(d)
	}

	outLower := strings.ToLower(output)
	for _, p := range DefaultAuthPatterns() {
		if strings.Contains(outLower, strings.ToLower(p)) {
			matchedLine := ""
			for _, line := range strings.Split(output, "\n") {
				if strings.Contains(strings.ToLower(line), strings.ToLower(p)) {
					matchedLine = strings.TrimSpace(line)
					break
				}
			}
			if matchedLine == "" {
				matchedLine = firstNonEmptyLine(output)
			}
			return AuthNeeds, sanitizeDetail(matchedLine)
		}
	}

	if exit == 0 {
		return AuthOK, ""
	}

	d := firstNonEmptyLine(output)
	return AuthUnknown, sanitizeDetail(d)
}

var (
	reSecretPrefix = regexp.MustCompile(`(ghp_|gho_|sk-)\S*`)
	reLongSecret   = regexp.MustCompile(`[A-Za-z0-9]{32,}`)
)

func sanitizeDetail(s string) string {
	if s == "" {
		return ""
	}
	s = reSecretPrefix.ReplaceAllString(s, "***")
	s = reLongSecret.ReplaceAllString(s, "***")
	runes := []rune(s)
	if len(runes) > 80 {
		runes = runes[:80]
	}
	return string(runes)
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) > max {
		return string(runes[:max])
	}
	return s
}

func ParseAuthResult(kind string, exit int, output string, err error) (status, detail string) {
	if err != nil {
		d := err.Error()
		if f := firstNonEmptyLine(output); f != "" {
			d = f
		}
		return AuthUnknown, sanitizeDetail(d)
	}

	upperKind := strings.ToUpper(strings.TrimSpace(kind))

	if upperKind == "CLAUDE" {
		start := strings.Index(output, "{")
		end := strings.LastIndex(output, "}")
		if start != -1 && end > start {
			jsonStr := output[start : end+1]
			var parsed struct {
				LoggedIn *bool `json:"loggedIn"`
			}
			if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil && parsed.LoggedIn != nil {
				if *parsed.LoggedIn {
					return AuthOK, ""
				}
				return AuthNeeds, "claude: loggedIn=false"
			}
		}
		d := firstNonEmptyLine(output)
		if d == "" {
			d = "claude: invalid auth status json"
		}
		return AuthUnknown, sanitizeDetail(d)
	}

	if upperKind == AuthKindGH {
		if exit == 0 {
			return AuthOK, ""
		}
		d := firstNonEmptyLine(output)
		if strings.Contains(strings.ToLower(output), "not logged") {
			return AuthNeeds, sanitizeDetail(d)
		}
		return AuthUnknown, sanitizeDetail(d)
	}

	// Kind khác: exit 0 -> OK; khớp DefaultAuthPatterns -> NEEDS_AUTH; còn lại UNKNOWN
	if exit == 0 {
		return AuthOK, ""
	}

	outLower := strings.ToLower(output)
	for _, p := range DefaultAuthPatterns() {
		if strings.Contains(outLower, strings.ToLower(p)) {
			matchedLine := ""
			for _, line := range strings.Split(output, "\n") {
				if strings.Contains(strings.ToLower(line), strings.ToLower(p)) {
					matchedLine = strings.TrimSpace(line)
					break
				}
			}
			if matchedLine == "" {
				matchedLine = firstNonEmptyLine(output)
			}
			return AuthNeeds, sanitizeDetail(matchedLine)
		}
	}

	d := firstNonEmptyLine(output)
	return AuthUnknown, sanitizeDetail(d)
}

type authInfo struct {
	status    string
	detail    string
	checkedAt time.Time
}

func (r *Runner) maybeSendRequestTask(ctx context.Context, agentID string) {
	r.mu.Lock()
	if r.ghAuthStatus == AuthNeeds {
		r.mu.Unlock()
		return
	}
	ag, ok := r.agents[agentID]
	if !ok || ag.status == "BUSY" || ag.authStatus == AuthNeeds {
		r.mu.Unlock()
		return
	}
	if ag.everOK != nil && !*ag.everOK {
		r.mu.Unlock()
		return
	}
	if !ag.cooldownUntil.IsZero() && r.now().Before(ag.cooldownUntil) {
		r.mu.Unlock()
		return
	}

	var cmd []string
	if r.cfg.AuthChecks != nil {
		cmd = r.cfg.AuthChecks[ag.kind]
	}
	needsCheck := r.cfg.AuthChecks != nil && len(cmd) > 0 && r.now().Sub(ag.authCheckedAt) > r.cfg.AuthFreshFor
	kind := ag.kind
	configDir := ag.configDir
	r.mu.Unlock()

	if needsCheck {
		probeCtx, cancel := context.WithTimeout(ctx, r.cfg.AuthCheckTimeout)
		var buf bytes.Buffer
		exit, err := r.runCommand(probeCtx, "", cmd, agentEnv(kind, configDir), &buf)
		cancel()
		newStatus, newDetail := ParseAuthResult(kind, exit, buf.String(), err)

		r.mu.Lock()
		ag, ok = r.agents[agentID]
		if !ok || ag.status == "BUSY" {
			r.mu.Unlock()
			return
		}
		oldStatus := ag.authStatus
		if r.cfg.AuthProbes != nil && len(r.cfg.AuthProbes[kind]) > 0 {
			if oldStatus == AuthNeeds && newStatus == AuthOK {
				r.mu.Unlock()
				return
			}
		}
		ag.authStatus = newStatus
		ag.authDetail = newDetail
		ag.authCheckedAt = r.now()
		statusChanged := (newStatus != oldStatus)
		canRequest := (newStatus != AuthNeeds && r.ghAuthStatus != AuthNeeds)
		if canRequest {
			ag.status = "IDLE"
		}
		r.mu.Unlock()

		if statusChanged {
			payload := map[string]any{
				"event":       "AGENT_AUTH",
				"scope":       "agent",
				"agent_id":    agentID,
				"auth_status": newStatus,
			}
			if newDetail != "" {
				payload["detail"] = newDetail
			} else {
				payload["detail"] = nil
			}
			r.send(payload)
			r.writeStatus(true)
		}

		if canRequest {
			r.send(map[string]any{
				"event":    "REQUEST_TASK",
				"agent_id": agentID,
			})
		}
		return
	}

	// Không cần kiểm tra fresh
	r.mu.Lock()
	ag, ok = r.agents[agentID]
	if !ok || ag.status == "BUSY" || ag.authStatus == AuthNeeds || r.ghAuthStatus == AuthNeeds {
		r.mu.Unlock()
		return
	}
	ag.status = "IDLE"
	r.mu.Unlock()

	r.send(map[string]any{
		"event":    "REQUEST_TASK",
		"agent_id": agentID,
	})
}

func (r *Runner) handleAuthRecheck(ctx context.Context, targetAgentID string) {
	if targetAgentID != "" {
		// Kiểm riêng một agent
		r.mu.Lock()
		ag, ok := r.agents[targetAgentID]
		var kind string
		var configDir string
		var cmd []string
		var probeCmd []string
		if ok {
			kind = ag.kind
			configDir = ag.configDir
			if r.cfg.AuthChecks != nil {
				cmd = r.cfg.AuthChecks[kind]
			}
			if r.cfg.AuthProbes != nil {
				probeCmd = r.cfg.AuthProbes[kind]
			}
		}
		r.mu.Unlock()

		if !ok {
			return
		}

		extraEnv := agentEnv(kind, configDir)
		var newStatus string
		var newDetail string
		if len(cmd) > 0 {
			probeCtx, cancel := context.WithTimeout(ctx, r.cfg.AuthCheckTimeout)
			var buf bytes.Buffer
			exit, err := r.runCommand(probeCtx, "", cmd, extraEnv, &buf)
			cancel()
			newStatus, newDetail = ParseAuthResult(kind, exit, buf.String(), err)
		} else {
			// Agent không có lệnh kiểm (agy) -> trả UNKNOWN và xoá trạng thái NEEDS_AUTH cục bộ
			newStatus = AuthUnknown
			newDetail = ""
		}

		if len(probeCmd) > 0 {
			pCtx, cancelP := context.WithTimeout(ctx, r.cfg.ProbeTimeout)
			var pBuf bytes.Buffer
			pExit, pErr := r.runCommand(pCtx, "", probeCmd, extraEnv, &pBuf)
			cancelP()
			pStatus, pDetail := ParseProbeResult(pExit, pBuf.String(), pErr)
			if pStatus == AuthOK || pStatus == AuthNeeds {
				newStatus = pStatus
				newDetail = pDetail
			}
		}

		r.mu.Lock()
		ag, ok = r.agents[targetAgentID]
		if ok {
			ag.authStatus = newStatus
			ag.authDetail = newDetail
			ag.authCheckedAt = r.now()
		}
		r.mu.Unlock()

		// Luôn trả AGENT_AUTH
		payload := map[string]any{
			"event":       "AGENT_AUTH",
			"scope":       "agent",
			"agent_id":    targetAgentID,
			"auth_status": newStatus,
		}
		if newDetail != "" {
			payload["detail"] = newDetail
		} else {
			payload["detail"] = nil
		}
		r.send(payload)
		r.writeStatus(true)

		// Sau đó xin việc cho agent rảnh không còn NEEDS_AUTH
		r.maybeSendRequestTask(ctx, targetAgentID)
		return
	}

	// targetAgentID == "" -> GH + mọi agent
	// 1. GH
	var ghCmd []string
	r.mu.Lock()
	if r.cfg.AuthChecks != nil {
		ghCmd = r.cfg.AuthChecks[AuthKindGH]
	}
	r.mu.Unlock()

	var newGhStatus string
	var newGhDetail string
	if len(ghCmd) > 0 {
		probeCtx, cancel := context.WithTimeout(ctx, r.cfg.AuthCheckTimeout)
		var buf bytes.Buffer
		exit, err := r.exec.Run(probeCtx, "", ghCmd, &buf)
		cancel()
		newGhStatus, newGhDetail = ParseAuthResult(AuthKindGH, exit, buf.String(), err)
	} else {
		newGhStatus = AuthUnknown
		newGhDetail = ""
	}

	r.mu.Lock()
	r.ghAuthStatus = newGhStatus
	r.ghAuthDetail = newGhDetail
	r.ghAuthCheckedAt = r.now()
	r.mu.Unlock()

	ghPayload := map[string]any{
		"event":       "AGENT_AUTH",
		"scope":       "gh",
		"auth_status": newGhStatus,
	}
	if newGhDetail != "" {
		ghPayload["detail"] = newGhDetail
	} else {
		ghPayload["detail"] = nil
	}
	r.send(ghPayload)

	// 2. Mọi agent
	type agentCheckInfo struct {
		id        string
		kind      string
		configDir string
		cmd       []string
		probeCmd  []string
	}
	r.mu.Lock()
	var agentList []agentCheckInfo
	for id, ag := range r.agents {
		var cmd []string
		if r.cfg.AuthChecks != nil {
			cmd = r.cfg.AuthChecks[ag.kind]
		}
		var probeCmd []string
		if r.cfg.AuthProbes != nil {
			probeCmd = r.cfg.AuthProbes[ag.kind]
		}
		agentList = append(agentList, agentCheckInfo{id: id, kind: ag.kind, configDir: ag.configDir, cmd: cmd, probeCmd: probeCmd})
	}
	r.mu.Unlock()

	for _, item := range agentList {
		extraEnv := agentEnv(item.kind, item.configDir)
		var st string
		var dt string
		if len(item.cmd) > 0 {
			probeCtx, cancel := context.WithTimeout(ctx, r.cfg.AuthCheckTimeout)
			var buf bytes.Buffer
			exit, err := r.runCommand(probeCtx, "", item.cmd, extraEnv, &buf)
			cancel()
			st, dt = ParseAuthResult(item.kind, exit, buf.String(), err)
		} else {
			st = AuthUnknown
			dt = ""
		}

		if len(item.probeCmd) > 0 {
			pCtx, cancelP := context.WithTimeout(ctx, r.cfg.ProbeTimeout)
			var pBuf bytes.Buffer
			pExit, pErr := r.runCommand(pCtx, "", item.probeCmd, extraEnv, &pBuf)
			cancelP()
			pStatus, pDetail := ParseProbeResult(pExit, pBuf.String(), pErr)
			if pStatus == AuthOK || pStatus == AuthNeeds {
				st = pStatus
				dt = pDetail
			}
		}

		r.mu.Lock()
		if ag, ok := r.agents[item.id]; ok {
			ag.authStatus = st
			ag.authDetail = dt
			ag.authCheckedAt = r.now()
		}
		r.mu.Unlock()

		agPayload := map[string]any{
			"event":       "AGENT_AUTH",
			"scope":       "agent",
			"agent_id":    item.id,
			"auth_status": st,
		}
		if dt != "" {
			agPayload["detail"] = dt
		} else {
			agPayload["detail"] = nil
		}
		r.send(agPayload)
	}
	r.writeStatus(true)

	// Sau đó xin việc cho agent rảnh không còn NEEDS_AUTH
	r.mu.Lock()
	var toRequest []string
	if r.ghAuthStatus != AuthNeeds {
		for id, ag := range r.agents {
			if ag.status != "BUSY" && ag.authStatus != AuthNeeds {
				if ag.everOK != nil && !*ag.everOK {
					continue
				}
				if ag.cooldownUntil.IsZero() || !r.now().Before(ag.cooldownUntil) {
					toRequest = append(toRequest, id)
				}
			}
		}
	}
	r.mu.Unlock()

	for _, id := range toRequest {
		r.maybeSendRequestTask(ctx, id)
	}
}

func (r *Runner) performPeriodicAuthCheck(ctx context.Context) {
	if r.cfg.AuthChecks == nil {
		return
	}

	// 1. GH check
	var ghCmd []string
	r.mu.Lock()
	if r.cfg.AuthChecks != nil {
		ghCmd = r.cfg.AuthChecks[AuthKindGH]
	}
	r.mu.Unlock()

	if len(ghCmd) > 0 {
		probeCtx, cancel := context.WithTimeout(ctx, r.cfg.AuthCheckTimeout)
		var buf bytes.Buffer
		exit, err := r.exec.Run(probeCtx, "", ghCmd, &buf)
		cancel()
		newStatus, newDetail := ParseAuthResult(AuthKindGH, exit, buf.String(), err)

		r.mu.Lock()
		oldStatus := r.ghAuthStatus
		r.ghAuthStatus = newStatus
		r.ghAuthDetail = newDetail
		r.ghAuthCheckedAt = r.now()
		r.mu.Unlock()

		if newStatus != oldStatus {
			payload := map[string]any{
				"event":       "AGENT_AUTH",
				"scope":       "gh",
				"auth_status": newStatus,
			}
			if newDetail != "" {
				payload["detail"] = newDetail
			} else {
				payload["detail"] = nil
			}
			r.send(payload)
			r.writeStatus(true)

			if oldStatus == AuthNeeds && newStatus != AuthNeeds {
				r.mu.Lock()
				var toReq []string
				now := r.now()
				for id, ag := range r.agents {
					if ag.status != "BUSY" && ag.authStatus != AuthNeeds {
						if ag.cooldownUntil.IsZero() || !now.Before(ag.cooldownUntil) {
							toReq = append(toReq, id)
						}
					}
				}
				r.mu.Unlock()
				for _, id := range toReq {
					r.maybeSendRequestTask(ctx, id)
				}
			}
		}
	}

	// 2. Agents not BUSY with check command
	type agentCheckInfo struct {
		id        string
		kind      string
		configDir string
		cmd       []string
	}
	r.mu.Lock()
	var toCheck []agentCheckInfo
	for id, ag := range r.agents {
		if ag.status != "BUSY" {
			if cmd, ok := r.cfg.AuthChecks[ag.kind]; ok && len(cmd) > 0 {
				toCheck = append(toCheck, agentCheckInfo{id: id, kind: ag.kind, configDir: ag.configDir, cmd: cmd})
			}
		}
	}
	r.mu.Unlock()

	for _, item := range toCheck {
		probeCtx, cancel := context.WithTimeout(ctx, r.cfg.AuthCheckTimeout)
		var buf bytes.Buffer
		exit, err := r.runCommand(probeCtx, "", item.cmd, agentEnv(item.kind, item.configDir), &buf)
		cancel()
		newStatus, newDetail := ParseAuthResult(item.kind, exit, buf.String(), err)

		r.mu.Lock()
		ag, ok := r.agents[item.id]
		if !ok || ag.status == "BUSY" {
			r.mu.Unlock()
			continue
		}
		oldStatus := ag.authStatus
		if r.cfg.AuthProbes != nil && len(r.cfg.AuthProbes[item.kind]) > 0 {
			if oldStatus == AuthNeeds && newStatus == AuthOK {
				r.mu.Unlock()
				continue
			}
		}
		ag.authStatus = newStatus
		ag.authDetail = newDetail
		ag.authCheckedAt = r.now()
		statusChanged := (newStatus != oldStatus)
		canRequest := (oldStatus == AuthNeeds && newStatus != AuthNeeds && r.ghAuthStatus != AuthNeeds)
		r.mu.Unlock()

		if statusChanged {
			payload := map[string]any{
				"event":       "AGENT_AUTH",
				"scope":       "agent",
				"agent_id":    item.id,
				"auth_status": newStatus,
			}
			if newDetail != "" {
				payload["detail"] = newDetail
			} else {
				payload["detail"] = nil
			}
			r.send(payload)
			r.writeStatus(true)
		}

		if canRequest {
			r.maybeSendRequestTask(ctx, item.id)
		}
	}
}

func (r *Runner) performPeriodicAuthProbe(ctx context.Context) {
	if r.cfg.AuthProbes == nil {
		return
	}

	type agentProbeInfo struct {
		id        string
		kind      string
		configDir string
		cmd       []string
	}
	r.mu.Lock()
	var toProbe []agentProbeInfo
	for id, ag := range r.agents {
		if ag.status != "BUSY" {
			if cmd, ok := r.cfg.AuthProbes[ag.kind]; ok && len(cmd) > 0 {
				toProbe = append(toProbe, agentProbeInfo{id: id, kind: ag.kind, configDir: ag.configDir, cmd: cmd})
			}
		}
	}
	r.mu.Unlock()

	for _, item := range toProbe {
		probeCtx, cancel := context.WithTimeout(ctx, r.cfg.ProbeTimeout)
		var buf bytes.Buffer
		exit, err := r.runCommand(probeCtx, "", item.cmd, agentEnv(item.kind, item.configDir), &buf)
		cancel()
		newStatus, newDetail := ParseProbeResult(exit, buf.String(), err)

		// Brief: "Kết quả probe OK hoặc NEEDS_AUTH ghi đè kết quả lệnh kiểm; probe UNKNOWN giữ kết quả lệnh kiểm."
		if newStatus == AuthUnknown {
			continue
		}

		r.mu.Lock()
		ag, ok := r.agents[item.id]
		if !ok || ag.status == "BUSY" {
			r.mu.Unlock()
			continue
		}
		oldStatus := ag.authStatus
		statusChanged := (newStatus != oldStatus)
		if statusChanged {
			ag.authStatus = newStatus
			ag.authDetail = newDetail
			ag.authCheckedAt = r.now()
		}
		canRequest := (oldStatus == AuthNeeds && newStatus != AuthNeeds && r.ghAuthStatus != AuthNeeds)
		r.mu.Unlock()

		if statusChanged {
			payload := map[string]any{
				"event":       "AGENT_AUTH",
				"scope":       "agent",
				"agent_id":    item.id,
				"auth_status": newStatus,
			}
			if newDetail != "" {
				payload["detail"] = newDetail
			} else {
				payload["detail"] = nil
			}
			r.send(payload)
			r.writeStatus(true)
		}

		if canRequest {
			r.maybeSendRequestTask(ctx, item.id)
		}
	}

	// Kiểm tra các agent có EverOK == false trong cấu hình: khi ra OK → đặt true, lưu và đóng kết nối để HELLO lại
	var pendingAgents []AgentConfig
	r.mu.Lock()
	for _, a := range r.cfg.Agents {
		if a.EverOK != nil && !*a.EverOK {
			pendingAgents = append(pendingAgents, a)
		}
	}
	r.mu.Unlock()

	reconnectNeeded := false
	for _, pa := range pendingAgents {
		cmd := r.cfg.AuthProbes[pa.Kind]
		if len(cmd) == 0 && r.cfg.AuthChecks != nil {
			cmd = r.cfg.AuthChecks[pa.Kind]
		}
		if len(cmd) == 0 {
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, r.cfg.ProbeTimeout)
		var buf bytes.Buffer
		exit, err := r.runCommand(probeCtx, "", cmd, agentEnv(pa.Kind, pa.ConfigDir), &buf)
		cancel()
		st, _ := ParseProbeResult(exit, buf.String(), err)
		if st == AuthOK {
			r.mu.Lock()
			for i := range r.cfg.Agents {
				if strings.EqualFold(r.cfg.Agents[i].AgentName(), pa.AgentName()) {
					if r.cfg.Agents[i].EverOK != nil {
						*r.cfg.Agents[i].EverOK = true
					}
					break
				}
			}
			accountsPath := r.cfg.AccountsPath
			conn := r.conn
			r.mu.Unlock()

			if accountsPath != "" {
				if af, err := LoadAccounts(accountsPath); err == nil {
					for idx := range af.Agents {
						if strings.EqualFold(af.Agents[idx].Name, pa.AgentName()) {
							af.Agents[idx].EverOK = true
							break
						}
					}
					_ = SaveAccounts(accountsPath, af)
				}
			}
			if conn != nil {
				reconnectNeeded = true
			}
		}
	}

	if reconnectNeeded {
		r.mu.Lock()
		if r.conn != nil {
			_ = r.conn.Close()
		}
		r.mu.Unlock()
	}
}
