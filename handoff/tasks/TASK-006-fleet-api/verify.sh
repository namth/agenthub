#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-006 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
echo "== 1. gofmt + vet"
unformatted=$(gofmt -l internal/api); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/api/...
echo "== 2. Lease chỉ đổi qua dispatcher"
if grep -n -i -E "UPDATE +lead_lease|INSERT +INTO +lead_lease" internal/api/*.go | grep -v _test.go; then echo "Không ghi lead_lease trực tiếp"; exit 1; fi
echo "== 3. Test api (-race)"
out=$(go test ./internal/api/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic|DATA RACE" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL|--- )" || true
if echo "$out" | grep -q -- "--- SKIP"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 7 ]; then echo "Chỉ $passed test PASS, cần >= 7"; exit 1; fi
echo "== 4. Không phá test cũ"
go test ./internal/database/... ./internal/dispatcher/... ./internal/gateway/... ./internal/runner/... ./internal/server/... ./web/... -count=1
echo "TASK-006 ĐẠT"
