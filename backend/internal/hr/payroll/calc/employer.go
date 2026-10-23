package calc

// Employer is the line kind of employer-side contributions (PP-01). Such lines are never part of gross, deductions or net.
const Employer = "employer"

var (
	epsRate        = Money(833) // 8.33% of the PF wage, the pension share of the employer 12%
	edliRate       = Money(50)  // 0.50%
	pfAdminRate    = Money(50)  // 0.50%
	employerESIPct = int64(325) // 3.25%
)

// EmployerPF returns the employer PF lines on the earned BASIC (PP-01): EPF is the 12% total minus EPS, so the three
// pieces always add up to exactly the rounded 12%. Wages above the ceiling are capped like the employee share (PY-005).
func EmployerPF(earnedBasic Money) []Line {
	base := Min(earnedBasic, pfWageCeiling)
	if base <= 0 {
		return nil
	}
	total := base.Percent(pfRate).RoundRupee()
	eps := Min(base.Percent(epsRate).RoundRupee(), total)
	return []Line{
		{Code: "EPF_ER", Name: "Employer PF (EPF)", Kind: Employer, Amount: total - eps},
		{Code: "EPS_ER", Name: "Employer pension (EPS)", Kind: Employer, Amount: eps},
		{Code: "EDLI_ER", Name: "Employer EDLI", Kind: Employer, Amount: base.Percent(edliRate).RoundRupee()},
		{Code: "PFADM_ER", Name: "PF admin charges", Kind: Employer, Amount: base.Percent(pfAdminRate).RoundRupee()},
	}
}

// EmployerESI is 3.25% of earned gross rounded up to the rupee when full-month gross is within the ceiling (PP-01),
// computed exactly like the employee share (PY-006).
func EmployerESI(fullGross, earnedGross Money) Money {
	if fullGross > esiGrossCeiling || earnedGross <= 0 {
		return 0
	}
	const den = int64(10000 * 100)
	return Rupees((int64(earnedGross)*employerESIPct + den - 1) / den)
}
