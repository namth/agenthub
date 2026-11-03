#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-004 — do trưởng phòng viết, file này bị khoá.
# Chạy từ thư mục gốc repo. Exit 0 = đạt.
set -euo pipefail

echo "== 1. gofmt + vet"
unformatted=$(gofmt -l internal/runner cmd)
if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./...

echo "== 2. Không log token, không nhận token qua cờ"
if grep -n -i -E "log\.(Print|Fatal).*[Tt]oken" internal/runner/*.go | grep -v _test.go; then echo "Không log token"; exit 1; fi
if grep -n -E 'flag\.(String|StringVar)\([^)]*"token"' internal/runner/*.go; then echo "Không nhận token qua cờ --token"; exit 1; fi

echo "== 3. Test runner (-race)"
out=$(go test ./internal/runner/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic|DATA RACE" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL|--- )" || true
if echo "$out" | grep -q -- "--- SKIP"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 18 ]; then echo "Chỉ $passed test PASS, cần >= 18"; exit 1; fi

echo "== 4. Không phá test cũ"
go test ./internal/... ./web/... -count=1

echo "== 5. Binary thật: server + runner"
tmp=$(mktemp -d)
cleanup() { kill ${rpid:-0} ${spid:-0} 2>/dev/null || true; rm -rf "$tmp"; }
trap cleanup EXIT
CGO_ENABLED=0 go build -o "$tmp/agenthub" ./cmd/server
"$tmp/agenthub" runner --help >"$tmp/help.txt" 2>&1 || { cat "$tmp/help.txt"; echo "runner --help phải exit 0"; exit 1; }
grep -q -- "--server" "$tmp/help.txt" || { echo "--help thiếu --server"; exit 1; }
port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
(cd "$tmp" && PORT=$port RUNNER_REGISTRATION_KEY=k-verify exec ./agenthub >"$tmp/server.log" 2>&1) &
spid=$!
for _ in $(seq 1 50); do curl -sf "http://127.0.0.1:$port/api/health" >/dev/null 2>&1 && break; sleep 0.2; done
reg=$(curl -sf -X POST "http://127.0.0.1:$port/api/fleet/register" -H "X-Registration-Key: k-verify" \
  -H "Content-Type: application/json" -d '{"name":"verify-box","os":"darwin","agents":["ANTIGRAVITY"]}') || { echo "Đăng ký máy lỗi"; cat "$tmp/server.log"; exit 1; }
token=$(echo "$reg" | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["runner_token"])')
mkdir -p "$tmp/repo" && git -C "$tmp/repo" init -q
(cd "$tmp/repo" && AGENTHUB_RUNNER_TOKEN=$token exec "$tmp/agenthub" runner --server "ws://127.0.0.1:$port/ws/agent" \
  --agent antigravity='true {prompt}' >"$tmp/runner.log" 2>&1) &
rpid=$!
for _ in $(seq 1 50); do grep -q "runner: đã kết nối" "$tmp/runner.log" 2>/dev/null && break; sleep 0.2; done
grep -q "runner: đã kết nối" "$tmp/runner.log" || { echo "Runner không kết nối được:"; cat "$tmp/runner.log"; exit 1; }
if grep -q "$token" "$tmp/runner.log" "$tmp/server.log"; then echo "Token bị lộ trong log"; exit 1; fi
kill -TERM $rpid
for _ in $(seq 1 25); do kill -0 $rpid 2>/dev/null || break; sleep 0.2; done
if kill -0 $rpid 2>/dev/null; then echo "Runner không dừng sau SIGTERM"; exit 1; fi

echo "TASK-004 ĐẠT"
