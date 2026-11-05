#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-049 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal cmd); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/... ./cmd/...
PKGS="./internal/database/... ./internal/dispatcher/... ./internal/api/... ./internal/machines/... ./internal/gateway/... ./internal/office/..."
out=$(go test $PKGS -count=1 -race -v -skip "TestInstallScriptRegistersOnlyLoggedIn" 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined" | head -40; exit 1; }
for t in TestMigration006AgentAccounts TestMigration005KeepsData TestHelloTwoClaudeAccounts TestHelloDefaultNameKeepsLegacyID TestHelloRejectsBadName \
         TestNeverOKHiddenAndNotDispatched TestRemoveAgent TestRemoveLeadReleasesLease TestFleetHidesNeverOKAndRemoved TestDeleteAgent \
         TestJoinNewMachineAgentsNotEverOK TestOwnerOfMachineDeletesAgent TestAgentRemovedRoutesToWorker TestSnapshotOnlyRealAgents TestFleetShape; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
# Toàn bộ package server còn lại vẫn xanh (không -race cho nhanh).
go test $(go list ./internal/... | grep -v /internal/runner) -count=1 -skip "TestInstallScriptRegistersOnlyLoggedIn" 2>&1 | grep -E "^(FAIL|---)" && { echo "Có package khác đỏ"; exit 1; } || true
grep -q "RemoveAgent" cmd/server/main.go || { echo "main.go chưa nối machines.Config.RemoveAgent"; exit 1; }
go build ./...
echo "TASK-049 ĐẠT"
