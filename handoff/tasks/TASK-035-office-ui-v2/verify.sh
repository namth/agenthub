#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-035 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
cd web
test -d node_modules || npm ci --silent
out=$(npx vitest run 2>&1) || { echo "$out" | grep -E "FAIL|×|Error|expected|Tests " | head -60; exit 1; }
echo "$out" | grep -E "Test Files|Tests " || true
n=$(npx vitest run src/office/office_v2.test.tsx --reporter=verbose 2>&1 | grep -cE "✓" || true)
[ "$n" -ge 15 ] || { echo "office_v2.test.tsx chỉ có $n test PASS, muốn ≥ 15"; exit 1; }
grep -q "dangerouslySetInnerHTML" src/pages/Office.tsx src/office/*.ts && { echo "Cấm dangerouslySetInnerHTML"; exit 1; } || true
grep -q "plant\|Plant" src/office/renderer.ts || { echo "renderer.ts chưa vẽ cây xanh (bám legacy)"; exit 1; }
npm run typecheck --silent
npm run build --silent >/dev/null
test -f dist/brand/inova-logo.png && test -f dist/brand/inova-icon.png || { echo "Thiếu logo trong dist/brand"; exit 1; }
echo "TASK-035 ĐẠT"
