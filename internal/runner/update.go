package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

func downloadBaseURL(serverURL string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(u.Scheme) {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func parseExpectedChecksum(raw string) string {
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) > 0 {
			return strings.ToLower(parts[0])
		}
	}
	return ""
}

func (r *Runner) shouldUpdateLocked() bool {
	if r.cfg.NoUpdate || r.updating || r.updated {
		return false
	}
	if r.serverVersion == "" || r.serverVersion == Version {
		return false
	}
	if !r.lastUpdateFail.IsZero() && r.now().Sub(r.lastUpdateFail) < 30*time.Minute {
		return false
	}
	if len(r.activeTasks) > 0 {
		return false
	}
	for _, ag := range r.agents {
		if ag.status != "IDLE" {
			return false
		}
	}
	return true
}

func (r *Runner) shouldUpdate() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shouldUpdateLocked()
}

func (r *Runner) performSelfUpdate(ctx context.Context) bool {
	r.mu.Lock()
	if !r.shouldUpdateLocked() {
		r.mu.Unlock()
		return false
	}
	r.updating = true
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.updating = false
		r.mu.Unlock()
	}()

	err := r.doUpdate(ctx)
	if err != nil {
		log.Printf("runner: tự cập nhật thất bại: %v (thử lại sau 30 phút)", err)
		r.mu.Lock()
		r.lastUpdateFail = r.now()
		r.mu.Unlock()
		return false
	}

	r.mu.Lock()
	r.updated = true
	conn := r.conn
	r.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	return true
}

func (r *Runner) doUpdate(ctx context.Context) error {
	baseURL, err := downloadBaseURL(r.cfg.ServerURL)
	if err != nil {
		return fmt.Errorf("baseURL từ ServerURL: %w", err)
	}

	binaryURL := fmt.Sprintf("%s/download/agenthub-%s-%s", baseURL, runtime.GOOS, runtime.GOARCH)
	shaURL := binaryURL + ".sha256"

	client := &http.Client{Timeout: 30 * time.Second}

	// 1. Tải sha256
	shaReq, err := http.NewRequestWithContext(ctx, http.MethodGet, shaURL, nil)
	if err != nil {
		return err
	}
	shaResp, err := client.Do(shaReq)
	if err != nil {
		return fmt.Errorf("tải sha256 thất bại: %w", err)
	}
	defer shaResp.Body.Close()
	if shaResp.StatusCode != http.StatusOK {
		return fmt.Errorf("tải sha256 HTTP %d", shaResp.StatusCode)
	}
	shaData, err := io.ReadAll(io.LimitReader(shaResp.Body, 1024))
	if err != nil {
		return fmt.Errorf("đọc sha256: %w", err)
	}
	expectedHash := parseExpectedChecksum(string(shaData))
	if expectedHash == "" {
		return errors.New("file sha256 rỗng hoặc sai định dạng")
	}

	// 2. Tải binary
	binReq, err := http.NewRequestWithContext(ctx, http.MethodGet, binaryURL, nil)
	if err != nil {
		return err
	}
	binResp, err := client.Do(binReq)
	if err != nil {
		return fmt.Errorf("tải binary thất bại: %w", err)
	}
	defer binResp.Body.Close()
	if binResp.StatusCode != http.StatusOK {
		return fmt.Errorf("tải binary HTTP %d", binResp.StatusCode)
	}
	binData, err := io.ReadAll(io.LimitReader(binResp.Body, 100*1024*1024))
	if err != nil {
		return fmt.Errorf("đọc binary: %w", err)
	}

	// 3. Kiểm checksum
	h := sha256.Sum256(binData)
	actualHash := hex.EncodeToString(h[:])
	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("checksum không khớp: nhận %s, mong đợi %s", actualHash, expectedHash)
	}

	// 4. Ghi file tạm cùng thư mục SelfPath
	selfPath := r.cfg.SelfPath
	if selfPath == "" {
		p, err := os.Executable()
		if err != nil {
			return fmt.Errorf("os.Executable: %w", err)
		}
		selfPath = p
	}

	dir := filepath.Dir(selfPath)
	tmpFile, err := os.CreateTemp(dir, ".agenthub-update-*")
	if err != nil {
		return fmt.Errorf("tạo file tạm: %w", err)
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := tmpFile.Write(binData); err != nil {
		tmpFile.Close()
		return fmt.Errorf("ghi file tạm: %w", err)
	}
	if err := tmpFile.Chmod(0o755); err != nil {
		tmpFile.Close()
		return fmt.Errorf("chmod: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("đóng file tạm: %w", err)
	}

	if err := os.Rename(tmpName, selfPath); err != nil {
		return fmt.Errorf("rename ghi đè SelfPath: %w", err)
	}

	return nil
}

func (r *Runner) writeStatus(connected bool) {
	if r.cfg.StatusFile == "" {
		return
	}
	r.statusMu.Lock()
	defer r.statusMu.Unlock()

	r.mu.Lock()
	type agentEntry struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	agents := make([]agentEntry, 0, len(r.agents))
	for _, ag := range r.agents {
		agents = append(agents, agentEntry{
			ID:     ag.id,
			Status: ag.status,
		})
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })

	statusData := struct {
		Connected bool         `json:"connected"`
		Server    string       `json:"server"`
		Version   string       `json:"version"`
		Agents    []agentEntry `json:"agents"`
		UpdatedAt string       `json:"updated_at"`
	}{
		Connected: connected,
		Server:    r.cfg.ServerURL,
		Version:   Version,
		Agents:    agents,
		UpdatedAt: r.now().UTC().Format(time.RFC3339),
	}
	r.mu.Unlock()

	data, err := json.MarshalIndent(statusData, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(r.cfg.StatusFile), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(r.cfg.StatusFile, data, 0o600)
}
