package planning

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/google/uuid"
	"github.com/jaas/jaas/internal/projects/kernel"
)

// Allow-lists for saved views (VW-06): values outside these sets are refused, never stored.
var (
	viewLayouts    = []string{"board", "list", "calendar", "gantt"}
	viewGroupings  = []string{"", "column", "assignee", "priority", "milestone", "sprint"}
	viewSortFields = []string{"rank", "created", "updated", "due", "priority", "number", "title"}
	viewPriorities = []string{"none", "low", "medium", "high", "urgent"}
	viewCategories = []string{"backlog", "todo", "in_progress", "done", "canceled"}
)

const (
	maxFilterList   = 50
	maxSortKeys     = 3
	maxViewJSONSize = 8 * 1024
)

// decodeStrict decodes raw into dst and refuses unknown fields and trailing data.
func decodeStrict(raw json.RawMessage, dst any, field string) error {
	if len(raw) > maxViewJSONSize {
		return kernel.Invalid(field, "is too large")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil || dec.More() {
		return kernel.Invalid(field, "has an unknown or malformed entry")
	}
	return nil
}

// normaliseFilters validates a filters document and returns its canonical encoding.
func normaliseFilters(raw json.RawMessage) (RawJSON, error) {
	var f ViewFilters
	if len(raw) > 0 && string(raw) != "null" {
		if err := decodeStrict(raw, &f, "filters"); err != nil {
			return nil, err
		}
	}
	if err := f.check(); err != nil {
		return nil, err
	}
	out, err := json.Marshal(f)
	return RawJSON(out), err
}

func (f ViewFilters) check() error {
	switch {
	case len(f.ColumnIDs) > maxFilterList || len(f.AssigneeIDs) > maxFilterList || len(f.LabelIDs) > maxFilterList:
		return kernel.Invalid("filters", "a list holds at most 50 ids")
	case !subset(f.Categories, viewCategories):
		return kernel.Invalid("filters.categories", "contains an unknown category")
	case !subset(f.Priorities, viewPriorities):
		return kernel.Invalid("filters.priorities", "contains an unknown priority")
	case tooLong(f.Query, maxSearchQuery):
		return kernel.Invalid("filters.q", "is too long")
	case f.DueAfter != nil && f.DueBefore != nil && f.DueBefore.Before(*f.DueAfter):
		return kernel.Invalid("filters.due_before", "must not be before due_after")
	}
	if f.Sprint != "" && f.Sprint != "none" && f.Sprint != "active" {
		if _, err := uuid.Parse(f.Sprint); err != nil {
			return kernel.Invalid("filters.sprint", "must be none, active or a sprint id")
		}
	}
	return nil
}

// subset reports whether every value is in the allowed set and the list is short.
func subset(vals, allowed []string) bool {
	if len(vals) > len(allowed) {
		return false
	}
	for _, v := range vals {
		if !slices.Contains(allowed, v) {
			return false
		}
	}
	return true
}

// normaliseSort validates the sort keys and returns their canonical encoding (an empty list is "[]").
func normaliseSort(raw json.RawMessage) (RawJSON, error) {
	keys := []SortKey{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := decodeStrict(raw, &keys, "sort"); err != nil {
			return nil, err
		}
	}
	if len(keys) > maxSortKeys {
		return nil, kernel.Invalid("sort", "at most 3 keys")
	}
	for i := range keys {
		if keys[i].Dir == "" {
			keys[i].Dir = "asc"
		}
		if !slices.Contains(viewSortFields, keys[i].Field) || (keys[i].Dir != "asc" && keys[i].Dir != "desc") {
			return nil, kernel.Invalid("sort", "has an unknown field or direction")
		}
	}
	out, err := json.Marshal(keys)
	return RawJSON(out), err
}

// apply copies the set fields onto v after validating them (shared/project rules are checked by the service).
func (in ViewInput) apply(v *SavedView) error {
	var err error
	if in.Name != nil {
		if v.Name, err = text("name", *in.Name, 1, maxViewName); err != nil {
			return err
		}
	}
	if in.Layout != nil {
		if !slices.Contains(viewLayouts, *in.Layout) {
			return kernel.Invalid("layout", "must be board, list, calendar or gantt")
		}
		v.Layout = *in.Layout
	}
	if in.Grouping != nil {
		if !slices.Contains(viewGroupings, *in.Grouping) {
			return kernel.Invalid("grouping", "must be column, assignee, priority, milestone or sprint")
		}
		v.Grouping = *in.Grouping
	}
	if in.Filters != nil {
		if v.Filters, err = normaliseFilters(in.Filters); err != nil {
			return err
		}
	}
	if in.Sort != nil {
		if v.Sort, err = normaliseSort(in.Sort); err != nil {
			return err
		}
	}
	return nil
}
