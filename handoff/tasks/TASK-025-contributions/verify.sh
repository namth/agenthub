#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-025 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/contrib); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/contrib/...
out=$(go test ./internal/contrib/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL|--- )" || true
passed=$(echo "$out" | grep -c -- "--- PASS" || true)
if [ "$passed" -lt 4 ]; then echo "Chỉ $passed test Go PASS, cần >= 4"; exit 1; fi
go build ./... && go test ./... -count=1 >/dev/null
cd web
[ -d node_modules ] || npm ci --no-audit --no-fund
npx tsc -b
w=$(npx vitest run --reporter=verbose 2>&1) || { echo "$w" | tail -60; exit 1; }
echo "$w" | tail -8
if echo "$w" | grep -q -E "↓|skipped"; then echo "Có test bị skip"; exit 1; fi
n=$(echo "$w" | grep -E -c "✓" || true)
if [ "$n" -lt 57 ]; then echo "Chỉ $n test web PASS, cần >= 57"; exit 1; fi
d=$(mktemp -d); npx vite build --outDir "$d" --emptyOutDir >/dev/null; rm -rf "$d"
echo "TASK-025 ĐẠT"
