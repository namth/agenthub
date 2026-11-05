#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-026 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/tasks internal/pipeline internal/gateway); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/tasks/... ./internal/pipeline/... ./internal/gateway/...
out=$(go test ./internal/tasks/... ./internal/pipeline/... ./internal/gateway/... ./internal/e2e/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic|DATA RACE" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL)" || true
if echo "$out" | grep -q -- "--- SKIP"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 67 ]; then echo "Chỉ $passed test PASS, cần >= 67"; exit 1; fi
go build ./... && go test ./... -count=1 >/dev/null
echo "TASK-026 ĐẠT"
