package tracker

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Mode is the play style of a chart.
type Mode int

// Modes, in the order they are listed on a song.
const (
	ModeSingle Mode = iota
	ModeSinglePerformance
	ModeDouble
	ModeDoublePerformance
	ModeCoOp
)

var modeInfo = map[Mode]struct {
	prefix, name, class string
}{
	ModeSingle:            {"S", "Single", "single"},
	ModeSinglePerformance: {"SP", "Single Performance", "sp"},
	ModeDouble:            {"D", "Double", "double"},
	ModeDoublePerformance: {"DP", "Double Performance", "dp"},
	ModeCoOp:              {"CO-OP", "Co-op", "coop"},
}

// Name is the human readable name of the mode ("Single").
func (m Mode) Name() string { return modeInfo[m].name }

// Class is a CSS-friendly identifier for the mode ("single").
func (m Mode) Class() string { return modeInfo[m].class }

// Chart identifies one step chart of a song, such as S11 or D21.
// For co-op charts, Level holds the number of players.
type Chart struct {
	Mode  Mode
	Level int
}

var chartPattern = regexp.MustCompile(`^(SP|DP|S|D|CO-?OP\s*X?)\s*(\d{1,2})$`)

// ParseChart parses the notation shown on the arcade's level ball: S11, D21,
// SP12, DP20, or CoOp2 / Co-op x3 for co-op charts.
func ParseChart(s string) (Chart, error) {
	m := chartPattern.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(s)))
	if m == nil {
		return Chart{}, fmt.Errorf("chart %q is not in a recognised format (expected e.g. S11, D21, SP12, DP20 or CoOp2)", s)
	}
	level, _ := strconv.Atoi(m[2])
	var c Chart
	switch {
	case m[1] == "S":
		c.Mode = ModeSingle
	case m[1] == "D":
		c.Mode = ModeDouble
	case m[1] == "SP":
		c.Mode = ModeSinglePerformance
	case m[1] == "DP":
		c.Mode = ModeDoublePerformance
	default:
		c.Mode = ModeCoOp
	}
	c.Level = level
	if c.Mode == ModeCoOp {
		if level < 2 || level > 5 {
			return Chart{}, fmt.Errorf("chart %q: co-op charts are for 2 to 5 players", s)
		}
	} else if level < 1 || level > 30 {
		return Chart{}, fmt.Errorf("chart %q: level must be between 1 and 30", s)
	}
	return c, nil
}

// String renders the chart the way the game labels it: S11, D21, CO-OP x2.
func (c Chart) String() string {
	if c.Mode == ModeCoOp {
		return fmt.Sprintf("CO-OP x%d", c.Level)
	}
	return fmt.Sprintf("%s%d", modeInfo[c.Mode].prefix, c.Level)
}

// Ball is the short text shown inside a level ball: the level, or "x2" for co-op.
func (c Chart) Ball() string {
	if c.Mode == ModeCoOp {
		return fmt.Sprintf("x%d", c.Level)
	}
	return strconv.Itoa(c.Level)
}

// Key is a stable, URL-fragment friendly identifier ("s11", "coop2").
func (c Chart) Key() string {
	if c.Mode == ModeCoOp {
		return fmt.Sprintf("coop%d", c.Level)
	}
	return strings.ToLower(c.String())
}

// Less orders charts by mode, then level.
func (c Chart) Less(o Chart) bool {
	if c.Mode != o.Mode {
		return c.Mode < o.Mode
	}
	return c.Level < o.Level
}
