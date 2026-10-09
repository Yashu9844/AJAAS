//go:build integration

package api

import (
	"net/http"
	"sync"
	"testing"
)

// TestLeaveLive_G9_RBAC_G10_SelfApproval — member lacks leave:* but can use self endpoints; no self-approval.
func TestLeaveLive_G9_RBAC_G10_SelfApproval(t *testing.T) {
	a := newAttTenant(t)
	el := leaveType(t, a, "EL", 10)
	for _, p := range []struct{ m, path string }{
		{http.MethodPost, "/api/v1/leave/types"},
		{http.MethodPost, "/api/v1/leave/holidays"},
		{http.MethodGet, "/api/v1/leave/requests"},
		{http.MethodGet, "/api/v1/leave/balances?employee_id=" + a.adminEmp},
		{http.MethodGet, "/api/v1/leave/ledger?employee_id=" + a.adminEmp},
		{http.MethodPost, "/api/v1/leave/balances/adjust"},
	} {
		status, env := a.do(t, a.memberTok, p.m, p.path, map[string]string{})
		requireStatus(t, "G9 member "+p.m+" "+p.path, status, 403, env)
	}
	for _, path := range []string{"/api/v1/leave/types", "/api/v1/leave/holidays", "/api/v1/leave/balances/me", "/api/v1/leave/requests/me"} {
		status, env := a.do(t, a.memberTok, http.MethodGet, path, nil)
		requireStatus(t, "G9 member "+path, status, 200, env)
	}
	status, env := applyLeave(t, a, a.memberTok, el, monday(2), monday(2))
	requireStatus(t, "member apply", status, 201, env)
	memberReq := strField(t, env.Data, "id")
	status, env = a.do(t, a.memberTok, http.MethodPost, "/api/v1/leave/requests/"+memberReq+"/approve", nil)
	requireStatus(t, "G9 member approve", status, 403, env)

	status, env = applyLeave(t, a, a.adminTok, el, monday(2), monday(2))
	requireStatus(t, "admin apply", status, 201, env)
	adminReq := strField(t, env.Data, "id")
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/leave/requests/"+adminReq+"/approve", nil)
	requireCode(t, "G10 self approval", status, 403, env, "SELF_APPROVAL_FORBIDDEN")
	if st := psql(t, "SELECT status FROM leave_requests WHERE id = '"+adminReq+"'"); st != "pending" {
		t.Fatalf("G10: must stay pending, got %s", st)
	}
}

// TestLeaveLive_G1_Isolation — tenant B cannot touch tenant A's ids; A's JWT is rejected on B's host.
func TestLeaveLive_G1_Isolation(t *testing.T) {
	a, b := newAttTenant(t), newAttTenant(t)
	typA := leaveType(t, a, "EL", 10)
	status, env := applyLeave(t, a, a.memberTok, typA, monday(3), monday(3))
	requireStatus(t, "A apply", status, 201, env)
	reqA := strField(t, env.Data, "id")
	status, env = a.do(t, a.adminTok, http.MethodPost, "/api/v1/leave/holidays", map[string]string{"date": ymd(monday(4)), "name": "A day"})
	requireStatus(t, "A holiday", status, 201, env)
	holA := strField(t, env.Data, "id")
	for _, p := range []struct{ m, path string }{
		{http.MethodGet, "/api/v1/leave/types/" + typA},
		{http.MethodPatch, "/api/v1/leave/types/" + typA},
		{http.MethodGet, "/api/v1/leave/requests/" + reqA},
		{http.MethodPost, "/api/v1/leave/requests/" + reqA + "/approve"},
		{http.MethodDelete, "/api/v1/leave/holidays/" + holA},
	} {
		status, env := b.do(t, b.adminTok, p.m, p.path, map[string]string{})
		requireStatus(t, "G1 tenant B "+p.m+" "+p.path, status, 404, env)
	}
	status, env = applyLeave(t, b, b.memberTok, typA, monday(3), monday(3))
	requireStatus(t, "G1 B applies with A's type", status, 404, env)
	status, env = b.do(t, b.adminTok, http.MethodGet, "/api/v1/leave/balances?employee_id="+a.memberEmp, nil)
	requireCode(t, "G1 B reads A employee balance", status, 404, env, "EMPLOYEE_NOT_FOUND")
	status, env = doReq(t, http.MethodGet, "/api/v1/leave/requests", b.host, a.adminTok, nil)
	requireStatus(t, "G1 A token on B host", status, 403, env)
}

// TestLeaveLive_G14_Concurrency — LV-014: parallel applies cannot overdraw a balance.
func TestLeaveLive_G14_Concurrency(t *testing.T) {
	a := newAttTenant(t)
	typ := leaveType(t, a, "CL", 5)
	const n = 5
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mon := monday(i + 1) // disjoint Mon–Tue ranges, 2 days each
			codes[i], _ = applyLeave(t, a, a.memberTok, typ, mon, mon.AddDate(0, 0, 1))
		}(i)
	}
	wg.Wait()
	created, insufficient := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			insufficient++
		}
	}
	if created != 2 || insufficient != 3 {
		t.Fatalf("G14: want 2×201 + 3×409, got %v", codes)
	}
	if b := balanceFor(t, a, a.memberTok, typ); b["available"].(float64) != 1 || b["reserved"].(float64) != 4 {
		t.Fatalf("G14 balance: %+v", b)
	}
}
