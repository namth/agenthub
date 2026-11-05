#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-048 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
PKGS="./internal/pipeline/... ./internal/tasks/... ./internal/telegram/..."
unformatted=$(gofmt -l internal cmd); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet $PKGS ./cmd/...
out=$(go test $PKGS -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined" | head -40; exit 1; }
for t in TestReplanRetry TestReplanSubtaskReplaced TestReplanRootTask TestReplanAskUser TestReplanCancelIsOnlyProposal TestReplanInvalidNoLoop TestReplanPromptSchema \
         TestProposalDecisions TestProposalNotifyAndCallback TestNeedInfoMovesToWaitingUser TestSpecCreatesSubtasksAndCodeJobs; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
for s in WithOnProposal NotifyProposal; do grep -q "$s" cmd/server/main.go || { echo "main.go chưa nối $s"; exit 1; }; done
go build ./...
cd web
[ -d node_modules ] || npm ci --silent
wout=$(npx vitest run 2>&1) || { echo "$wout" | grep -E "FAIL|×|Error|expected|Tests " | head -40; exit 1; }
echo "$wout" | grep -E "Test Files|Tests " || true
n=$(npx vitest run src/pages/proposal.test.tsx --reporter=verbose 2>&1 | grep -cE "✓" || true)
[ "$n" -ge 4 ] || { echo "proposal.test.tsx chỉ $n test PASS, muốn ≥ 4"; exit 1; }
npm run typecheck --silent
npm run build --silent >/dev/null
echo "TASK-048 ĐẠT"
