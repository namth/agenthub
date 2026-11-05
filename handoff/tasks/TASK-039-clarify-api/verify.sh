#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-039 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/tasks internal/telegram cmd); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/tasks/... ./internal/telegram/... ./cmd/...
out=$(go test ./internal/tasks/... ./internal/telegram/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined" | head -40; exit 1; }
for t in TestGetClarifications TestClarifyAnswerFlow TestClarifyAuto TestKanbanWaitingUser TestParseAnswerComment TestAnswerByLoginAndHook \
         TestPickOptionTelegram TestIssueCommentWebhook TestClarifyCallback TestNewCommand TestMergeCallback TestApproveMovesToPlanningAndQueuesSpec; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
grep -q "WithOnIssueComment" cmd/server/main.go || { echo "main.go chưa nối WithOnIssueComment → AnswerByLogin"; exit 1; }
go build ./...
echo "TASK-039 ĐẠT"
