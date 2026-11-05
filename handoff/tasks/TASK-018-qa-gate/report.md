# Report TASK-018 — Chốt chặn CI, Merge → DONE

> Ghi bởi trưởng phòng: agent viết xong code; verify đỏ do `ci_test.go` của trưởng phòng còn import thừa (`dispatcher`) — đã sửa test, khoá lại. Code không đổi.

## Output verify (15 dòng cuối)
```
--- PASS: TestServiceApproveRejectShareRules (0.04s)
--- PASS: TestServiceCreateHookAndErrors (0.04s)
--- PASS: TestHandlerUsesServiceHook (0.04s)
--- PASS: TestCreateTaskCreatesIssue (0.04s)
--- PASS: TestCreateValidation (0.03s)
--- PASS: TestCreateGitHubFailureNoRow (0.03s)
--- PASS: TestApproveMovesToPlanningAndQueuesSpec (0.04s)
--- PASS: TestRejectRequiresReasonAndClosesIssue (0.04s)
--- PASS: TestKanbanColumnsDEC11 (0.05s)
--- PASS: TestPartnerSeesOnlyRootTasksWithoutInternals (0.04s)
--- PASS: TestTaskDetail (0.04s)
--- PASS: TestWebhookSignature (0.03s)
--- PASS: TestWebhookIssueOpenedSyncsOnce (0.04s)
ok  	github.com/namth/agenthub/internal/tasks	2.243s
TASK-018 ĐẠT
```
