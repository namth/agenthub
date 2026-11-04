#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-009 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
echo "== 1. gofmt + vet"
unformatted=$(gofmt -l internal/telegram); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/telegram/...
echo "== 2. So secret hằng thời gian"
grep -q -E "subtle\.ConstantTimeCompare|hmac\.Equal" internal/telegram/*.go || { echo "Phải so secret hằng thời gian"; exit 1; }
echo "== 3. Test telegram (-race)"
out=$(go test ./internal/telegram/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic|DATA RACE" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL|--- )" || true
if echo "$out" | grep -q -- "--- SKIP"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 11 ]; then echo "Chỉ $passed test PASS, cần >= 11"; exit 1; fi
echo "== 4. Không phá test cũ"
go test ./internal/database/... ./internal/dispatcher/... ./internal/gateway/... ./internal/runner/... ./internal/server/... ./internal/auth/... ./web/... -count=1
echo "TASK-009 ĐẠT"
