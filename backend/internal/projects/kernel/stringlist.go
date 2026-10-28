package kernel

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// StringList is a JSON array of strings stored in a JSONB column.
type StringList []string

// Value stores the list as JSON; nil is stored as an empty array.
func (l StringList) Value() (driver.Value, error) {
	if l == nil {
		return "[]", nil
	}
	raw, err := json.Marshal([]string(l))
	return string(raw), err
}

// Scan reads a JSONB array.
func (l *StringList) Scan(src any) error {
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	case nil:
		*l = StringList{}
		return nil
	default:
		return fmt.Errorf("kernel: cannot scan %T into StringList", src)
	}
	return json.Unmarshal(raw, l)
}
