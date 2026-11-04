#!/usr/bin/env bash
# Lệnh xác minh tự động cho TASK-037 — do trưởng phòng viết, file này bị khoá.
set -euo pipefail
[ -f internal/skills/skills.go ] || { echo "Thiếu internal/skills/skills.go"; exit 1; }
grep -q "go:embed agenthub-spec/SKILL.md" internal/skills/skills.go || { echo "skills.go phải //go:embed agenthub-spec/SKILL.md"; exit 1; }
unformatted=$(gofmt -l internal/runner internal/skills); if [ -n "$unformatted" ]; then echo "Chưa gofmt: $unformatted"; exit 1; fi
go vet ./internal/runner/... ./internal/skills/...
out=$(go test ./internal/runner/... ./internal/skills/... -count=1 -race -v 2>&1) || { echo "$out" | grep -E -- "--- FAIL|_test.go:[0-9]+|panic|undefined" | head -40; exit 1; }
for t in TestEmbeddedSkill TestSpecGetsEmbeddedSkillInSnapshot TestProjectSkillOverrides TestCodeJobHasNoSkill TestNoSnapshotterKeepsOldBehaviour \
         TestClaudeGetsPermissionFlags TestProbeOverridesStatusCheck TestExhaustedOnSpecJobNoGit TestCodeJobSuccessCommitsPushesSubmits; do
  echo "$out" | grep -q -- "--- PASS: $t " || { echo "Thiếu PASS $t"; exit 1; }
done
echo "$out" | grep -E "^(ok|FAIL)"
go build ./...
echo "TASK-037 ĐẠT"
