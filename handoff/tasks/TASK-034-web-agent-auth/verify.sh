#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-034 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
cd web
test -d node_modules || npm ci --silent
out=$(npx vitest run 2>&1) || { echo "$out" | grep -E "FAIL|✗|×|Error|expected|Tests " | head -60; exit 1; }
echo "$out" | grep -E "Test Files|Tests " || true
n=$(npx vitest run src/pages/agentauth.test.tsx --reporter=verbose 2>&1 | grep -cE "✓" || true)
[ "$n" -ge 10 ] || { echo "agentauth.test.tsx chỉ có $n test PASS, muốn ≥ 10"; exit 1; }
grep -q "dangerouslySetInnerHTML" src/pages/Fleet.tsx src/pages/MyMachines.tsx src/components/AuthBadge.tsx && { echo "Cấm dangerouslySetInnerHTML"; exit 1; } || true
npm run typecheck --silent
npm run build --silent >/dev/null
echo "TASK-034 ĐẠT"
