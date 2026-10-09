package services

import (
	"encoding/json"

	"github.com/jaas/jaas/internal/payroll/calc"
	"github.com/jaas/jaas/internal/payroll/dto"
	"github.com/jaas/jaas/internal/payroll/events"
	"github.com/jaas/jaas/internal/payroll/models"
	"github.com/jaas/jaas/internal/payroll/validators"
)

func mapStructure(s *models.Structure, comps []models.Component) dto.StructureResponse {
	out := dto.StructureResponse{ID: s.ID, Name: s.Name, Description: s.Description, PFEnabled: s.PFEnabled, ESIEnabled: s.ESIEnabled,
		PTEnabled: s.PTEnabled, TDSEnabled: s.TDSEnabled, Status: s.Status, Components: []dto.ComponentResponse{},
		CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	for _, c := range comps {
		out.Components = append(out.Components, dto.ComponentResponse{Code: c.Code, Name: c.Name, Kind: c.Kind, Calc: c.Calc,
			Value: c.Value, Taxable: c.Taxable, Position: c.Position})
	}
	return out
}

func mapAssignment(a *models.Assignment) dto.AssignmentResponse {
	out := dto.AssignmentResponse{ID: a.ID, EmployeeID: a.EmployeeProfileID, StructureID: a.StructureID, AnnualCTC: a.AnnualCTC,
		EffectiveFrom: a.EffectiveFrom.Format(validators.DateLayout), CreatedAt: a.CreatedAt}
	if a.EffectiveTo != nil {
		to := a.EffectiveTo.Format(validators.DateLayout)
		out.EffectiveTo = &to
	}
	return out
}

func mapBreakdown(monthlyCTC calc.Money, r calc.Result) dto.BreakdownResponse {
	out := dto.BreakdownResponse{MonthlyCTC: monthlyCTC, Gross: r.Gross, Deductions: r.Deductions, Net: r.Net, Lines: []dto.LineResponse{}}
	for _, l := range r.Lines {
		out.Lines = append(out.Lines, dto.LineResponse{Code: l.Code, Name: l.Name, Kind: l.Kind, Amount: l.Amount})
	}
	return out
}

func mapRun(r *models.Run) dto.RunResponse {
	warnings := []string{}
	_ = json.Unmarshal([]byte(r.Warnings), &warnings)
	return dto.RunResponse{ID: r.ID, Month: r.Month, Year: r.Year, Status: r.Status, EmployeeCount: r.EmployeeCount,
		GrossTotal: r.GrossTotal, DeductionTotal: r.DeductionTotal, NetTotal: r.NetTotal, Warnings: warnings,
		CalculatedByUserID: r.CalculatedByUserID, ApprovedByUserID: r.ApprovedByUserID, CalculatedAt: r.CalculatedAt,
		ApprovedAt: r.ApprovedAt, FinalizedAt: r.FinalizedAt, CreatedAt: r.CreatedAt}
}

func mapPayslip(p *models.Payslip, run *models.Run) dto.PayslipResponse {
	out := dto.PayslipResponse{ID: p.ID, RunID: p.RunID, Month: run.Month, Year: run.Year, EmployeeID: p.EmployeeProfileID,
		EmployeeCode: p.EmployeeCode, EmployeeName: p.EmployeeName, StructureID: p.StructureID, AnnualCTC: p.AnnualCTC,
		DaysInMonth: p.DaysInMonth, PayableDays: p.PayableDays, LopDays: p.LopDays, Gross: p.Gross, Deductions: p.Deductions,
		Net: p.Net, Lines: []dto.LineResponse{}, CreatedAt: p.CreatedAt}
	for _, l := range p.Lines {
		out.Lines = append(out.Lines, dto.LineResponse{Code: l.Code, Name: l.Name, Kind: l.Kind, Amount: l.Amount})
	}
	return out
}

func runPayload(r *models.Run) events.RunPayload {
	return events.RunPayload{RunID: r.ID, Year: r.Year, Month: r.Month, Status: r.Status, EmployeeCount: r.EmployeeCount,
		GrossTotal: r.GrossTotal, DeductionTotal: r.DeductionTotal, NetTotal: r.NetTotal}
}
