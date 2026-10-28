package integration

import (
	"testing"
)

// Custom fields end to end: typed definitions per project, values per task with the task version contract (If-Match),
// https-only URLs, user fields limited to project members, and no access for outsiders.
func TestCustomFieldsThroughTheRouter(t *testing.T) {
	tc := newTenant(t)
	owner, _, _ := tc.limitedUser(t, [2]string{"projects", "create"})
	stranger, _, _ := tc.limitedUser(t)
	memberID, _ := tc.newUser(t, "fld")

	pid := tc.asTok(t, owner, "POST", "/projects", body(map[string]any{"key": "FLD" + uniq()[:3], "name": "Fields"})).str("data.id")
	expect(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/members", body(map[string]any{"user_id": memberID, "role": "member"})), 201, "member")
	task := tc.asTok(t, owner, "POST", "/projects/"+pid+"/tasks", body(map[string]any{"title": "With fields"}))
	expect(t, task, 201, "task")
	tid := task.str("data.id")

	expectErr(t, tc.asTok(t, stranger, "GET", "/projects/"+pid+"/custom-fields"), 404, "NOT_FOUND", "outsiders see nothing")
	expectErr(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/custom-fields", body(map[string]any{"name": "Env", "type": "select"})), 400, "VALIDATION_ERROR", "select needs options")
	expect(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/custom-fields", body(map[string]any{"name": "Env", "type": "select", "options": []string{"dev", "prod"}})), 201, "select field")
	expect(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/custom-fields", body(map[string]any{"name": "Spec", "type": "url"})), 201, "url field")
	expect(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/custom-fields", body(map[string]any{"name": "Reviewer", "type": "user"})), 201, "user field")
	expectErr(t, tc.asTok(t, owner, "POST", "/projects/"+pid+"/custom-fields", body(map[string]any{"name": "env", "type": "text"})), 409, "FIELD_NAME_TAKEN", "names are unique per project")

	set := func(tok string, values map[string]any, opts ...opt) resp {
		return tc.asTok(t, tok, "PUT", "/tasks/"+tid+"/custom-fields", append([]opt{body(map[string]any{"values": values})}, opts...)...)
	}
	expect(t, set(owner, map[string]any{"env": "prod", "spec": "https://example.com/spec", "reviewer": memberID}), 200, "set values")
	expectErr(t, set(owner, map[string]any{"spec": "http://example.com"}), 400, "VALIDATION_ERROR", "https only")
	expectErr(t, set(owner, map[string]any{"env": "staging"}), 400, "VALIDATION_ERROR", "not an option")
	expectErr(t, set(owner, map[string]any{"reviewer": "00000000-0000-4000-8000-000000000001"}), 400, "VALIDATION_ERROR", "user must be a member")
	expectErr(t, set(stranger, map[string]any{"env": "dev"}), 404, "NOT_FOUND", "outsiders cannot write")

	read := tc.asTok(t, owner, "GET", "/tasks/"+tid+"/custom-fields")
	if len(read.list("data")) != 3 {
		t.Fatalf("every project field is listed: %s", read.Raw)
	}
	// The task version moved with the values: the creation version is stale now.
	expectErr(t, set(owner, map[string]any{"env": "dev"}, header("If-Match", `"1"`)), 409, "VERSION_CONFLICT", "stale version")
	cur := tc.asTok(t, owner, "GET", "/tasks/"+tid).get("data.version")
	expect(t, set(owner, map[string]any{"env": nil}, header("If-Match", `"`+formatVersion(cur)+`"`)), 200, "clear with the current version")
}

func formatVersion(v any) string {
	if f, ok := v.(float64); ok {
		return itoa(int(f))
	}
	return "0"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
