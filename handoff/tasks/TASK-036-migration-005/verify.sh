#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-036 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
[ -f internal/database/migrations/005_ops_remote.sql ] || { echo "Thiếu 005_ops_remote.sql"; exit 1; }
head -1 internal/database/migrations/005_ops_remote.sql | grep -qx -- "-- agenthub:foreign_keys=off" || { echo "005 phải bắt đầu bằng dòng -- agenthub:foreign_keys=off"; exit 1; }
unformatted=$(gofmt -l internal/database); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/database/...
out=$(go test ./internal/database/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic" | head -40; exit 1; }
for t in TestMigration005KeepsData TestMigration005NewSchema TestMigration004 TestMigrateLegacyDBKeepsData TestMigrateIdempotentVersions; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
go build ./...   # test package khác thuộc task song song: chỉ kiểm build
echo "TASK-036 ĐẠT"
