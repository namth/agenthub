#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-044 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/smoke cmd); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/smoke/... ./cmd/...
out=$(go test ./internal/smoke/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined" | head -40; exit 1; }
for t in TestStartRules TestTrackToPassed TestTrackFailedAndTimeout TestHTTP; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
grep -q "smoke.Handler" cmd/server/main.go || { echo "main.go chưa gắn smoke.Handler"; exit 1; }
go build ./...
cd web
[ -d node_modules ] || npm ci --silent
wout=$(npx vitest run 2>&1) || { echo "$wout" | grep -E "FAIL|×|Error|expected|Tests " | head -40; exit 1; }
echo "$wout" | grep -E "Test Files|Tests " || true
n=$(npx vitest run src/pages/projects_ops.test.tsx --reporter=verbose 2>&1 | grep -cE "✓" || true)
[ "$n" -ge 3 ] || { echo "projects_ops.test.tsx chỉ $n test PASS, muốn ≥ 3"; exit 1; }
npm run typecheck --silent
npm run build --silent >/dev/null
echo "TASK-044 ĐẠT"
