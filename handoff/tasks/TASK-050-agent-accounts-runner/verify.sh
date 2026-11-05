#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-050 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal cmd); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/runner/... ./internal/machines/... ./cmd/...
out=$(go test ./internal/runner/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined" | head -40; exit 1; }
for t in TestAccountsFileRoundTrip TestAccountEnvOnJobAndProbe TestHelloOnlyEverOK TestAgentRemoveRewritesAccounts \
         TestConnectHandshakeAndRequestPerAgent TestCodeJobSuccessCommitsPushesSubmits TestProbeOverridesStatusCheck; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
mout=$(go test ./internal/machines/... -count=1 -v 2>&1) || { echo "$mout" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic" | head -40; exit 1; }
for t in TestInstallScriptRegistersOnlyLoggedIn TestInstallScriptEndToEnd; do
  echo "$mout" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
go build ./...
bin=$(mktemp -d)/agenthub
go build -o "$bin" ./cmd/server
help=$("$bin" account --help 2>&1 || true)
for sub in add remove list login; do
  echo "$help" | grep -q "$sub" || { echo "thiếu lệnh 'agenthub account $sub' trong --help"; exit 1; }
done
echo "TASK-050 ĐẠT"
