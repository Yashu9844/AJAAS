package integration

import (
	"testing"
)

// TestOnboardingLifecycle — OG1..OG8 end to end: templates, a core hire starting onboarding (idempotently), a document
// task, completion, a termination starting offboarding, clearance, exit interview privacy and a rescind.
func TestOnboardingLifecycle(t *testing.T) {
	cc := newCoreTenant(t)
	alice := cc.memberEmp
	const base = "/hr/onboarding"
	mkTemplate := func(b map[string]any, tasks []map[string]any) string {
		t.Helper()
		r := cc.do(t, "POST", base+"/templates", body(b))
		expect(t, r, 201, "template")
		expect(t, cc.do(t, "PUT", base+"/templates/"+r.str("data.id")+"/tasks", body(tasks)), 200, "template tasks")
		return r.str("data.id")
	}
	mkTemplate(map[string]any{"kind": "onboarding", "name": "General"}, []map[string]any{{"title": "General", "owner_role": "hr"}})
	mkTemplate(map[string]any{"kind": "onboarding", "name": "Dept 1", "department_id": cc.dept1}, []map[string]any{
		{"title": "Upload ID proof", "owner_role": "employee", "document_type": "id_proof", "blocking": true, "due_offset_days": -2},
		{"title": "Welcome 1:1", "owner_role": "manager", "due_offset_days": 1},
		{"title": "Laptop", "owner_role": "it"}})
	mkTemplate(map[string]any{"kind": "offboarding", "name": "Exit"}, []map[string]any{{"title": "Return laptop", "owner_role": "it"}})

	// OB-03 a completed core hire starts onboarding; OG3 the department template wins.
	hire := cc.complete(t, "hire", alice, cc.day(-5), map[string]any{"position_id": cc.p1, "manager_employee_id": cc.adminEmp,
		"worker_type": "employee", "employment_type": "full_time"})
	lst := cc.do(t, "GET", base+"/checklists?kind=onboarding&employee_id="+alice)
	if len(lst.list("data")) != 1 || lst.str("data.0.source") != "core_action" || lst.str("data.0.manager_employee_id") != cc.adminEmp {
		t.Fatalf("OB-03 start: %s", lst.Raw)
	}
	cid := lst.str("data.0.id")
	cl := cc.do(t, "GET", base+"/checklists/"+cid)
	if len(cl.list("data.tasks")) != 3 || cl.str("data.tasks.0.title") != "Upload ID proof" || cl.str("data.tasks.0.assignee_employee_id") != alice ||
		cl.str("data.tasks.1.assignee_employee_id") != cc.adminEmp || cl.get("data.progress.overdue") != 3.0 {
		t.Fatalf("OG3/OB-04 tasks: %s", cl.Raw)
	}
	idTask, welcome, laptop := cl.str("data.tasks.0.id"), cl.str("data.tasks.1.id"), cl.str("data.tasks.2.id")

	// ON-007 the worker sees and completes their own task; OG5 needs a verified document.
	if me := cc.as(t, cc.memberTok, "GET", base+"/me"); len(me.list("data")) != 1 {
		t.Fatalf("worker /me: %s", me.Raw)
	}
	if mine := cc.as(t, cc.memberTok, "GET", base+"/my-tasks"); len(mine.list("data")) != 1 {
		t.Fatalf("worker my-tasks: %s", mine.Raw)
	}
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/tasks/"+laptop+"/done"), 404, "NOT_FOUND", "worker completes a pool task")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/tasks/"+idTask+"/done"), 409, "DOCUMENT_REQUIRED", "OG5 no document")
	doc := cc.do(t, "POST", "/hr/employees/"+alice+"/documents", body(map[string]any{"document_type": "id_proof", "file_name": "id.pdf",
		"file_url": "https://example.com/id.pdf", "file_size": 1024, "mime_type": "application/pdf"}))
	expect(t, doc, 201, "document")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/tasks/"+idTask+"/done"), 409, "DOCUMENT_REQUIRED", "OG5 unverified document")
	expect(t, cc.do(t, "POST", "/hr/employees/"+alice+"/documents/"+doc.str("data.id")+"/verify"), 200, "verify document")
	if d := cc.as(t, cc.memberTok, "POST", base+"/tasks/"+idTask+"/done", body(map[string]any{"note": "uploaded"})); d.str("data.evidence_document_id") != doc.str("data.id") {
		t.Fatalf("OG5 evidence: %s", d.Raw)
	}
	expectErr(t, cc.do(t, "POST", base+"/tasks/"+idTask+"/skip", body(map[string]any{"reason": "x"})), 409, "TASK_STATE", "skip a done task")
	expect(t, cc.do(t, "POST", base+"/tasks/"+welcome+"/done"), 200, "manager task")
	expect(t, cc.do(t, "POST", base+"/tasks/"+laptop+"/done"), 200, "pool task")
	if s := sqlStr(t, "SELECT status FROM onb_checklists WHERE id = ?", cid); s != "completed" {
		t.Fatalf("OB-06 auto-complete: %s", s)
	}

	// OB-03 termination starts offboarding from the last working day; OG6 clearance; OG7 exit interview privacy.
	term := cc.complete(t, "terminate", alice, cc.day(-1), map[string]any{"termination_type": "voluntary", "last_working_day": cc.day(10)})
	off := cc.do(t, "GET", base+"/checklists?kind=offboarding&employee_id="+alice)
	if len(off.list("data")) != 1 || off.str("data.0.anchor_date") != cc.day(10) {
		t.Fatalf("OB-03 offboarding: %s", off.Raw)
	}
	oid := off.str("data.0.id")
	expectErr(t, cc.do(t, "POST", base+"/checklists/"+oid+"/complete"), 409, "CLEARANCE_PENDING", "OG6 complete before clearance")
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/checklists/"+oid+"/clearances/hr/clear"), 403, "FORBIDDEN", "worker signs clearance")
	for _, area := range []string{"manager", "hr", "it"} {
		expect(t, cc.do(t, "POST", base+"/checklists/"+oid+"/clearances/"+area+"/clear", body(map[string]any{"note": "ok"})), 200, "clear "+area)
	}
	expectErr(t, cc.do(t, "POST", base+"/checklists/"+oid+"/clearances/hr/clear"), 409, "ALREADY_CLEARED", "clear twice")
	if n := sqlStr(t, "SELECT count(*)::text FROM onb_events_outbox WHERE tenant_id = ? AND routing_key = 'hr.onboarding.clearance_completed'", cc.id); n != "0" {
		t.Fatalf("OG6 early event: %s", n)
	}
	expect(t, cc.do(t, "POST", base+"/checklists/"+oid+"/clearances/finance/clear"), 200, "clear finance")
	if n := sqlStr(t, "SELECT count(*)::text FROM onb_events_outbox WHERE tenant_id = ? AND routing_key = 'hr.onboarding.clearance_completed'", cc.id); n != "1" {
		t.Fatalf("OG6 event: %s", n)
	}
	// The worker's login is deactivated only after the last working day, so they can still submit.
	expect(t, cc.as(t, cc.memberTok, "PUT", base+"/checklists/"+oid+"/exit-interview", body(map[string]any{"answers": map[string]string{"why": "SECRET-EXIT-REASON"}})), 201, "exit interview")
	expectErr(t, cc.as(t, cc.memberTok, "PUT", base+"/checklists/"+oid+"/exit-interview", body(map[string]any{"answers": map[string]string{"why": "x"}})), 409, "ALREADY_SUBMITTED", "submit twice")
	reader, _, _ := cc.limitedUser(t, [2]string{"onboarding", "read"})
	expectErr(t, cc.as(t, reader, "GET", base+"/checklists/"+oid+"/exit-interview"), 404, "NOT_FOUND", "OG7 onboarding:read cannot read answers")
	if ei := cc.do(t, "GET", base+"/checklists/"+oid+"/exit-interview"); ei.str("data.answers.why") != "SECRET-EXIT-REASON" {
		t.Fatalf("OG7 hr reads: %s", ei.Raw)
	}
	if n := sqlStr(t, "SELECT count(*)::text FROM onb_events_outbox WHERE tenant_id = ? AND payload::text LIKE '%SECRET%'", cc.id); n != "0" {
		t.Fatalf("event privacy: %s", n)
	}
	if p := cc.do(t, "GET", base+"/reports/progress?kind=offboarding"); len(p.list("data")) != 1 || p.get("data.0.open_checklists") != 1.0 {
		t.Fatalf("OB-10 progress: %s", p.Raw)
	}

	// OG8 rescinding the termination cancels its checklist.
	expect(t, cc.as(t, cc.approver, "POST", "/hr/core/actions/"+term.str("data.id")+"/rescind", body(map[string]any{"reason": "withdrawn"})), 200, "rescind")
	if s := sqlStr(t, "SELECT status FROM onb_checklists WHERE id = ?", oid); s != "cancelled" {
		t.Fatalf("OG8: %s", s)
	}
	_ = hire

	// OG1 isolation and RBAC.
	expectErr(t, cc.as(t, cc.memberTok, "POST", base+"/templates", body(map[string]any{})), 403, "", "worker manages templates")
	other := newTenant(t)
	expectErr(t, other.do(t, "GET", base+"/checklists/"+cid), 404, "NOT_FOUND", "OG1 foreign checklist")
}
