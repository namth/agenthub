#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-007 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
echo "== 1. gofmt + vet"
unformatted=$(gofmt -l internal/tasks); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/tasks/...
echo "== 2. So chữ ký hằng thời gian, không log bí mật"
grep -q "hmac.Equal" internal/tasks/*.go || { echo "Phải so chữ ký bằng hmac.Equal"; exit 1; }
if grep -n -i -E "log\.(Print|Fatal).*(token|secret)" internal/tasks/*.go | grep -v _test.go; then echo "Không log bí mật"; exit 1; fi
echo "== 3. Test tasks (-race)"
out=$(go test ./internal/tasks/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic|DATA RACE" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL|--- )" || true
if echo "$out" | grep -q -- "--- SKIP"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 10 ]; then echo "Chỉ $passed test PASS, cần >= 10"; exit 1; fi
echo "== 4. Không phá test cũ"
go test ./internal/database/... ./internal/dispatcher/... ./internal/gateway/... ./internal/runner/... ./internal/server/... ./web/... -count=1
echo "TASK-007 ĐẠT"
