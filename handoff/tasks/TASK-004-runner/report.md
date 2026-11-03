# Report TASK-004 — Runner daemon

> Ghi bởi trưởng phòng: agent viết xong code (exec.go, git.go, main.go, runner.go, utils.go, nhánh `runner` trong cmd/server/main.go) nhưng phiên agy dừng trước khi tự nộp.

## Lệch so với brief
- Hai lỗi nằm ở `verify.sh` của trưởng phòng (đã sửa, khoá lại): `trap` gọi `kill ${pid:-0}` giết cả nhóm tiến trình khi PID trống; kiểm `--help` đòi `--server` hai gạch trong khi `flag` của Go in một gạch.
- Còn để sau: `ExecExecutor` chỉ giết tiến trình con trực tiếp khi huỷ (CLI sinh tiến trình cháu có thể sót) — nên đặt process group.

## Output verify (30 dòng cuối)
```
== 2. Không log token, không nhận token qua cờ
== 3. Test runner (-race)
--- PASS: TestEndToEndWithRealServer (1.33s)
--- PASS: TestRealGitWorktreeLifecycle (0.46s)
--- PASS: TestExecExecutor (0.21s)
--- PASS: TestConnectHandshakeAndRequestPerAgent (0.00s)
--- PASS: TestHeartbeat (0.09s)
--- PASS: TestReconnectsAfterDrop (0.03s)
--- PASS: TestCodeJobSuccessCommitsPushesSubmits (0.00s)
--- PASS: TestLogChunksBatchedAndComplete (0.00s)
--- PASS: TestPromptIncludesHandoffNoteAsData (0.00s)
--- PASS: TestExhaustedMidCodeJobHandsOff (0.50s)
--- PASS: TestExhaustedPushFailStillReports (0.00s)
--- PASS: TestExhaustedOnSpecJobNoGit (0.00s)
--- PASS: TestCLIFailureNotSubmitted (0.40s)
--- PASS: TestAbortCancelsCLI (0.38s)
--- PASS: TestLeaseRenewLoopAndStop (0.82s)
--- PASS: TestParseResetsAt (0.00s)
--- PASS: TestDefaultExhaustPatterns (0.00s)
--- PASS: TestBuildHandoffNote (0.01s)
ok  	github.com/namth/agenthub/internal/runner	5.609s
== 4. Không phá test cũ
ok  	github.com/namth/agenthub/internal/database	0.610s
ok  	github.com/namth/agenthub/internal/dispatcher	0.722s
ok  	github.com/namth/agenthub/internal/gateway	1.620s
ok  	github.com/namth/agenthub/internal/runner	4.924s
ok  	github.com/namth/agenthub/internal/server	1.166s
ok  	github.com/namth/agenthub/web	0.904s
== 5. Binary thật: server + runner
TASK-004 ĐẠT
```
