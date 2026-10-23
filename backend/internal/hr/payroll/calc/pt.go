package calc

import (
	"sort"
	"strings"
)

// PTSlab is one professional-tax slab (PP-02): amount applies when MinGross ≤ earned gross ≤ MaxGross (MaxGross 0 =
// unbounded); a non-zero Month makes it apply to that calendar month only and wins over the generic slab.
type PTSlab struct {
	MinGross, MaxGross, Amount Money
	Month                      int
}

// PTFromSlabs picks the slab for the month and earned gross; a month-specific slab beats the generic one (PP-02).
func PTFromSlabs(slabs []PTSlab, month int, earnedGross Money) Money {
	var generic *PTSlab
	for i := range slabs {
		s := &slabs[i]
		if earnedGross < s.MinGross || (s.MaxGross > 0 && earnedGross > s.MaxGross) {
			continue
		}
		switch s.Month {
		case month:
			return s.Amount
		case 0:
			generic = s
		}
	}
	if generic != nil {
		return generic.Amount
	}
	return 0
}

// ValidatePTSlabs enforces ascending, non-overlapping slabs per series (the generic series and each month override),
// with the generic series starting at 0 (PP-09).
func ValidatePTSlabs(slabs []PTSlab) error {
	if len(slabs) == 0 || len(slabs) > 20 {
		return invalid("a state needs 1-20 slabs")
	}
	generic := false
	for _, month := range monthsOf(slabs) {
		var series []PTSlab
		for _, s := range slabs {
			if s.Month == month {
				series = append(series, s)
			}
		}
		sort.Slice(series, func(i, j int) bool { return series[i].MinGross < series[j].MinGross })
		if month == 0 {
			generic = true
			if series[0].MinGross != 0 {
				return invalid("the first generic slab must start at 0")
			}
		}
		for i, s := range series {
			switch {
			case month < 0 || month > 12:
				return invalid("slab month must be 1-12")
			case s.Amount < 0 || s.MinGross < 0 || (s.MaxGross != 0 && s.MaxGross < s.MinGross):
				return invalid("slab %d has an invalid range or amount", i+1)
			case i > 0 && (series[i-1].MaxGross == 0 || s.MinGross <= series[i-1].MaxGross):
				return invalid("slabs overlap at %s", s.MinGross)
			}
		}
	}
	if !generic {
		return invalid("a state needs generic slabs")
	}
	return nil
}

func monthsOf(slabs []PTSlab) []int {
	seen := map[int]bool{}
	var out []int
	for _, s := range slabs {
		if !seen[s.Month] {
			seen[s.Month] = true
			out = append(out, s.Month)
		}
	}
	sort.Ints(out)
	return out
}

// DefaultPTSlabs returns built-in monthly slabs for a state, or nil. They are configuration data that must be checked
// against current state law before production use (PP-02); tenants override them per state.
func DefaultPTSlabs(state string) []PTSlab {
	r := Rupees
	switch strings.ToUpper(state) {
	case "KA":
		return []PTSlab{{MinGross: 0, MaxGross: r(24999), Amount: 0}, {MinGross: r(25000), Amount: r(200)}, {MinGross: r(25000), Amount: r(300), Month: 2}}
	case "MH":
		return []PTSlab{{MinGross: 0, MaxGross: r(7500), Amount: 0}, {MinGross: r(7501), MaxGross: r(10000), Amount: r(175)}, {MinGross: r(10001), Amount: r(200)},
			{MinGross: r(10001), Amount: r(300), Month: 2}}
	case "WB":
		return []PTSlab{{MinGross: 0, MaxGross: r(10000), Amount: 0}, {MinGross: r(10001), MaxGross: r(15000), Amount: r(110)},
			{MinGross: r(15001), MaxGross: r(25000), Amount: r(130)}, {MinGross: r(25001), MaxGross: r(40000), Amount: r(150)}, {MinGross: r(40001), Amount: r(200)}}
	case "GJ":
		return []PTSlab{{MinGross: 0, MaxGross: r(12000), Amount: 0}, {MinGross: r(12001), Amount: r(200)}}
	case "AP", "TS":
		return []PTSlab{{MinGross: 0, MaxGross: r(15000), Amount: 0}, {MinGross: r(15001), MaxGross: r(20000), Amount: r(150)}, {MinGross: r(20001), Amount: r(200)}}
	}
	return nil
}
