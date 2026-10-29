package pmclient

// Compact projections keep what a person or a model needs and drop the rest. They exist for token economy: an MCP tool
// result goes straight into a model context, so identifiers, timestamps and nested objects that do not help the next
// decision are removed, long text is truncated with an explicit marker, and lists are capped.

// DefaultTextLimit is the number of characters kept from long text fields (descriptions, comments).
const DefaultTextLimit = 600

// Truncate shortens s to at most n runes and appends a marker that tells the reader (or model) how to get the rest.
func Truncate(s string, n int) string {
	r := []rune(s)
	if n <= 0 || len(r) <= n {
		return s
	}
	return string(r[:n]) + "… [truncated " + itoa(len(r)-n) + " chars; request detail to read all]"
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

func pick(m Map, keys ...string) Map {
	out := Map{}
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil && v != "" {
			out[k] = v
		}
	}
	return out
}

// CompactProject keeps key, name, status and the caller role.
func CompactProject(m Map) Map {
	return pick(m, "id", "key", "name", "status", "visibility", "mode", "my_role", "due_on")
}

// CompactTask keeps the fields that drive decisions. Long descriptions are truncated unless detail is set.
func CompactTask(m Map, detail bool) Map {
	out := pick(m, "id", "key", "title", "priority", "column_id", "column_category", "due_at", "version", "blocked", "overdue", "progress")
	if ids, ok := m["assignee_ids"].([]any); ok && len(ids) > 0 {
		out["assignee_ids"] = ids
	}
	if d, ok := m["description"].(string); ok && d != "" {
		if detail {
			out["description"] = d
		} else {
			out["description"] = Truncate(d, DefaultTextLimit)
		}
	}
	return out
}

// CompactComment keeps author, time and a bounded body.
func CompactComment(m Map, detail bool) Map {
	out := pick(m, "id", "author_user_id", "created_at", "edited_at")
	if b, ok := m["body"].(string); ok {
		if detail {
			out["body"] = b
		} else {
			out["body"] = Truncate(b, DefaultTextLimit)
		}
	}
	return out
}

// CompactAll applies f to every item and keeps at most limit of them; the second result says how many were left out.
func CompactAll(items []Map, limit int, f func(Map) Map) ([]Map, int) {
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}
	out := make([]Map, 0, limit)
	for _, it := range items[:limit] {
		out = append(out, f(it))
	}
	return out, len(items) - limit
}
