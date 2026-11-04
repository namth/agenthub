#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-027 — do trưởng phòng viết, file này bị khoá.
# Chạy từ thư mục gốc repo. Exit 0 = đạt.
set -euo pipefail

echo "== 1. gofmt + vet"
unformatted=$(gofmt -l internal/dispatcher)
if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/dispatcher/...

echo "== 2. Không dùng time.Now() trực tiếp"
if grep -n "time\.Now()" internal/dispatcher/*.go | grep -v "_test.go"; then
  echo "Phải lấy thời gian từ Clock"; exit 1
fi

echo "== 3. Test dispatcher (-race)"
out=$(go test ./internal/dispatcher/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL|--- )" || true
if echo "$out" | grep -q -- "--- SKIP"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 37 ]; then echo "Chỉ $passed test PASS, cần >= 37"; exit 1; fi

echo "== 4. Không phá test cũ"
go build ./... && go test ./... -count=1 >/dev/null

echo "TASK-027 ĐẠT"
