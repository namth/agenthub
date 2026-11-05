#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-030 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
PKGS="./internal/database/... ./internal/dispatcher/... ./internal/gateway/... ./internal/telegram/... ./internal/machines/... ./internal/api/..."
test -f internal/database/migrations/004_agent_auth_office.sql || { echo "Thiếu migration 004_agent_auth_office.sql"; exit 1; }
unformatted=$(gofmt -l internal cmd); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet $PKGS ./cmd/...
out=$(go test $PKGS -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|cannot|undefined" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL)" || true
for t in TestConnectAuthStoresStatusBeforeLease TestAuthGuardRequestJob TestSetAuthValidationAndEvents TestAuthDetailSanitized \
         TestAuthRequiredRequeuesWithoutAttemptPenalty TestLeadLosesAuth \
         TestHandshakeCarriesAuth TestAgentAuthMessageRouting TestAgentAuthErrorReported TestAgentAuthIgnoredWithoutAuthAPI TestSendAuthRecheck \
         TestAuthNotifyTransitions TestAuthNotifyCommandHints \
         TestAuthRecheckPermissions TestMyMachinesAuthFields TestJoinAntigravityNotLeadEligible \
         TestFleetAuthFields TestMigration004; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
grep -q "SendAuthRecheck" cmd/server/main.go || { echo "cmd/server/main.go chưa nối Recheck ← hub.SendAuthRecheck"; exit 1; }
go build ./... && go test ./... -count=1 >/dev/null
echo "TASK-030 ĐẠT"
