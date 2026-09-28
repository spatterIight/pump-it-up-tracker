package tracker

import (
	"fmt"
	"math"
)

// Pump It Up scored plays the same way from Zero to XX. Each judgment scores
// points: Perfect 1,000, Great 500, Good 100, Bad -200 and Miss -500 (-300 on
// a hold). From the 51st combo on, a Perfect scores 2,000 and a Great 1,500.
// A row of three notes scores 1.5 times as much, and a row of four twice as
// much. An S or better adds a grade bonus of up to 300,000. The total is
// multiplied by the level / 10 above level 10, by 1.2 for doubles and by 1.2
// in rank mode, then rounded down to a multiple of 100.
//
// A result screen does not show where the combo broke, which misses were on
// holds or how many notes each row had, so the exact score cannot be worked
// out. What can be is the range of scores the judgments and max combo allow,
// which every real result screen in testdata/result-screens.json falls
// within. The documented grade table does not reproduce real result screens,
// so grades are taken as logged. See README.md, "Game versions".

// Prime2Scoring scores and grades plays the way Pump It Up Prime 2 does.
// Its best grade is SS; its S is shown in gold or silver.
var Prime2Scoring ScoringSystem = &legacyScoring{
	name:   "Prime 2",
	grades: []Grade{"SS", "S", "A", "B", "C", "D", "F"},
}

// XXScoring scores and grades plays the way Pump It Up XX does. It scores
// like Prime 2, but renamed the grades: Prime 2's SS, gold S and silver S
// are XX's SSS, SS and S. Having different grades, it is a scoring system of
// its own.
var XXScoring ScoringSystem = &legacyScoring{
	name:   "XX",
	grades: []Grade{"SSS", "SS", "S", "A", "B", "C", "D", "F"},
}

// scoreStep is what scores before Phoenix are rounded down to.
const scoreStep = 100

// legacyScoring is the scoring of the versions before Phoenix. Scores have
// no fixed upper limit, since level and grade bonuses take them past
// 1,000,000, and there are no plates.
type legacyScoring struct {
	name   string
	grades []Grade
}

func (s *legacyScoring) Name() string                        { return s.name }
func (s *legacyScoring) MaxScore() int                       { return 0 }
func (s *legacyScoring) ComputesScores() bool                { return false }
func (s *legacyScoring) ComputeScore(Judgments, int) int     { return 0 }
func (s *legacyScoring) Grades() []Grade                     { return s.grades }
func (s *legacyScoring) Grade(int, *Judgments) (Grade, bool) { return "", false }
func (s *legacyScoring) GradeThresholds() []GradeThreshold   { return nil }
func (s *legacyScoring) HasPlates() bool                     { return false }

func (s *legacyScoring) CheckScore(r Result) error {
	if r.Score%scoreStep != 0 {
		return fmt.Errorf("score %d is not a multiple of %d, as every %s score is; check for a typo", r.Score, scoreStep, s.name)
	}
	if r.Judgments == nil {
		return nil
	}
	lo, hi := legacyScoreRange(r)
	given := "judgments"
	if r.MaxCombo >= 0 {
		given = "judgments and max combo"
	}
	if r.Grade != "" {
		given += " and grade"
	}
	switch {
	case r.Score < lo:
		return fmt.Errorf("score %d is lower than %s allows for its %s, which is at least %d; check for a typo%s", r.Score, s.name, given, lo, octalHint)
	case hi >= 0 && r.Score > hi:
		return fmt.Errorf("score %d is higher than %s allows for its %s, which is at most %d; check for a typo%s", r.Score, s.name, given, hi, octalHint)
	}
	return nil
}

// legacyScoreRange returns the lowest and highest score the scoring before
// Phoenix awards for a result with judgments, whatever its result screen does
// not show. hi is -1 when there is no known limit.
func legacyScoreRange(r Result) (lo, hi int) {
	j, maxCombo, chart := *r.Judgments, r.MaxCombo, r.Chart
	// Perfects and Greats build the combo, Bads and Misses break it, and
	// Goods keep it without adding to it.
	hits, breaks := j.Perfect+j.Great, j.Bad+j.Miss

	// The fewest hits past the 50th of a combo. The longest combo has
	// maxCombo hits. The rest come in at most `breaks` more runs, each of
	// which can hold 50 hits without reaching the bonus.
	var bonusHits int
	if maxCombo >= 0 {
		longest := min(maxCombo, hits)
		bonusHits = max(0, longest-50) + max(0, hits-longest-50*breaks)
	} else {
		bonusHits = max(0, hits-50*(breaks+1))
	}
	// Grade bonuses: 100,000 for an S, which result screens bear out, and up
	// to 300,000 for the best grade. Grades below S have none.
	minBonus, maxBonus := 0, 300_000
	switch {
	case r.Grade == "":
	case r.Grade.Tier() == "s":
		minBonus = 100_000
	default:
		maxBonus = 0
	}

	// Single notes, every Miss off a hold, the least grade bonus and no
	// multiplier.
	lowest := 1000*j.Perfect + 500*j.Great + 100*j.Good - 200*j.Bad - 500*j.Miss + 1000*bonusHits + minBonus
	lo = max(0, lowest) / scoreStep * scoreStep

	// Co-op scoring is not documented.
	if chart.Mode == ModeCoOp {
		return lo, -1
	}
	// Rows of four notes, every hit past the 50th of one combo, every Miss
	// on a hold, the best grade bonus, and every multiplier.
	highest := 2000*j.Perfect + 1000*j.Great + 100*j.Good - 200*j.Bad - 300*j.Miss + 2000*max(0, hits-50) + maxBonus
	multiplier := 1.2 * max(1, float64(chart.Level)/10)
	if chart.Mode == ModeDouble || chart.Mode == ModeDoublePerformance {
		multiplier *= 1.2
	}
	return lo, int(math.Ceil(float64(max(0, highest)) * multiplier))
}
