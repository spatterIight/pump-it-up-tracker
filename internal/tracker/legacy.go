package tracker

import "fmt"

// Pump It Up scored plays the same way from Zero to XX: points per judgment,
// a bonus per note from the 51st combo, grade bonuses, and multipliers for
// high levels and doubles, with the result rounded down to a multiple of 100.
// The exact score cannot be worked out from a result screen, since it
// depends on where the combo broke and on which misses were on hold notes,
// and the documented grade table does not reproduce real result screens.
// So only the rounding is checked, and scores and grades are taken as
// logged. See README.md, "Game versions".

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
// no known upper limit, since level and grade bonuses take them past
// 1,000,000, and there are no plates.
type legacyScoring struct {
	name   string
	grades []Grade
}

func (s *legacyScoring) Name() string                            { return s.name }
func (s *legacyScoring) MaxScore() int                           { return 0 }
func (s *legacyScoring) ComputeScore(Judgments, int) (int, bool) { return 0, false }
func (s *legacyScoring) Grades() []Grade                         { return s.grades }
func (s *legacyScoring) Grade(int, *Judgments) (Grade, bool)     { return "", false }
func (s *legacyScoring) GradeThresholds() []GradeThreshold       { return nil }
func (s *legacyScoring) HasPlates() bool                         { return false }

func (s *legacyScoring) CheckScore(score int, _ *Judgments, _ int) error {
	if score%scoreStep != 0 {
		return fmt.Errorf("score %d is not a multiple of %d, as every %s score is; check for a typo", score, scoreStep, s.name)
	}
	return nil
}
