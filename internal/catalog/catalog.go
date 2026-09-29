// Package catalog is the list of every chart of Phoenix and Phoenix 2, with
// facts about each, as PIU Scores (https://piuscores.arroweclip.se) knows
// them. It is built into the app; data/SOURCE.txt says where it comes from.
package catalog

import (
	"embed"
	"encoding/csv"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

// The charts of each version are PIU Scores' chart export for it, named
// after the version's ID.
//
//go:embed data/*.csv
var files embed.FS

// Catalog is the charts of every version it has a list for.
type Catalog struct {
	mixes map[string]*Mix
}

// Mix is every chart of one game version.
type Mix struct {
	// Version is the ID of the version ("phoenix2").
	Version string
	// Charts in the order PIU Scores lists them.
	Charts []*Chart

	byChart map[chartKey]*Chart
	// songs maps each song's lowercased name to its name, and loose maps
	// names without case or punctuation ("rockthehouse") to the names they
	// come from.
	songs map[string]string
	loose map[string][]string
}

type chartKey struct {
	song  string
	chart tracker.Chart
}

// Chart is one chart in a version.
type Chart struct {
	// ID identifies the step chart: a chart has the same ID in every
	// version, whatever its level there.
	ID         string
	Song       string
	Artist     string
	StepArtist string
	Chart      tracker.Chart
	// SongType is "Arcade", "ShortCut", "FullSong" or "Remix".
	SongType string
	BPM      string
	// Notes is the note count PIU Scores has, 0 when it has none. Many are
	// placeholders; see TrustedNotes.
	Notes int
	// Skills are the chart's skill tags, such as "Twists" or "Drills".
	Skills []string
	// Pass is how hard the chart is to clear for its level, and Score how
	// hard it is to score well on, as PIU Scores' players find.
	Pass, Score Difficulty
	// ScoringLevel is the level the chart scores like, such as 17.4 for a
	// hard S17, 0 when it is not known.
	ScoringLevel float64
}

// Load reads the charts built into the app.
func Load() (*Catalog, error) {
	sub, err := fs.Sub(files, "data")
	if err != nil {
		return nil, err
	}
	return LoadFS(sub)
}

// LoadFS reads a catalog from the CSV files in fsys, one per version, named
// after the version's ID.
func LoadFS(fsys fs.FS) (*Catalog, error) {
	c := &Catalog{mixes: map[string]*Mix{}}
	names, err := fs.Glob(fsys, "*.csv")
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		f, err := fsys.Open(name)
		if err != nil {
			return nil, err
		}
		m, err := readMix(strings.TrimSuffix(name, ".csv"), f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("chart list %s: %w", name, err)
		}
		c.mixes[m.Version] = m
	}
	return c, nil
}

// columns are the columns a chart list needs.
var columns = []string{"ChartId", "Song", "Artist", "StepArtist", "Type", "Level", "SongType", "BPM", "NoteCount", "Badges", "PassDifficulty", "ScoreDifficulty", "ScoringLevel"}

func readMix(version string, r io.Reader) (*Mix, error) {
	cr := csv.NewReader(r)
	header, err := cr.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimPrefix(h, "\uFEFF")] = i
	}
	for _, name := range columns {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("no %s column", name)
		}
	}
	m := &Mix{Version: version, byChart: map[chartKey]*Chart{}, songs: map[string]string{}, loose: map[string][]string{}}
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		get := func(name string) string { return strings.TrimSpace(rec[col[name]]) }
		chart, err := parseChart(get("Type"), get("Level"))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		ch := &Chart{
			ID:         get("ChartId"),
			Song:       get("Song"),
			Artist:     get("Artist"),
			StepArtist: get("StepArtist"),
			Chart:      chart,
			SongType:   get("SongType"),
			BPM:        tidyBPM(get("BPM")),
			Pass:       parseDifficulty(get("PassDifficulty")),
			Score:      parseDifficulty(get("ScoreDifficulty")),
		}
		ch.Notes, _ = strconv.Atoi(get("NoteCount"))
		ch.ScoringLevel, _ = strconv.ParseFloat(get("ScoringLevel"), 64)
		for _, s := range strings.Split(get("Badges"), ";") {
			if s = strings.TrimSpace(s); s != "" {
				ch.Skills = append(ch.Skills, s)
			}
		}
		key := chartKey{strings.ToLower(ch.Song), chart}
		if ch.Song == "" || m.byChart[key] != nil {
			continue
		}
		m.Charts = append(m.Charts, ch)
		m.byChart[key] = ch
		if _, ok := m.songs[key.song]; !ok {
			m.songs[key.song] = ch.Song
			loose := alnum(ch.Song)
			m.loose[loose] = append(m.loose[loose], ch.Song)
		}
	}
	return m, nil
}

func parseChart(typ, level string) (tracker.Chart, error) {
	prefix := map[string]string{"S": "S", "D": "D", "CoOp": "CoOp"}[typ]
	if prefix == "" {
		return tracker.Chart{}, fmt.Errorf("chart type %q is not S, D or CoOp", typ)
	}
	return tracker.ParseChart(prefix + level)
}

var bpmRange = regexp.MustCompile(`^([\d.]+)\s*~\s*([\d.]+)$`)

// tidyBPM writes a range whose ends are the same ("120 ~ 120") as one BPM,
// and any other range with a dash.
func tidyBPM(s string) string {
	if m := bpmRange.FindStringSubmatch(s); m != nil {
		if m[1] == m[2] {
			return m[1]
		}
		return m[1] + "–" + m[2]
	}
	return s
}

// Mix returns the charts of version v, nil when the catalog has no list of
// them.
func (c *Catalog) Mix(v *tracker.Version) *Mix {
	if c == nil || v == nil {
		return nil
	}
	return c.mixes[v.ID]
}

// featuring is a title's credit of a featured artist, which a song's name
// in the list may leave out.
var featuring = regexp.MustCompile(`(?i)\s+(?:\(?feat\.?|\(?ft\.)\s.*$`)

// Song returns the name the list gives the song with the given title: the
// same name ignoring case, or else the only one that matches ignoring case
// and punctuation. A title with a featured artist that matches neither way
// is tried again without them.
func (m *Mix) Song(title string) (string, bool) {
	if m == nil {
		return "", false
	}
	if name, ok := m.songs[strings.ToLower(strings.TrimSpace(title))]; ok {
		return name, true
	}
	if names := m.loose[alnum(title)]; alnum(title) != "" && len(names) == 1 {
		return names[0], true
	}
	if short := featuring.ReplaceAllString(title, ""); short != title {
		return m.Song(short)
	}
	return "", false
}

// Lookup returns chart c of the song with the given title, nil when the list
// does not have it. A nil Mix has no charts.
func (m *Mix) Lookup(title string, c tracker.Chart) *Chart {
	name, ok := m.Song(title)
	if !ok {
		return nil
	}
	return m.byChart[chartKey{strings.ToLower(name), c}]
}

// Folder returns the charts of one mode at one level, as the game's level
// folders list them, in the order of the list.
func (m *Mix) Folder(mode tracker.Mode, level int) []*Chart {
	if m == nil {
		return nil
	}
	var out []*Chart
	for _, ch := range m.Charts {
		if ch.Chart.Mode == mode && ch.Chart.Level == level {
			out = append(out, ch)
		}
	}
	return out
}

// Levels lists the levels there are charts of in one mode, lowest first.
func (m *Mix) Levels(mode tracker.Mode) []int {
	if m == nil {
		return nil
	}
	var out []int
	for _, ch := range m.Charts {
		if ch.Chart.Mode == mode && !slices.Contains(out, ch.Chart.Level) {
			out = append(out, ch.Chart.Level)
		}
	}
	slices.Sort(out)
	return out
}

// ChartID identifies the step chart that chart c of a song is in version v,
// "" when the catalog does not have it. It links charts across versions; see
// tracker.Options.
func (c *Catalog) ChartID(song string, v *tracker.Version, chart tracker.Chart) string {
	if ch := c.Mix(v).Lookup(song, chart); ch != nil {
		return ch.ID
	}
	return ""
}

// TrustedNotes returns the chart's note count, or 0 when PIU Scores has none
// or it looks like a placeholder: a round hundred, or the same digit
// repeated (1111), which hundreds of charts have and few really do.
func (ch *Chart) TrustedNotes() int {
	n := ch.Notes
	if n <= 0 || n%100 == 0 {
		return 0
	}
	s := strconv.Itoa(n)
	if len(s) >= 3 && strings.Count(s, s[:1]) == len(s) {
		return 0
	}
	return n
}

// alnum lowercases s and keeps only its ASCII letters and digits.
func alnum(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
