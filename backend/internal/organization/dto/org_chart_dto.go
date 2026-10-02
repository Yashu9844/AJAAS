package dto

// OrgChartNode is one department node with nested children, teams, and headcount. FR-O001.
type OrgChartNode struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Code      *string        `json:"code,omitempty"`
	Status    string         `json:"status"`
	Headcount int64          `json:"headcount"`
	Teams     []TeamResponse `json:"teams"`
	Children  []OrgChartNode `json:"children"`
}

// OrgChartResponse is the tenant forest (roots only at top level).
type OrgChartResponse struct {
	Data []OrgChartNode `json:"data"`
}

// UserChainResponse is a user's chain to root plus direct reports. FR-O002.
type UserChainResponse struct {
	UserID        string               `json:"user_id"`
	Mapping       *MappingResponse     `json:"mapping,omitempty"`
	Team          *TeamResponse        `json:"team,omitempty"`
	Department    *DepartmentResponse  `json:"department,omitempty"`
	Ancestors     []DepartmentResponse `json:"ancestors"`
	DirectReports []string             `json:"direct_reports"`
}
