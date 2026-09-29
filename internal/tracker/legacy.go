package tracker

import "fmt"

// Pump It Up scored plays the same way from Zero to XX, rounding every score
// down to a multiple of 100. The exact score cannot be worked out from a
// result screen, since it depends on where the combo broke and on which
// misses were on holds, and the documented grade table does not reproduce
// real result screens. So scores and grades are taken as logged, and only
// the rounding is checked. See docs/game-versions.md.

// Prime2Scoring scores and grades plays the way Pump It Up Prime 2 does.
// Its best grade is SS; its S is shown in gold or silver.
var Prime2Scoring ScoringSystem = &legacyScoring{
	name:   "Prime 2",
	grades: []Grade{"SS", "S", "A", "B", "C", "D", "F"},
	scale:  ScoreScale{Name: "Prime 2"},
}

// XXScoring scores and grades plays the way Pump It Up XX does. It scores
// like Prime 2, but renamed the grades: Prime 2's SS, gold S and silver S
// are XX's SSS, SS and S. Having different grades, it is a scoring system of
// its own.
var XXScoring ScoringSystem = &legacyScoring{
	name:   "XX",
	grades: []Grade{"SSS", "SS", "S", "A", "B", "C", "D", "F"},
	scale:  ScoreScale{Name: "XX"},
}

// scoreStep is what scores before Phoenix are rounded down to.
const scoreStep = 100

// legacyScoring is the scoring of the versions before Phoenix. Scores have
// no fixed upper limit, since level and grade bonuses take them past
// 1,000,000, and there are no plates. Each version keeps a scale of its own:
// a score depends on the chart's level, which is often re-rated between
// versions, so the same steps can score differently in the next one.
type legacyScoring struct {
	name   string
	grades []Grade
	scale  ScoreScale
}

func (s *legacyScoring) Name() string                        { return s.name }
func (s *legacyScoring) Scale() *ScoreScale                  { return &s.scale }
func (s *legacyScoring) MaxScore() int                       { return 0 }
func (s *legacyScoring) ComputesScores() bool                { return false }
func (s *legacyScoring) ComputeScore(Judgments, int) int     { return 0 }
func (s *legacyScoring) Grades() []Grade                     { return s.grades }
func (s *legacyScoring) Grade(int, *Judgments) (Grade, bool) { return "", false }
func (s *legacyScoring) GradeThresholds() []GradeThreshold   { return nil }
func (s *legacyScoring) HasPlates() bool                     { return false }

func (s *legacyScoring) CheckScore(score int, _ *Judgments, _ int) error {
	if score%scoreStep != 0 {
		return fmt.Errorf("score %d is not a multiple of %d, as every %s score is; check for a typo", score, scoreStep, s.name)
	}
	return nil
}
