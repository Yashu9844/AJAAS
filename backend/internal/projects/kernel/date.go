package kernel

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// Date is a calendar day without a time zone; it reads and writes DATE columns and JSON as YYYY-MM-DD.
type Date struct{ time.Time }

// ParseDate reads YYYY-MM-DD.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return Date{}, Invalid("date", "must be YYYY-MM-DD")
	}
	return Date{t}, nil
}

// String formats the day.
func (d Date) String() string { return d.Time.Format(dateLayout) }

// MarshalJSON writes the day as a string.
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON reads YYYY-MM-DD.
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = v
	return nil
}

// Value stores the day.
func (d Date) Value() (driver.Value, error) { return d.String(), nil }

// Scan reads a DATE.
func (d *Date) Scan(src any) error {
	switch v := src.(type) {
	case time.Time:
		d.Time = time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC)
	case string:
		p, err := ParseDate(v[:min(len(v), 10)])
		if err != nil {
			return err
		}
		*d = p
	case nil:
		*d = Date{}
	default:
		return fmt.Errorf("kernel: cannot scan %T into Date", src)
	}
	return nil
}

// Before reports whether d is strictly before o.
func (d Date) Before(o Date) bool { return d.Time.Before(o.Time) }
