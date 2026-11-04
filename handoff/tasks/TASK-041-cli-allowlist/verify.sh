#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-041 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
PKGS="./internal/runner/... ./internal/pipeline/... ./internal/gateway/... ./internal/projects/..."
unformatted=$(gofmt -l internal cmd); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet $PKGS ./cmd/...
out=$(go test $PKGS -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined" | head -40; exit 1; }
for t in TestClaudeGetsPermissionFlags TestAgyGetsSessionSettings TestNoAllowlistKeepsOldArgv TestNeedsCINoteInPrompt \
         TestDefaultAllowedCommands TestPayloadForMergesProjectCommands TestPayloadExtrasMerged \
         TestPatchAllowedCommands TestPatchAllowedCommandsValidation; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
grep -q "WithPayloadExtras" cmd/server/main.go || { echo "main.go chưa nối WithPayloadExtras(pl.PayloadFor)"; exit 1; }
grep -rn "dangerously-skip-permissions\|bypassPermissions" internal/runner/*.go | grep -v _test.go && { echo "Cấm chế độ bỏ qua quyền trong runner"; exit 1; } || true
go build ./...
echo "TASK-041 ĐẠT"
