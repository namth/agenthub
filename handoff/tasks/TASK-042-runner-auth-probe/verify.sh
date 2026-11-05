#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-042 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/runner); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/runner/...
out=$(go test ./internal/runner/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined|unknown field" | head -40; exit 1; }
for t in TestParseProbeResult TestDefaultsIncludeOAuthAndProbes TestProbeOverridesStatusCheck TestProbeUnknownKeepsCheckResult \
         TestProbePeriodicOnlyOnChange TestJobOAuthErrorIsAuthNotFailure TestRecheckRunsProbe \
         TestHandshakeIncludesAuthAndBlocksRequests TestAuthPatternOnJobFailure TestExhaustedMidCodeJobHandsOff; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
grep -q "DefaultAuthProbes()" internal/runner/main.go || { echo "main.go chưa đặt AuthProbes: DefaultAuthProbes()"; exit 1; }
go build ./...   # test package khác thuộc task song song: chỉ kiểm build
echo "TASK-042 ĐẠT"
