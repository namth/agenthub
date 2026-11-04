#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-038 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
PKGS="./internal/pipeline/... ./internal/telegram/... ./internal/tasks/..."
unformatted=$(gofmt -l internal cmd); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet $PKGS ./cmd/...
out=$(go test $PKGS -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined" | head -40; exit 1; }
for t in TestNeedInfoMovesToWaitingUser TestNeedInfoInvalidIsReplan TestRoundCap TestPromptIncludesPreviousAnswers TestExplicitDoneStillCreatesSubtasks TestReminders \
         TestNotifyClarifyQuestions TestCommentIssue TestSpecCreatesSubtasksAndCodeJobs TestSpecInvalidTriggersReplan TestClarifyAnswerFlow; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
for s in WithOnNeedInfo RemindClarifications NotifyClarifyQuestions; do grep -q "$s" cmd/server/main.go || { echo "main.go chưa nối $s"; exit 1; }; done
go build ./...
echo "TASK-038 ĐẠT"
