#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-031 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/runner); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/runner/...
out=$(go test ./internal/runner/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined|unknown field" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL)" || true
for t in TestParseAuthResult TestDefaultAuthChecksAndPatterns TestHandshakeIncludesAuthAndBlocksRequests TestAuthCheckTimeoutIsUnknown \
         TestGhNeedsAuthBlocksAllRequests TestRecheckClearsNeedsAuth TestRecheckWithoutCommandReturnsUnknown \
         TestAuthPatternOnJobFailure TestExhaustBeatsAuthPattern TestPeriodicCheckSendsOnlyChanges; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
grep -q "DefaultAuthChecks()" internal/runner/main.go || { echo "main.go chưa đặt AuthChecks: DefaultAuthChecks()"; exit 1; }
go build ./...   # test của package khác thuộc task song song (TASK-030): chỉ kiểm build
echo "TASK-031 ĐẠT"
