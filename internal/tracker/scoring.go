package tracker

import (
	"fmt"
	"slices"
	"strings"
)

// ScoringSystem is how a game version scores and grades a play. Versions
// whose scoring systems put scores on the same scale share personal bests
// across chart links; see docs/development.md, "Adding a game version".
// Systems are compared with ==, so each is a pointer or a value of a
// comparable type; a version list with any other kind is refused.
type ScoringSystem interface {
	// Name names the scoring system in messages ("Phoenix").
	Name() string
	// Scale is the scale the system's scores are on. Scores on the same scale
	// compare, even when their systems grade them differently.
	Scale() *ScoreScale
	// MaxScore is the highest score a chart can award, or 0 when there is no
	// known limit.
	MaxScore() int
	// ComputesScores reports whether ComputeScore works out scores. When it
	// does not, as when the system has no verified formula, a play must give
	// its score.
	ComputesScores() bool
	// ComputeScore works out the score the game awards for a set of judgments
	// and max combo. It is only called when ComputesScores is true.
	ComputeScore(j Judgments, maxCombo int) int
	// CheckScore returns an error when a logged score cannot be right. j is
	// the judgments to check it against, or nil when the play has none or is
	// a stage break, whose result screen does not add up the same way.
	// maxCombo is -1 when not recorded.
	CheckScore(score int, j *Judgments, maxCombo int) error
	// Grades lists every grade the system awards, best first.
	Grades() []Grade
	// Grade returns the grade a result earns, from its score and, when they
	// were recorded, its judgments (nil otherwise). ok is false when the
	// grade cannot be worked out: the system has no verified grade table for
	// the score, or it needs judgments that were not recorded.
	Grade(score int, j *Judgments) (g Grade, ok bool)
	// GradeThresholds lists the grades that follow from the score alone, each
	// with the lowest score that earns it, best first; nil when none do.
	// Progress charts draw them as grade lines.
	GradeThresholds() []GradeThreshold
	// HasPlates reports whether the game awards plates.
	HasPlates() bool
}

// ScoreScale is what a version's scores are measured on. Versions whose
// scoring systems share a scale score plays the same way, so their scores
// compare and personal bests carry across a chart link between them, even
// when they grade the same score differently (Phoenix and Phoenix 2).
type ScoreScale struct {
	// Name names the scale in messages ("Phoenix").
	Name string
}

// phoenixScale is the scale Phoenix scores plays on, out of 1,000,000 from
// the judgments and max combo alone.
var phoenixScale = &ScoreScale{Name: "Phoenix"}

// PhoenixScoring scores and grades plays the way Pump It Up Phoenix does.
var PhoenixScoring ScoringSystem = &phoenixScoring{name: "Phoenix", thresholds: gradeThresholds}

// Phoenix2Scoring scores plays the way Phoenix does, but grades them the way
// Pump It Up Phoenix 2 does: it raised every grade cutoff below AAA.
var Phoenix2Scoring ScoringSystem = &phoenixScoring{name: "Phoenix 2", thresholds: phoenix2Thresholds}

// phoenixScoring is the scoring of Phoenix and of the versions that keep its
// score formula. Only the grade cutoffs differ between them.
type phoenixScoring struct {
	name string
	// thresholds are the grades that follow from the score, best first. A
	// score below the last one's minimum is graded as logged.
	thresholds []GradeThreshold
}

func (s *phoenixScoring) Name() string         { return s.name }
func (s *phoenixScoring) Scale() *ScoreScale   { return phoenixScale }
func (s *phoenixScoring) MaxScore() int        { return MaxScore }
func (s *phoenixScoring) ComputesScores() bool { return true }
func (s *phoenixScoring) ComputeScore(j Judgments, maxCombo int) int {
	return ComputeScore(j, maxCombo)
}
func (s *phoenixScoring) CheckScore(score int, j *Judgments, maxCombo int) error {
	if j == nil || maxCombo < 0 {
		return nil
	}
	return checkScore(score, *j, maxCombo)
}
func (s *phoenixScoring) Grades() []Grade { return phoenixGrades }
func (s *phoenixScoring) Grade(score int, _ *Judgments) (Grade, bool) {
	for _, t := range s.thresholds {
		if score >= t.Min {
			return t.Grade, true
		}
	}
	return "", false
}
func (s *phoenixScoring) GradeThresholds() []GradeThreshold { return slices.Clone(s.thresholds) }
func (s *phoenixScoring) HasPlates() bool                   { return true }

// MaxScore is the highest score a Phoenix chart awards.
const MaxScore = 1_000_000

// Grade is a letter grade, such as "AA+". Which grades exist depends on the
// scoring system.
type Grade string

// GradeThreshold is a grade together with the minimum score that earns it.
type GradeThreshold struct {
	Grade Grade
	Min   int
}

// gradeThresholds lists every Phoenix grade with the minimum score that
// earns it, best first.
var gradeThresholds = []GradeThreshold{
	{"SSS+", 995_000},
	{"SSS", 990_000},
	{"SS+", 985_000},
	{"SS", 980_000},
	{"S+", 975_000},
	{"S", 970_000},
	{"AAA+", 960_000},
	{"AAA", 950_000},
	{"AA+", 925_000},
	{"AA", 900_000},
	{"A+", 825_000},
	{"A", 750_000},
	{"B", 650_000},
	{"C", 550_000},
	{"D", 450_000},
	{"F", 0},
}

// phoenix2Thresholds are the Phoenix 2 grades that follow from the score. It
// awards Phoenix's grades, with the cutoffs below AAA raised. PIU Scores read
// them off the official leaderboards: A+ and up exactly, and A to within a
// few thousand points, as 800,000. Nothing on the leaderboards shows where
// B, C and D start, so a score under 800,000 is graded as logged.
var phoenix2Thresholds = []GradeThreshold{
	{"SSS+", 995_000},
	{"SSS", 990_000},
	{"SS+", 985_000},
	{"SS", 980_000},
	{"S+", 975_000},
	{"S", 970_000},
	{"AAA+", 960_000},
	{"AAA", 950_000},
	{"AA+", 940_000},
	{"AA", 920_000},
	{"A+", 900_000},
	{"A", 800_000},
}

var phoenixGrades = func() []Grade {
	gs := make([]Grade, len(gradeThresholds))
	for i, t := range gradeThresholds {
		gs[i] = t.Grade
	}
	return gs
}()

// GradeForScore returns the grade a score earns in Phoenix.
func GradeForScore(score int) Grade {
	g, _ := PhoenixScoring.Grade(score, nil)
	return g
}

// GradeThresholds returns every Phoenix grade with its minimum score, best
// first.
func GradeThresholds() []GradeThreshold { return slices.Clone(gradeThresholds) }

// parseGrade reads a grade as written in the data file, if the scoring
// system awards it.
func parseGrade(sys ScoringSystem, s string) (Grade, bool) {
	g := Grade(strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", "")))
	if slices.Contains(sys.Grades(), g) {
		return g, true
	}
	return "", false
}

// GradeRank orders the grades of a scoring system: a higher rank is a
// better grade, and 0 means no grade. Grades of different scoring systems
// are not comparable.
func GradeRank(sys ScoringSystem, g Grade) int {
	gs := sys.Grades()
	if i := slices.Index(gs, g); i >= 0 {
		return len(gs) - i
	}
	return 0
}

// Tier groups grades for styling: "s", "a", "b" (B to D) or "f".
func (g Grade) Tier() string {
	switch {
	case strings.HasPrefix(string(g), "S"):
		return "s"
	case strings.HasPrefix(string(g), "A"):
		return "a"
	case g == "F":
		return "f"
	default:
		return "b"
	}
}

// Plate is the clear lamp awarded under the grade, e.g. "TALENTED GAME".
type Plate string

// Plates, best first.
var plates = []struct {
	Plate Plate
	Name  string
}{
	{"PG", "Perfect Game"},
	{"UG", "Ultimate Game"},
	{"EG", "Extreme Game"},
	{"SG", "Superb Game"},
	{"MG", "Marvelous Game"},
	{"TG", "Talented Game"},
	{"FG", "Fair Game"},
	{"RG", "Rough Game"},
}

func parsePlate(s string) (Plate, bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
		}
	}
	norm := b.String()
	for _, p := range plates {
		if norm == string(p.Plate) || norm == strings.ToUpper(strings.ReplaceAll(p.Name, " ", "")) {
			return p.Plate, true
		}
	}
	return "", false
}

// Name is the full name of the plate ("Talented Game").
func (p Plate) Name() string {
	for _, x := range plates {
		if x.Plate == p {
			return x.Name
		}
	}
	return ""
}

// Rank orders plates: a higher rank is a better plate, 0 means no plate.
func (p Plate) Rank() int {
	for i, x := range plates {
		if x.Plate == p {
			return len(plates) - i
		}
	}
	return 0
}

// Judgments are the per-note judgment counts from the result screen.
type Judgments struct {
	Perfect, Great, Good, Bad, Miss int
}

// Notes is the number of judged notes on the chart.
func (j Judgments) Notes() int { return j.Perfect + j.Great + j.Good + j.Bad + j.Miss }

// Misses counts the judgments that break combo (Bad and Miss).
func (j Judgments) Misses() int { return j.Bad + j.Miss }

// PerfectRate is the share of notes judged Perfect, from 0 to 100.
func (j Judgments) PerfectRate() float64 {
	if j.Notes() == 0 {
		return 0
	}
	return float64(j.Perfect) * 100 / float64(j.Notes())
}

// ComputeScore returns the Phoenix score for a set of judgments and max combo:
//
//	1,000,000 × (0.995 × (Perfect + 0.6 Great + 0.2 Good + 0.1 Bad) + 0.005 × MaxCombo) / Notes
//
// Integer arithmetic keeps the result exact; the game truncates.
func ComputeScore(j Judgments, maxCombo int) int {
	notes := j.Notes()
	if notes == 0 {
		return 0
	}
	weighted := 10*j.Perfect + 6*j.Great + 2*j.Good + j.Bad
	return 100 * (995*weighted + 50*maxCombo) / notes
}

// scoreTolerance allows for rounding differences between ComputeScore and the
// value printed by the cabinet.
const scoreTolerance = 1

func checkScore(score int, j Judgments, maxCombo int) error {
	expected := ComputeScore(j, maxCombo)
	if diff := score - expected; diff > scoreTolerance || diff < -scoreTolerance {
		return fmt.Errorf("score %d does not match the judgments and max combo, which give %d; check for a typo%s", score, expected, octalHint)
	}
	return nil
}
