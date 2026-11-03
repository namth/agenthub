#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-003 — do trưởng phòng viết, file này bị khoá.
# Chạy từ thư mục gốc repo. Exit 0 = đạt.
set -euo pipefail

echo "== 1. gofmt + vet"
unformatted=$(gofmt -l internal/gateway)
if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/gateway/...

echo "== 2. Phụ thuộc"
grep -q "github.com/gorilla/websocket" go.mod || { echo "Thiếu gorilla/websocket"; exit 1; }

echo "== 3. Không lộ token"
if grep -n -i -E "log\.(Print|Fatal).*token" internal/gateway/*.go | grep -v _test.go; then echo "Không log token"; exit 1; fi

echo "== 4. Test gateway (-race)"
out=$(go test ./internal/gateway/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic|DATA RACE" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL|--- )" || true
if echo "$out" | grep -q -- "--- SKIP"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 19 ]; then echo "Chỉ $passed test PASS, cần >= 19"; exit 1; fi

echo "== 5. Không phá test cũ"
go test ./internal/database/... ./internal/server/... ./web/... -count=1

echo "TASK-003 ĐẠT"
