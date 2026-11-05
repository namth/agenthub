#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-043 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
unformatted=$(gofmt -l internal/dispatcher); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/dispatcher/...
out=$(go test ./internal/dispatcher/... ./internal/api/... ./internal/gateway/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic" | head -40; exit 1; }
for t in TestPartnerMachineOnlyMemberProjects TestPartnerFilterCombinesWithAllowlist TestPartnerLeadOnlyJobsFiltered TestReclaimWhenMembershipLost \
         TestAgentProjectAllowlist TestLeadOnlyIgnoresAllowlist TestFairShareByWeight TestAuthGuardRequestJob TestLeadAnyKind; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
go build ./...   # test package khác thuộc task song song: chỉ kiểm build
echo "TASK-043 ĐẠT"
