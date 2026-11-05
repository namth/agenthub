#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-001 — do trưởng phòng viết, file này bị khoá.
# Chạy từ thư mục gốc repo. Exit 0 = đạt.
set -euo pipefail

echo "== 1. gofmt"
unformatted=$(gofmt -l cmd internal web 2>/dev/null || true)
if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi

echo "== 2. Không dùng CGO sqlite"
if grep -q "mattn/go-sqlite3" go.mod; then echo "Cấm mattn/go-sqlite3"; exit 1; fi
grep -q "modernc.org/sqlite" go.mod || { echo "Thiếu modernc.org/sqlite"; exit 1; }

echo "== 3. go vet"
go vet ./...

echo "== 4. go test"
out=$(go test ./... -count=1 -v 2>&1) || { echo "$out" | tail -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL|---)" | grep -v -- "--- PASS: TestConstraintsRejectInvalid/" || true
if echo "$out" | grep -q -- "--- SKIP"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 32 ]; then echo "Chỉ $passed test PASS, cần >= 32"; exit 1; fi

echo "== 5. Build single binary (CGO tắt) và chạy ở thư mục trống"
tmp=$(mktemp -d)
trap 'kill ${pid:-0} 2>/dev/null || true; rm -rf "$tmp"' EXIT
CGO_ENABLED=0 go build -o "$tmp/agenthub" ./cmd/server
port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
(cd "$tmp" && PORT=$port exec ./agenthub >"$tmp/server.log" 2>&1) &
pid=$!
for _ in $(seq 1 50); do
  curl -sf "http://127.0.0.1:$port/api/health" >/dev/null 2>&1 && break
  sleep 0.2
done
health=$(curl -sf "http://127.0.0.1:$port/api/health") || { echo "Server không lên:"; cat "$tmp/server.log"; exit 1; }
echo "health: $health"
echo "$health" | grep -q '"status":"ok"' || { echo "health sai"; exit 1; }
curl -sf "http://127.0.0.1:$port/" | grep -q "AgentHub" || { echo "/ không trả index.html nhúng"; exit 1; }
curl -sf "http://127.0.0.1:$port/kanban" | grep -q "AgentHub" || { echo "SPA fallback hỏng"; exit 1; }
[ -f "$tmp/data/agenthub.db" ] || { echo "Không tạo ./data/agenthub.db mặc định"; exit 1; }

echo "== 6. Tắt êm khi SIGTERM"
kill -TERM $pid
for _ in $(seq 1 25); do kill -0 $pid 2>/dev/null || break; sleep 0.2; done
if kill -0 $pid 2>/dev/null; then echo "Server không tắt sau SIGTERM 5s"; exit 1; fi

echo "TASK-001 ĐẠT"
