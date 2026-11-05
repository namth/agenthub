package dispatcher_test

// Test do trưởng phòng viết: sự kiện giao/nộp việc cho văn phòng ảo (FEAT-02).

import (
	"testing"

	"github.com/namth/agenthub/internal/dispatcher"
)

func TestAssignAndSubmitEvents(t *testing.T) {
	e := newEnv(t)
	e.machine("m", dispatcher.KindAntigravity)
	job := e.job("t1", dispatcher.JobCode, dispatcher.KindAntigravity)
	a := e.request("m", "m/antigravity")
	ev, ok := e.rec.last(dispatcher.EvJobAssigned)
	if !ok || ev.AgentID != "m/antigravity" || ev.AssignmentID != a.ID || ev.JobID != job || ev.TaskID != "t1" || ev.Reason != "CODE" {
		t.Fatalf("JOB_ASSIGNED = %+v", ev)
	}
	if err := e.svc.Submit("m", a.ID); err != nil {
		t.Fatal(err)
	}
	ev, ok = e.rec.last(dispatcher.EvJobSubmitted)
	if !ok || ev.AgentID != "m/antigravity" || ev.AssignmentID != a.ID || ev.JobID != job {
		t.Fatalf("JOB_SUBMITTED = %+v", ev)
	}
	if e.rec.count(dispatcher.EvJobSubmitted) != 1 {
		t.Fatal("chỉ phát 1 JOB_SUBMITTED")
	}
}
