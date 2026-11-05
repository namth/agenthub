#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-015 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
cd web
[ -d node_modules ] || npm ci --no-audit --no-fund
echo "== 1. typecheck"
npx tsc -b
echo "== 2. vitest"
out=$(npx vitest run --reporter=verbose 2>&1) || { echo "$out" | tail -60; exit 1; }
echo "$out" | tail -30
if echo "$out" | grep -q -E "↓|skipped"; then echo "Có test bị skip"; exit 1; fi
passed=$(echo "$out" | grep -E -c "✓" || true)
if [ "$passed" -lt 42 ]; then echo "Chỉ $passed test PASS, cần >= 42"; exit 1; fi
echo "== 3. build"
out_dir=$(mktemp -d)
npx vite build --outDir "$out_dir" --emptyOutDir >/dev/null
grep -q "AgentHub" "$out_dir/index.html"
rm -rf "$out_dir"
echo "TASK-015 ĐẠT"
