#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-029 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/gateway); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/gateway/...
out=$(go test ./internal/gateway/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go|panic" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL)" || true
for t in TestHandshakeServerVersion TestVersionSinkReceivesRunnerVersion TestVersionSinkSkipsEmptyVersion TestCloseWorker; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
go build ./... && go test ./... -count=1 >/dev/null
echo "TASK-029 ĐẠT"
