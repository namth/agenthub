#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-020 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/projects internal/auth); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/projects/... ./internal/auth/...
if grep -n -i -E "log\.(Print|Fatal).*(token|secret)" internal/projects/*.go internal/auth/*.go | grep -v _test.go; then echo "Không log bí mật"; exit 1; fi
out=$(go test ./internal/projects/... ./internal/auth/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic|DATA RACE" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL|--- )" || true
if echo "$out" | grep -q -- "--- SKIP"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 21 ]; then echo "Chỉ $passed test PASS, cần >= 21"; exit 1; fi
go build ./... && go test ./... -count=1 >/dev/null
echo "TASK-020 ĐẠT"
