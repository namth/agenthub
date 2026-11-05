#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-047 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
cd web
[ -d node_modules ] || npm ci --silent
out=$(npx vitest run 2>&1) || { echo "$out" | grep -E "FAIL|×|Error|expected|Tests " | head -60; exit 1; }
echo "$out" | grep -E "Test Files|Tests " || true
n=$(npx vitest run src/layout/responsive.test.tsx --reporter=verbose 2>&1 | grep -cE "✓" || true)
[ "$n" -ge 7 ] || { echo "responsive.test.tsx chỉ $n test PASS, muốn ≥ 7"; exit 1; }
npm run typecheck --silent
npm run build --silent >/dev/null
echo "TASK-047 ĐẠT"
