#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-033 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
PKGS="./internal/office/... ./internal/tasks/..."
unformatted=$(gofmt -l internal cmd); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet $PKGS ./cmd/...
out=$(go test $PKGS -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined|unknown field" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL)" || true
for t in TestPartnerStreamsNewPath TestOldPathRedirects TestPerUserAndTotalLimits TestDefaultLimits TestSettingsTelegramEnabled \
         TestAuthEventMapping TestPresenceTouchThrottled TestPresenceOrderingAndEvents TestPresenceMiddleware TestBuildSnapshot \
         TestOnApprovedHook TestRBACAndDisabled TestMaxStreams; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
for s in "office.BuildSnapshot" "office.NewPresence" "WithOnApproved" "/api/office/"; do
  grep -q "$s" cmd/server/main.go || { echo "cmd/server/main.go chưa nối: $s"; exit 1; }
done
go build ./...   # test package khác thuộc task song song: chỉ kiểm build
echo "TASK-033 ĐẠT"
