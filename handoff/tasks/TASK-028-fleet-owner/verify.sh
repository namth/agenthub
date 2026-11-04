#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-028 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/api internal/projects); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/api/... ./internal/projects/...
out=$(go test ./internal/api/... ./internal/projects/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL)" || true
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 23 ]; then echo "Chỉ $passed test PASS, cần >= 23"; exit 1; fi
go build ./... && go test ./... -count=1 >/dev/null
echo "TASK-028 ĐẠT"
