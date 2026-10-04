package validators

import (
	"errors"
	"regexp"
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
