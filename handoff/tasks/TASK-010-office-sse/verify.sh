#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-010 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
echo "== 1. gofmt + vet"
unformatted=$(gofmt -l internal/office); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/office/...
echo "== 2. Test office (-race)"
out=$(go test ./internal/office/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic|DATA RACE" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL|--- )" || true
if echo "$out" | grep -q -- "--- SKIP"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 8 ]; then echo "Chỉ $passed test PASS, cần >= 8"; exit 1; fi
echo "== 3. Không phá test cũ"
go test ./internal/database/... ./internal/dispatcher/... ./internal/gateway/... ./internal/runner/... ./internal/server/... ./internal/auth/... ./web/... -count=1
echo "TASK-010 ĐẠT"
