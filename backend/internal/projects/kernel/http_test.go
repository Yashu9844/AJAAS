package kernel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func ctxWith(method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return m
}

func TestFailMapsErrorsWithoutLeaking(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{NotFound("task"), 404, "NOT_FOUND"},
		{Conflict("X_Y", "boom"), 409, "X_Y"},
		{&VersionConflict{Current: 7}, 409, "VERSION_CONFLICT"},
		{gorm.ErrRecordNotFound, 404, "NOT_FOUND"},
		{gorm.ErrDuplicatedKey, 409, "CONFLICT"},
		{http.ErrAbortHandler, 500, "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		c, w := ctxWith("GET", "/", "")
		Fail(c, tc.err)
		body := decode(t, w)
		e := body["error"].(map[string]any)
		if w.Code != tc.status || e["code"] != tc.code {
			t.Errorf("%v: %d %v", tc.err, w.Code, body)
		}
		if tc.status == 500 && strings.Contains(w.Body.String(), "abort") {
			t.Error("internal errors must not leak their text")
		}
	}
	c, w := ctxWith("GET", "/", "")
	Fail(c, &VersionConflict{Current: 7})
	if decode(t, w)["error"].(map[string]any)["current_version"] != float64(7) {
		t.Fatal("the version conflict carries the current version")
	}
}

func TestRespondersAndParsers(t *testing.T) {
	c, w := ctxWith("GET", "/", "")
	Respond(c, 201, map[string]int{"a": 1}, nil)
	if w.Code != 201 || decode(t, w)["data"] == nil {
		t.Fatal("respond")
	}
	c, w = ctxWith("GET", "/", "")
	Respond(c, 200, nil, NotFound("x"))
	if w.Code != 404 {
		t.Fatal("respond error")
	}
	c, w = ctxWith("GET", "/", "")
	RespondCursor(c, NewCursorPage[int](nil, 10, ""), nil)
	m := decode(t, w)
	if len(m["data"].([]any)) != 0 || m["meta"].(map[string]any)["next_cursor"] != nil {
		t.Fatalf("empty page: %v", m)
	}
	c, w = ctxWith("GET", "/", "")
	RespondCursor(c, NewCursorPage([]int{1}, 1, "tok"), nil)
	if decode(t, w)["meta"].(map[string]any)["next_cursor"] != "tok" {
		t.Fatal("next cursor")
	}
	c, w = ctxWith("GET", "/", "")
	RespondCursor(c, NewCursorPage[int](nil, 1, ""), NotFound("x"))
	if w.Code != 404 {
		t.Fatal("cursor error")
	}

	var in struct{ A int }
	c, _ = ctxWith("POST", "/", `{"A":2}`)
	if !Bind(c, &in) || in.A != 2 {
		t.Fatal("bind")
	}
	c, w = ctxWith("POST", "/", `{`)
	if Bind(c, &in) || w.Code != 400 {
		t.Fatal("malformed body is a 400")
	}
	id := uuid.New()
	c, _ = ctxWith("GET", "/?x="+id.String(), "")
	c.Params = gin.Params{{Key: "id", Value: id.String()}}
	if got, ok := ParamUUID(c, "id"); !ok || got != id {
		t.Fatal("param uuid")
	}
	if got, ok := QueryUUID(c, "x"); !ok || got == nil || *got != id {
		t.Fatal("query uuid")
	}
	if got, ok := QueryUUID(c, "absent"); !ok || got != nil {
		t.Fatal("absent query uuid")
	}
	c, w = ctxWith("GET", "/?x=nope", "")
	c.Params = gin.Params{{Key: "id", Value: "nope"}}
	if _, ok := ParamUUID(c, "id"); ok || w.Code != 400 {
		t.Fatal("bad param")
	}
	c, w = ctxWith("GET", "/?x=nope", "")
	if _, ok := QueryUUID(c, "x"); ok || w.Code != 400 {
		t.Fatal("bad query")
	}
}

func TestIfMatchVersion(t *testing.T) {
	for in, want := range map[string]int{`"3"`: 3, `W/"12"`: 12, `7`: 7, `abc`: 0, ``: 0, `"1x"`: 0} {
		c, _ := ctxWith("PATCH", "/", "")
		if in != "" {
			c.Request.Header.Set("If-Match", in)
		}
		if got := IfMatchVersion(c); got != want {
			t.Errorf("If-Match %q = %d, want %d", in, got, want)
		}
	}
}

func TestDateAndStringList(t *testing.T) {
	d, err := ParseDate("2026-12-01")
	if err != nil || d.String() != "2026-12-01" {
		t.Fatal(err)
	}
	if _, err = ParseDate("01/12/2026"); err == nil {
		t.Fatal("bad date")
	}
	raw, _ := json.Marshal(d)
	var back Date
	if json.Unmarshal(raw, &back) != nil || back.String() != "2026-12-01" || json.Unmarshal([]byte(`"x"`), &back) == nil || json.Unmarshal([]byte(`1`), &back) == nil {
		t.Fatal("json round trip")
	}
	v, _ := d.Value()
	var scanned Date
	if v != "2026-12-01" || scanned.Scan(d.Time) != nil || scanned.Scan("2026-12-01T00:00:00Z") != nil || scanned.Scan(nil) != nil || scanned.Scan(5) == nil || scanned.Scan("zz") == nil {
		t.Fatal("scan")
	}
	later, _ := ParseDate("2027-01-01")
	if !d.Before(later) || later.Before(d) {
		t.Fatal("before")
	}
	var l StringList
	if l.Scan([]byte(`["a","b"]`)) != nil || len(l) != 2 || l.Scan(`["c"]`) != nil || l.Scan(nil) != nil || len(l) != 0 || l.Scan(1) == nil {
		t.Fatal("string list scan")
	}
	if val, _ := StringList(nil).Value(); val != "[]" {
		t.Fatal("nil list stores an empty array")
	}
}
