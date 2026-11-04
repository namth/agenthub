#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-016 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/logs internal/gateway); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/logs/... ./internal/gateway/...
out=$(go test ./internal/logs/... ./internal/gateway/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic|DATA RACE" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL|--- )" || true
if echo "$out" | grep -q -- "--- SKIP"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 27 ]; then echo "Chỉ $passed test PASS, cần >= 27"; exit 1; fi
go build ./... && go test ./... -count=1 >/dev/null
echo "TASK-016 ĐẠT"
