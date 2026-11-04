#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-046 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal cmd); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/tokens/... ./internal/mcp/... ./cmd/...
out=$(go test ./internal/tokens/... ./internal/mcp/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined" | head -40; exit 1; }
for t in TestCreateAuthenticateRevoke TestCreateValidationAndLimits TestLastUsedThrottled TestTokensHTTP TestAuth TestToolsList TestToolsCall TestRateLimit; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
grep -q "mcp.Handler" cmd/server/main.go || { echo "main.go chưa gắn /mcp"; exit 1; }
grep -q "tokens.Handler" cmd/server/main.go || { echo "main.go chưa gắn /api/me/tokens"; exit 1; }
go build ./...
cd web
[ -d node_modules ] || npm ci --silent
wout=$(npx vitest run 2>&1) || { echo "$wout" | grep -E "FAIL|×|Error|expected|Tests " | head -40; exit 1; }
echo "$wout" | grep -E "Test Files|Tests " || true
n=$(npx vitest run src/pages/profile.test.tsx --reporter=verbose 2>&1 | grep -cE "✓" || true)
[ "$n" -ge 6 ] || { echo "profile.test.tsx chỉ $n test PASS, muốn ≥ 6"; exit 1; }
npm run typecheck --silent
npm run build --silent >/dev/null
echo "TASK-046 ĐẠT"
