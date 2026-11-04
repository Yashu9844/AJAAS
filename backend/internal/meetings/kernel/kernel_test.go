package kernel

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ctx(target, ifMatch string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, target, strings.NewReader("{"))
	if ifMatch != "" {
		c.Request.Header.Set("If-Match", ifMatch)
	}
	return c, w
}

func TestTextOneOfAndVersions(t *testing.T) {
	if s, err := Text("t", "  hi  ", 1, 5); err != nil || s != "hi" {
		t.Fatalf("trim: %q %v", s, err)
	}
	if _, err := Text("t", "", 1, 5); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("required: %v", err)
	}
	if _, err := Text("t", "toolong", 0, 3); err == nil {
		t.Fatal("too long")
	}
	if OneOf("k", "a", "a", "b") != nil || OneOf("k", "c", "a", "b") == nil {
		t.Fatal("one of")
	}
	if err := CheckVersion(0, 3); err == nil || !strings.Contains(err.Error(), "VERSION_REQUIRED") {
		t.Fatalf("version required: %v", err)
	}
	v, ok := AsVersionConflict(CheckVersion(2, 3))
	if !ok || v.Current != 3 || !strings.Contains(v.Error(), "version 3") || CheckVersion(3, 3) != nil {
		t.Fatal("conflict")
	}
	if !IsUniqueViolation(gorm.ErrDuplicatedKey) || IsUniqueViolation(nil) || IsUniqueViolation(errors.New("x")) {
		t.Fatal("unique violation")
	}
}

func TestHTTPHelpers(t *testing.T) {
	for raw, want := range map[string]int{`W/"3"`: 3, `"12"`: 12, "x1": 0, "": 0, "1234567890": 0} {
		c, _ := ctx("/", raw)
		if got := IfMatchVersion(c); got != want {
			t.Errorf("If-Match %q: %d", raw, got)
		}
	}
	c, _ := ctx("/", "")
	if Version(c, 4) != 4 {
		t.Fatal("body version is the fallback")
	}
	c, w := ctx("/?from=bad", "")
	if _, ok := QueryTime(c, "from"); ok || w.Code != 400 {
		t.Fatalf("bad time: %d", w.Code)
	}
	c, _ = ctx("/?from=2027-01-01T00:00:00Z", "")
	if v, ok := QueryTime(c, "from"); !ok || v == nil || v.Year() != 2027 {
		t.Fatal("good time")
	}
	c, w = ctx("/", "")
	c.Params = gin.Params{{Key: "id", Value: "nope"}}
	if _, ok := ParamUUID(c, "id"); ok || w.Code != 400 {
		t.Fatal("bad uuid")
	}
	var dst map[string]any
	c, w = ctx("/", "")
	if Bind(c, &dst) || w.Code != 400 {
		t.Fatal("malformed body")
	}
	c, w = ctx("/", "")
	Fail(c, &VersionConflict{Current: 2, Body: "now"})
	if w.Code != 409 || !strings.Contains(w.Body.String(), `"current_body":"now"`) {
		t.Fatalf("conflict body: %s", w.Body.String())
	}
	c, w = ctx("/", "")
	Fail(c, errors.New("secret detail"))
	if w.Code != 500 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("unknown errors never leak: %s", w.Body.String())
	}
	c, w = ctx("/", "")
	NoContent(c, nil)
	if c.Writer.Status() != 204 {
		t.Fatalf("no content: %d", c.Writer.Status())
	}
	c, w = ctx("/", "")
	NoContent(c, Unprocessable("X", "y"))
	if w.Code != 422 {
		t.Fatal("422")
	}
}
