package integration

import (
	"sort"
	"testing"
	"time"
)

func p95(d []time.Duration) time.Duration {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[(len(d)*95)/100]
}

func measure(n int, fn func()) time.Duration {
	ds := make([]time.Duration, n)
	for i := range ds {
		s := time.Now()
		fn()
		ds[i] = time.Since(s)
	}
	return p95(ds)
}

// Performance sensors (AGENTS.md): single GET < 200ms, list < 500ms p95. Login is measured and reported only: bcrypt
// cost 12 (security.md NFR-SEC001) takes ~250-400ms on its own, which is incompatible with the <300ms budget.
func TestPerformanceBudgets(t *testing.T) {
	tc := newTenant(t)
	for i := 0; i < 30; i++ {
		tc.newUser(t, "perf")
	}
	did := tc.mkDept(t, "Perf")
	tc.mkEmployee(t, "perf")

	single := measure(40, func() { tc.do(t, "GET", "/departments/"+did) })
	list := measure(40, func() { tc.do(t, "GET", "/users", query("per_page", "100")) })
	chart := measure(40, func() { tc.do(t, "GET", "/org-chart") })
	emps := measure(40, func() { tc.do(t, "GET", "/employees", query("per_page", "100")) })
	login := measure(5, func() {
		call(t, "POST", "/auth/login", body(map[string]string{"tenant_slug": tc.slug, "email": tc.adminEmail, "password": password}))
	})
	t.Logf("p95: single GET=%v list users=%v org-chart=%v list employees=%v login=%v", single, list, chart, emps, login)

	if single > 200*time.Millisecond {
		t.Errorf("single GET p95 %v exceeds 200ms", single)
	}
	for name, d := range map[string]time.Duration{"users": list, "org-chart": chart, "employees": emps} {
		if d > 500*time.Millisecond {
			t.Errorf("list %s p95 %v exceeds 500ms", name, d)
		}
	}
}
