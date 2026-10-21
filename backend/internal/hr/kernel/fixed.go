package kernel

// Fixed is a 2-decimal quantity that is not a day count (FTE, headcount) with the same exact codec as Days
// (hundredths, NUMERIC, JSON number).
type Fixed = Days

// ParseFixed parses a 2-decimal quantity; see ParseDays.
var ParseFixed = ParseDays

// One is 1.00.
const One = Fixed(100)
