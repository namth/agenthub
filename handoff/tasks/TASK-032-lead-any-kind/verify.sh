#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-032 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
PKGS="./internal/dispatcher/... ./internal/api/... ./internal/gateway/... ./internal/machines/..."
unformatted=$(gofmt -l internal); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet $PKGS
out=$(go test $PKGS -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined" | head -60; exit 1; }
echo "$out" | grep -E "^(ok|FAIL)" || true
for t in TestNewAntigravityNotLeadEligible TestLeadAnyKind TestCandidateOrderingAcrossKinds TestVacantWhenNoEligibleAndEtaAnyKind \
         TestQAIndependent TestAdminAssignAntigravityWhenEligible TestLeadLosesAuth TestLeadEligible TestCandidateOrdering; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
go build ./...   # test package khác thuộc task song song: chỉ kiểm build
echo "TASK-032 ĐẠT"
