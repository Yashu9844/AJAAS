package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNormalizePagination(t *testing.T) {
	cases := []struct{ p, pp, wp, wpp int }{
		{0, 0, 1, 20}, {-5, -1, 1, 20}, {3, 50, 3, 50}, {2, 1000, 2, 100}, {1, 100, 1, 100}, {1, 101, 1, 100},
	}
	for _, c := range cases {
		p, pp := NormalizePagination(c.p, c.pp)
		if p != c.wp || pp != c.wpp {
			t.Errorf("NormalizePagination(%d,%d) = %d,%d want %d,%d", c.p, c.pp, p, pp, c.wp, c.wpp)
		}
	}
}

func TestParsePaginationToleratesJunk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for q, want := range map[string][2]int{
		"":                       {1, 20},
		"?page=2&per_page=10":    {2, 10},
		"?page=abc&per_page=xyz": {1, 20},
		"?per_page=0":            {1, 20},
		"?per_page=5000":         {1, 100},
		"?page=-9":               {1, 20},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/x"+q, nil)
		p, pp := ParsePagination(c)
		if p != want[0] || pp != want[1] {
			t.Errorf("%q -> %d,%d want %v", q, p, pp, want)
		}
	}
}

func TestBindErrorDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type body struct {
		Email string `json:"email" binding:"required,email"`
		Age   int    `json:"age"`
	}
	bind := func(raw string) error {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/x", stringsReader(raw))
		c.Request.Header.Set("Content-Type", "application/json")
		var b body
		return c.ShouldBindJSON(&b)
	}
	// validator failure -> field details, no Go struct names
	d := BindErrorDetails(bind(`{}`))
	if len(d) != 1 || d[0].Field != "Email" || d[0].Message == "" {
		t.Fatalf("validator details: %+v", d)
	}
	// type error -> field named
	d = BindErrorDetails(bind(`{"email":"a@b.co","age":"old"}`))
	if len(d) != 1 || d[0].Field != "age" {
		t.Fatalf("type error details: %+v", d)
	}
	// syntax error / empty body -> generic body detail
	for _, raw := range []string{`{bad`, ``} {
		d = BindErrorDetails(bind(raw))
		if len(d) != 1 || d[0].Field != "body" {
			t.Fatalf("%q details: %+v", raw, d)
		}
	}
	if b, _ := json.Marshal(d); len(b) == 0 {
		t.Fatal("details must be serialisable")
	}
}

func stringsReader(s string) *strings.Reader { return strings.NewReader(s) }
