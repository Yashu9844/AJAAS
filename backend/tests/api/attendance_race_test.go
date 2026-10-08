//go:build integration

package api

import (
	"net/http"
	"sync"
	"testing"
	"time"
)

// TestAttendanceLive_AST6_ConcurrentPunches — AS-T6/EC-15: the per-employee advisory lock admits exactly one
// session for simultaneous INs; AS-T1: a client-sent punch_time is ignored (server clock only).
func TestAttendanceLive_AST6_ConcurrentPunches(t *testing.T) {
	a := newAttTenant(t)
	const n = 10
	codes := make([]int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i], _ = a.do(t, a.memberTok, http.MethodPost, "/api/v1/attendance/punch",
				map[string]string{"type": "in", "punch_time": "2020-01-01T09:00:00Z"})
		}(i)
	}
	wg.Wait()
	created, conflicts := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicts++
		}
	}
	if created != 1 || conflicts != n-1 {
		t.Fatalf("AS-T6: want 1×201 and %d×409, got %v", n-1, codes)
	}
	where := "employee_profile_id = '" + a.memberEmp + "'"
	if got := psql(t, "SELECT count(*) FROM attendance_punches WHERE "+where); got != "1" {
		t.Fatalf("AS-T6: exactly one punch row expected, got %s", got)
	}
	if got := psql(t, "SELECT count(*) FROM attendance_records WHERE "+where); got != "1" {
		t.Fatalf("AS-T6: exactly one record expected, got %s", got)
	}
	year := psql(t, "SELECT extract(year FROM punch_time)::int FROM attendance_punches WHERE "+where)
	if year != time.Now().UTC().Format("2006") {
		t.Fatalf("AS-T1: punch_time must be server time, got year %s", year)
	}
}
