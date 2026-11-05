#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-040 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
cd web
[ -d node_modules ] || npm ci --silent
out=$(npx vitest run 2>&1) || { echo "$out" | grep -E "FAIL|×|Error|expected|Tests " | head -60; exit 1; }
echo "$out" | grep -E "Test Files|Tests " || true
n=$(npx vitest run src/pages/clarify.test.tsx --reporter=verbose 2>&1 | grep -cE "✓" || true)
[ "$n" -ge 8 ] || { echo "clarify.test.tsx chỉ $n test PASS, muốn ≥ 8"; exit 1; }
if grep -n "dangerouslySetInnerHTML" src/pages/TaskDetail.tsx src/components/ClarifyPanel.tsx; then echo "Cấm dangerouslySetInnerHTML"; exit 1; fi
npm run typecheck --silent
npm run build --silent >/dev/null
echo "TASK-040 ĐẠT"
