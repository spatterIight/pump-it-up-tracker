package catalog

// Difficulty is how hard a chart is for its level, as PIU Scores' players
// find: how many pass it, or how well they score on it, against the other
// charts of the level.
type Difficulty int

// Difficulties, easiest first. PIU Scores calls a chart that is easier
// than a whole level "overrated", and one harder "underrated".
const (
	Unknown Difficulty = iota
	LevelEasier
	VeryEasy
	Easy
	Medium
	Hard
	VeryHard
	LevelHarder
)

var difficulties = map[string]Difficulty{
	"Overrated":  LevelEasier,
	"VeryEasy":   VeryEasy,
	"Easy":       Easy,
	"Medium":     Medium,
	"Hard":       Hard,
	"VeryHard":   VeryHard,
	"Underrated": LevelHarder,
}

// parseDifficulty reads a difficulty as PIU Scores writes it. Anything
// else, such as "Unrecorded" for a chart too few players have played, is
// Unknown.
func parseDifficulty(s string) Difficulty { return difficulties[s] }

// String names the difficulty ("Very easy"), "" when it is unknown.
func (d Difficulty) String() string {
	switch d {
	case LevelEasier:
		return "1+ level easier"
	case VeryEasy:
		return "Very easy"
	case Easy:
		return "Easy"
	case Medium:
		return "Medium"
	case Hard:
		return "Hard"
	case VeryHard:
		return "Very hard"
	case LevelHarder:
		return "1+ level harder"
	}
	return ""
}

// Class is a CSS-friendly name of the difficulty ("very-easy").
func (d Difficulty) Class() string {
	switch d {
	case LevelEasier:
		return "easier"
	case VeryEasy:
		return "very-easy"
	case Easy:
		return "easy"
	case Medium:
		return "medium"
	case Hard:
		return "hard"
	case VeryHard:
		return "very-hard"
	case LevelHarder:
		return "harder"
	}
	return "unknown"
}

// Ease orders charts by how hard they are to clear, easiest first: by how
// many pass them where PIU Scores knows, otherwise by how well players
// score on them, then by the level they score like. It returns a negative
// number when a is easier than b, a positive one when it is harder.
func Ease(a, b *Chart) int {
	ta, tb := a.clearTier(), b.clearTier()
	if ta != tb {
		return int(ta) - int(tb)
	}
	la, lb := a.ScoringLevel, b.ScoringLevel
	switch {
	case la == lb:
		return 0
	case la == 0:
		return 1
	case lb == 0:
		return -1
	case la < lb:
		return -1
	}
	return 1
}

// ClearDifficulty is how hard the chart is to clear for its level: its pass
// difficulty, or its score difficulty when too few have passed it to tell.
func (ch *Chart) ClearDifficulty() Difficulty {
	if ch.Pass != Unknown {
		return ch.Pass
	}
	return ch.Score
}

// clearTier is ClearDifficulty, with Unknown after every known difficulty.
func (ch *Chart) clearTier() Difficulty {
	if d := ch.ClearDifficulty(); d != Unknown {
		return d
	}
	return LevelHarder + 1
}
