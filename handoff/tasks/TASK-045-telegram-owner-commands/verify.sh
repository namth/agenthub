#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-045 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/telegram cmd); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/telegram/... ./cmd/...
out=$(go test ./internal/telegram/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined" | head -40; exit 1; }
for t in TestNewCommand TestCommandsIgnoreOtherChats TestTasksAndStatus TestMergeCallback TestNotifyQAPassedHasMergeButton TestCallbackApproveFromAdminChat TestCallbackSecurity; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
[ -f cmd/server/telegram_commander.go ] || { echo "Thiếu cmd/server/telegram_commander.go"; exit 1; }
grep -q "WebhookHandlerWith" cmd/server/main.go || { echo "main.go chưa dùng WebhookHandlerWith"; exit 1; }
grep -q "NotifyQAPassed" cmd/server/*.go || { echo "main chưa gọi NotifyQAPassed"; exit 1; }
go build ./...
echo "TASK-045 ĐẠT"
