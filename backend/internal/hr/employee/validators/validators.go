package validators

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var codeRegex = regexp.MustCompile(`^[A-Z0-9-]{2,32}$`)

var validStatuses = map[string]bool{
	"active":     true,
	"probation":  true,
	"notice":     true,
	"terminated": true,
	"resigned":   true,
	"on_leave":   true,
	"inactive":   true,
}

var validEmploymentTypes = map[string]bool{
	"full_time": true,
	"part_time": true,
	"contract":  true,
	"intern":    true,
}

func ValidateEmployeeCode(code string) error {
	code = strings.TrimSpace(strings.ToUpper(code))
	if !codeRegex.MatchString(code) {
		return errors.New("invalid employee code format: must be 2-32 uppercase alphanumeric chars or hyphens")
	}
	return nil
}

func ValidateStatus(status string) error {
	if !validStatuses[strings.ToLower(status)] {
		return errors.New("invalid status: must be active, probation, notice, terminated, resigned, on_leave, or inactive")
	}
	return nil
}

func ValidateEmploymentType(empType string) error {
	if !validEmploymentTypes[strings.ToLower(empType)] {
		return errors.New("invalid employment type: must be full_time, part_time, contract, or intern")
	}
	return nil
}

// allowedTransitions is the FR-ED002 state machine. resigned/terminated are terminal.
var allowedTransitions = map[string][]string{
	"probation":  {"active", "notice", "resigned", "terminated", "on_leave", "inactive"},
	"active":     {"notice", "resigned", "terminated", "on_leave", "inactive"},
	"on_leave":   {"active", "notice", "resigned", "terminated", "inactive"},
	"notice":     {"active", "resigned", "terminated", "inactive"},
	"inactive":   {"active", "terminated"},
	"resigned":   {},
	"terminated": {},
}

// ValidateTransition enforces the employee status state machine (from != to).
func ValidateTransition(from, to string) error {
	from, to = strings.ToLower(from), strings.ToLower(to)
	if from == to {
		return nil
	}
	for _, ok := range allowedTransitions[from] {
		if ok == to {
			return nil
		}
	}
	return errors.New("illegal status transition " + from + " -> " + to)
}

// ValidateEmergencyContacts checks FR-EC001: a JSON array of {name, relation, phone} objects (name and phone required).
// An empty string is accepted and means "no contacts".
func ValidateEmergencyContacts(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var items []struct {
		Name     string `json:"name"`
		Relation string `json:"relation"`
		Phone    string `json:"phone"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return errors.New("emergency_contacts must be a JSON array of {name, relation, phone} objects")
	}
	for i, it := range items {
		if strings.TrimSpace(it.Name) == "" || strings.TrimSpace(it.Phone) == "" {
			return errors.New("emergency_contacts[" + strconv.Itoa(i) + "] requires name and phone")
		}
	}
	return nil
}
