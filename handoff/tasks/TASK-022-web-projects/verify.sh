#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-022 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
cd web
[ -d node_modules ] || npm ci --no-audit --no-fund
npx tsc -b
w=$(npx vitest run --reporter=verbose 2>&1) || { echo "$w" | grep -E "✗|×|FAIL|Error|expected" | head -60; echo "$w" | tail -30; exit 1; }
echo "$w" | tail -8
if echo "$w" | grep -q -E "↓|skipped"; then echo "Có test bị skip"; exit 1; fi
n=$(echo "$w" | grep -E -c "✓" || true)
if [ "$n" -lt 82 ]; then echo "Chỉ $n test web PASS, cần >= 82"; exit 1; fi
d=$(mktemp -d); npx vite build --outDir "$d" --emptyOutDir >/dev/null; rm -rf "$d"
cd ..
go build ./...
echo "TASK-022 ĐẠT"
