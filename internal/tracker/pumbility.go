package tracker

import (
	"math"
	"slices"
	"time"
)

// PUMBILITY is the game's rating of a player since Phoenix: the sum of the
// values of their fifty most valuable charts, singles and doubles together.
// A chart is worth more the higher its level and the better the grade of
// its best clear. Each version prices charts its own way.

// PumbilityPool is how many charts PUMBILITY adds up.
const PumbilityPool = 50

// PumbilityFormula prices the charts of a version for its PUMBILITY.
type PumbilityFormula interface {
	// Value is what a clear of chart c adds to PUMBILITY, from its score, its
	// grade ("" when it cannot be worked out and none was logged) and its
	// plate ("" when none was logged). It is 0 for a chart that does not
	// count: co-op, performance, and anything below level 10.
	Value(c Chart, score int, g Grade, p Plate) float64
}

// counts reports whether a chart can count towards PUMBILITY at all.
func counts(c Chart) bool {
	return (c.Mode == ModeSingle || c.Mode == ModeDouble) && c.Level >= 10
}

// cents rounds a value the way the game shows it.
func cents(v float64) float64 { return math.Round(v*100) / 100 }

// PhoenixPumbility prices charts the way Phoenix does: a chart of level 10
// or more is worth 100 + 5·(level − 10)·(level − 9), times a multiplier for
// its grade that is 1 for AA. Plates do not count.
var PhoenixPumbility PumbilityFormula = phoenixPumbility{}

type phoenixPumbility struct{}

var phoenixPumbilityGrades = map[Grade]float64{
	"SSS+": 1.50, "SSS": 1.44, "SS+": 1.38, "SS": 1.32, "S+": 1.26, "S": 1.20,
	"AAA+": 1.15, "AAA": 1.10, "AA+": 1.05, "AA": 1.00, "A+": 0.90, "A": 0.80,
	"B": 0.70, "C": 0.60, "D": 0.50, "F": 0.40,
}

func (phoenixPumbility) Value(c Chart, score int, g Grade, _ Plate) float64 {
	if !counts(c) {
		return 0
	}
	if g == "" {
		g = GradeForScore(score)
	}
	return cents(float64(100+5*(c.Level-10)*(c.Level-9)) * phoenixPumbilityGrades[g])
}

// Phoenix2Pumbility prices charts the way Phoenix 2 does, as PIU Scores
// worked it out from the official breakdown of players' PUMBILITY. A double
// of level 10 or more is worth 130 + 5·level, plus 5 more for each level
// over 24, times the sum of a multiplier for its grade and a small bonus
// for its plate. A single is priced as a double one level higher, and its
// grades and plates are priced a little differently.
var Phoenix2Pumbility PumbilityFormula = phoenix2Pumbility{}

type phoenix2Pumbility struct{}

// phoenix2PumbilityGrades and phoenix2PumbilityPlates price doubles first,
// then singles.
var phoenix2PumbilityGrades = map[Grade][2]float64{
	"SSS+": {1.50, 1.50}, "SSS": {1.49, 1.49}, "SS+": {1.48, 1.48}, "SS": {1.47, 1.47}, "S+": {1.46, 1.46}, "S": {1.45, 1.45},
	"AAA+": {1.43, 1.43}, "AAA": {1.41, 1.41}, "AA+": {1.39, 1.39}, "AA": {1.37, 1.36}, "A+": {1.35, 1.33}, "A": {1.30, 1.28},
	"B": {1.25, 1.20}, "C": {1.20, 1.10}, "D": {1.10, 1.00}, "F": {1.00, 0.90},
}

var phoenix2PumbilityPlates = map[Plate][2]float64{
	"PG": {0.020, 0.020}, "UG": {0.016, 0.017}, "EG": {0.012, 0.014}, "SG": {0.008, 0.008},
	"MG": {0.006, 0.006}, "TG": {0.004, 0.004}, "FG": {0.002, 0.002}, "RG": {0, 0},
}

func (phoenix2Pumbility) Value(c Chart, score int, g Grade, p Plate) float64 {
	if !counts(c) {
		return 0
	}
	level, i := c.Level, 0
	if c.Mode == ModeSingle {
		level, i = level+1, 1
	}
	if g == "" {
		g = phoenix2GradeBelowA(score)
	}
	base := 130 + 5*level + 5*max(0, level-24)
	return cents(float64(base) * (phoenix2PumbilityGrades[g][i] + phoenix2PumbilityPlates[p][i]))
}

// phoenix2GradeBelowA is the grade PUMBILITY is worked out with for a
// Phoenix 2 score under 800,000 logged without one. Where B, C and D start
// is not known; PIU Scores takes 700,000, 600,000 and 500,000.
func phoenix2GradeBelowA(score int) Grade {
	switch {
	case score >= 700_000:
		return "B"
	case score >= 600_000:
		return "C"
	case score >= 500_000:
		return "D"
	}
	return "F"
}

// Pumbility is a player's PUMBILITY in one version, from the plays logged.
type Pumbility struct {
	Version *Version
	// Total is the sum of the values of the charts in the pool.
	Total float64
	// Charts are the cleared charts that count, most valuable first. The
	// first PumbilityPool of them are the pool.
	Charts []*RatedChart
	// History is the total at the end of each day with a play in the
	// version, oldest first.
	History []RatingPoint
}

// RatedChart is what a chart is worth towards PUMBILITY.
type RatedChart struct {
	History *ChartHistory
	// Best is the play the chart is valued by: its highest-scoring clear in
	// the version.
	Best *Play
	// Grade is the grade the chart is valued at: Best's, or the one PIU
	// Scores takes for its score when it has none, which Estimated reports.
	Grade     Grade
	Estimated bool
	Value     float64
	// Rank is the chart's place by value, from 1.
	Rank int
	// InPool reports whether the chart is one of the fifty that count.
	InPool bool
	// Next is the next grade up, "" when there is none, NextMin the score
	// that earns it, and Gain what earning it would add to the total.
	Next    Grade
	NextMin int
	Gain    float64
}

// RatingPoint is the total on a day.
type RatingPoint struct {
	Date  time.Time
	Total float64
}

// Pool returns the charts that count, most valuable first.
func (p *Pumbility) Pool() []*RatedChart { return p.Charts[:min(PumbilityPool, len(p.Charts))] }

// Full reports whether the pool holds PumbilityPool charts, so that a new
// chart only counts by pushing out the least valuable one.
func (p *Pumbility) Full() bool { return len(p.Charts) >= PumbilityPool }

// Cut is the value a chart has to beat to get into a full pool: that of the
// least valuable chart in it. It is 0 while the pool is not full.
func (p *Pumbility) Cut() float64 {
	if !p.Full() {
		return 0
	}
	return p.Charts[PumbilityPool-1].Value
}

// TotalOn returns the total at the end of the given day, 0 before the first
// play.
func (p *Pumbility) TotalOn(day time.Time) float64 {
	total := 0.0
	for _, pt := range p.History {
		if pt.Date.After(day) {
			break
		}
		total = pt.Total
	}
	return total
}

// Gain returns how much a clear worth value would add to the total.
func (p *Pumbility) Gain(value float64) float64 { return cents(max(0, value-p.Cut())) }

// Pumbility works out the player's PUMBILITY in version v from its plays,
// nil when the version has none.
func (t *Tracker) Pumbility(v *Version) *Pumbility {
	f := v.Pumbility
	if f == nil {
		return nil
	}
	out := &Pumbility{Version: v}
	for _, song := range t.Songs {
		for _, h := range song.ChartsIn(v) {
			best := bestClear(f, h.Plays)
			if best == nil || !counts(h.Chart) {
				continue
			}
			rc := &RatedChart{History: h, Best: best, Grade: best.Grade}
			if rc.Grade == "" {
				rc.Grade, rc.Estimated = estimatedGrade(v, best.Score), true
			}
			rc.Value = f.Value(h.Chart, best.Score, best.Grade, best.Plate)
			out.Charts = append(out.Charts, rc)
		}
	}
	slices.SortStableFunc(out.Charts, func(a, b *RatedChart) int {
		switch {
		case a.Value != b.Value:
			if a.Value > b.Value {
				return -1
			}
			return 1
		case a.Best.Score != b.Best.Score:
			return b.Best.Score - a.Best.Score
		}
		return a.Best.Date.Compare(b.Best.Date)
	})
	for i, rc := range out.Charts {
		rc.Rank = i + 1
		rc.InPool = i < PumbilityPool
		if rc.InPool {
			out.Total += rc.Value
		}
	}
	out.Total = cents(out.Total)
	for _, rc := range out.Charts {
		rc.Next, rc.NextMin = nextGrade(v.Scoring, rc.Grade, rc.Estimated)
		if rc.Next == "" {
			continue
		}
		value := f.Value(rc.History.Chart, rc.NextMin, rc.Next, rc.Best.Plate)
		if rc.InPool {
			rc.Gain = cents(value - rc.Value)
		} else {
			rc.Gain = out.Gain(value)
		}
	}
	out.History = pumbilityHistory(f, t.Plays, v)
	return out
}

// bestClear returns the highest-scoring clear among plays, the most
// valuable one of those that tie, or nil when none was cleared.
func bestClear(f PumbilityFormula, plays []*Play) *Play {
	var best *Play
	for _, p := range plays {
		if p.Broken || !p.HasScore() {
			continue
		}
		if best == nil || p.Score > best.Score ||
			p.Score == best.Score && f.Value(p.Chart, p.Score, p.Grade, p.Plate) > f.Value(best.Chart, best.Score, best.Grade, best.Plate) {
			best = p
		}
	}
	return best
}

// estimatedGrade is the grade a score is valued at when its version could
// not work it out and none was logged.
func estimatedGrade(v *Version, score int) Grade {
	if v.Pumbility == Phoenix2Pumbility {
		return phoenix2GradeBelowA(score)
	}
	return ""
}

// nextGrade returns the grade above g that the score alone earns, and the
// lowest score that earns it, or "" when there is none. An estimated grade
// is below every grade the score is known to earn.
func nextGrade(sys ScoringSystem, g Grade, estimated bool) (Grade, int) {
	ts := sys.GradeThresholds()
	if len(ts) == 0 {
		return "", 0
	}
	if estimated {
		last := ts[len(ts)-1]
		return last.Grade, last.Min
	}
	for i, t := range ts {
		if t.Grade == g && i > 0 {
			return ts[i-1].Grade, ts[i-1].Min
		}
	}
	return "", 0
}

// pumbilityHistory works out the total at the end of each day with a play
// in version v. plays are in chronological order.
func pumbilityHistory(f PumbilityFormula, plays []*Play, v *Version) []RatingPoint {
	best := map[*ChartHistory]*Play{}
	var out []RatingPoint
	flush := func(day time.Time) {
		values := make([]float64, 0, len(best))
		for h, p := range best {
			values = append(values, f.Value(h.Chart, p.Score, p.Grade, p.Plate))
		}
		slices.SortFunc(values, func(a, b float64) int {
			if a > b {
				return -1
			}
			if a < b {
				return 1
			}
			return 0
		})
		total := 0.0
		for _, x := range values[:min(PumbilityPool, len(values))] {
			total += x
		}
		out = append(out, RatingPoint{Date: day, Total: cents(total)})
	}
	var day time.Time
	for _, p := range plays {
		if p.Version != v {
			continue
		}
		y, m, d := p.Date.Date()
		pDay := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		if !day.IsZero() && !pDay.Equal(day) {
			flush(day)
		}
		day = pDay
		if !counts(p.Chart) {
			continue
		}
		candidates := []*Play{p}
		if b := best[p.History]; b != nil {
			candidates = append(candidates, b)
		}
		if b := bestClear(f, candidates); b != nil {
			best[p.History] = b
		}
	}
	if !day.IsZero() {
		flush(day)
	}
	return out
}
