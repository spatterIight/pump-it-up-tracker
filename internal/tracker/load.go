package tracker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// SchemaVersion is the data file format this build reads.
const SchemaVersion = 1

// ValidationError lists every problem found in a data file.
type ValidationError struct {
	Problems []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%d problem(s) in the score data:\n  - %s", len(e.Problems), strings.Join(e.Problems, "\n  - "))
}

type fileData struct {
	SchemaVersion int                 `json:"schema_version"`
	Player        playerData          `json:"player"`
	Songs         map[string]songData `json:"songs"`
	Scores        []scoreData         `json:"scores"`
}

type playerData struct {
	Name flexString `json:"name"`
}

type songData struct {
	Artist flexString `json:"artist"`
	BPM    flexString `json:"bpm"`
	Image  string     `json:"image"`
}

type scoreData struct {
	Song      flexString     `json:"song"`
	Chart     flexString     `json:"chart"`
	Date      flexString     `json:"date"`
	Score     *flexInt       `json:"score"`
	Grade     string         `json:"grade"`
	Plate     string         `json:"plate"`
	Broken    bool           `json:"broken"`
	Judgments *judgmentsData `json:"judgments"`
	MaxCombo  *flexInt       `json:"max_combo"`
	Kcal      *float64       `json:"kcal"`
	Note      flexString     `json:"note"`
}

type judgmentsData struct {
	Perfect *flexInt `json:"perfect"`
	Great   *flexInt `json:"great"`
	Good    *flexInt `json:"good"`
	Bad     *flexInt `json:"bad"`
	Miss    *flexInt `json:"miss"`
}

// flexString accepts a JSON string or number, since YAML turns unquoted
// values such as `bpm: 160` or a song called `1950` into numbers.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		*f = ""
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("expected text, got %s", b)
	}
	*f = flexString(n.String())
	return nil
}

// flexInt accepts a JSON integer or a string of digits, which may carry
// thousands separators or the leading zeros the cabinet prints ("0938204").
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		i, err := strconv.Atoi(n.String())
		if err != nil {
			return fmt.Errorf("expected a whole number, got %s", b)
		}
		*f = flexInt(i)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("expected a whole number, got %s", b)
	}
	cleaned := strings.NewReplacer(",", "", "_", "", " ", "").Replace(strings.TrimSpace(s))
	i, err := strconv.Atoi(cleaned)
	if err != nil {
		return fmt.Errorf("expected a whole number, got %q", s)
	}
	*f = flexInt(i)
	return nil
}

// LoadFile reads and validates a data file.
func LoadFile(path string) (*Tracker, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Load(f)
}

// Load reads and validates score data. A *ValidationError lists every problem
// found, so that they can all be fixed in one go.
func Load(r io.Reader) (*Tracker, error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var data fileData
	if err := dec.Decode(&data); err != nil {
		return nil, &ValidationError{Problems: []string{describeDecodeError(err)}}
	}
	return build(data)
}

func describeDecodeError(err error) string {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return fmt.Sprintf("%s: expected %s, got a JSON %s", typeErr.Field, typeErr.Type, typeErr.Value)
	}
	return err.Error()
}

var dateLayouts = []struct {
	layout  string
	hasTime bool
}{
	{"2006-01-02", false},
	{"2006-01-02T15:04", true},
	{"2006-01-02T15:04:05", true},
	{"2006-01-02 15:04", true},
	{"2006-01-02 15:04:05", true},
	{time.RFC3339, true},
}

func parseDate(s string) (time.Time, bool, error) {
	s = strings.TrimSpace(s)
	for _, l := range dateLayouts {
		if t, err := time.Parse(l.layout, s); err == nil {
			return t, l.hasTime, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("date %q is not in a recognised format (expected YYYY-MM-DD, optionally followed by a time such as 20:15)", s)
}

const octalHint = " (YAML reads numbers written with a leading zero, such as 031, as octal: write 31)"

func build(data fileData) (*Tracker, error) {
	var problems []string
	addf := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if data.SchemaVersion != SchemaVersion {
		addf("schema_version is %d, but this version of pump-it-up-tracker reads schema_version %d", data.SchemaVersion, SchemaVersion)
		return nil, &ValidationError{Problems: problems}
	}

	t := &Tracker{
		Player:      strings.TrimSpace(string(data.Player.Name)),
		songsBySlug: map[string]*Song{},
	}

	// Songs are matched case-insensitively, so "Big daddy" in one entry and
	// "Big Daddy" in another are the same song. The spelling used in the song
	// metadata wins, otherwise the first spelling encountered.
	songs := map[string]*Song{}
	metaTitles := make([]string, 0, len(data.Songs))
	for title := range data.Songs {
		metaTitles = append(metaTitles, title)
	}
	sort.Strings(metaTitles)
	for _, title := range metaTitles {
		meta := data.Songs[title]
		clean := strings.TrimSpace(title)
		if clean == "" {
			addf("songs: a song has an empty title")
			continue
		}
		key := strings.ToLower(clean)
		if _, dup := songs[key]; dup {
			addf("songs: %q is listed more than once (titles are matched case-insensitively)", clean)
			continue
		}
		songs[key] = &Song{
			Title:  clean,
			Artist: strings.TrimSpace(string(meta.Artist)),
			BPM:    strings.TrimSpace(string(meta.BPM)),
			Image:  strings.TrimSpace(meta.Image),
		}
	}

	histories := map[*Song]map[Chart]*ChartHistory{}

	for i, raw := range data.Scores {
		title := strings.TrimSpace(string(raw.Song))
		where := fmt.Sprintf("scores[%d]", i)
		if title != "" {
			where = fmt.Sprintf("scores[%d] (%s %s)", i, title, strings.TrimSpace(string(raw.Chart)))
		}
		entryProblems := len(problems)

		if title == "" {
			addf("%s: song is required", where)
		}
		chart, err := ParseChart(string(raw.Chart))
		if strings.TrimSpace(string(raw.Chart)) == "" {
			addf("%s: chart is required", where)
		} else if err != nil {
			addf("%s: %v", where, err)
		}
		var date time.Time
		var hasTime bool
		if strings.TrimSpace(string(raw.Date)) == "" {
			addf("%s: date is required", where)
		} else if date, hasTime, err = parseDate(string(raw.Date)); err != nil {
			addf("%s: %v", where, err)
		}

		// A stage break with no score is a fail the result screen shows as
		// "-": nothing about it was recorded beyond when it happened.
		noScore := raw.Broken && raw.Score == nil && (raw.Judgments == nil || raw.MaxCombo == nil)
		if noScore {
			var extra []string
			if raw.Grade != "" {
				extra = append(extra, "grade")
			}
			if raw.Plate != "" {
				extra = append(extra, "plate")
			}
			if raw.Judgments != nil {
				extra = append(extra, "judgments")
			}
			if raw.MaxCombo != nil {
				extra = append(extra, "max_combo")
			}
			if len(extra) > 0 {
				addf("%s: a broken play without a score is a fail with no result, so it cannot have %s (only song, chart, date, kcal and note); "+
					"if the result screen showed a score, add it, or both judgments and max_combo", where, strings.Join(extra, " or "))
			}
		}

		var judgments *Judgments
		if raw.Judgments != nil && !noScore {
			j := raw.Judgments
			var missing []string
			get := func(name string, v *flexInt) int {
				if v == nil {
					missing = append(missing, name)
					return 0
				}
				if *v < 0 {
					addf("%s: judgments.%s cannot be negative", where, name)
				}
				return int(*v)
			}
			judgments = &Judgments{
				Perfect: get("perfect", j.Perfect),
				Great:   get("great", j.Great),
				Good:    get("good", j.Good),
				Bad:     get("bad", j.Bad),
				Miss:    get("miss", j.Miss),
			}
			if len(missing) > 0 {
				addf("%s: judgments must list all of perfect, great, good, bad and miss (missing: %s)", where, strings.Join(missing, ", "))
			} else if judgments.Notes() == 0 {
				addf("%s: judgments add up to zero notes", where)
			}
		}

		maxCombo := -1
		if raw.MaxCombo != nil && !noScore {
			maxCombo = int(*raw.MaxCombo)
			if maxCombo < 0 {
				addf("%s: max_combo cannot be negative", where)
			} else if judgments != nil && judgments.Notes() > 0 && maxCombo > judgments.Notes() {
				addf("%s: max_combo %d is more than the %d notes the judgments add up to%s", where, maxCombo, judgments.Notes(), octalHint)
			}
		}

		score := -1
		if raw.Score != nil {
			score = int(*raw.Score)
			if score < 0 || score > MaxScore {
				addf("%s: score %d is outside 0 to %d", where, score, MaxScore)
			}
		}

		if len(problems) == entryProblems && !noScore {
			canReconcile := judgments != nil && maxCombo >= 0
			switch {
			case score < 0 && canReconcile:
				score = ComputeScore(*judgments, maxCombo)
			case score < 0:
				addf("%s: score is required, unless judgments and max_combo are given to work it out from "+
					"(for a failed play with no score, add broken: true instead)", where)
			case canReconcile && !raw.Broken:
				// A broken stage stops counting notes part-way through, so its
				// result screen does not add up the same way.
				if err := checkScore(score, *judgments, maxCombo); err != nil {
					addf("%s: %v%s", where, err, octalHint)
				}
			}
		}

		var grade Grade
		if score >= 0 {
			grade = GradeForScore(score)
		}
		if raw.Grade != "" && !noScore {
			g, ok := parseGrade(raw.Grade)
			switch {
			case !ok:
				addf("%s: grade %q is not a Phoenix grade (SSS+, SSS, SS+, SS, S+, S, AAA+, AAA, AA+, AA, A+, A, B, C, D, F)", where, raw.Grade)
			case raw.Broken:
				grade = g
			case score >= 0 && g != grade:
				addf("%s: grade %s does not match score %d, which earns %s; check for a typo", where, g, score, grade)
			}
		}

		var plate Plate
		if raw.Plate != "" && !noScore {
			p, ok := parsePlate(raw.Plate)
			if !ok {
				addf("%s: plate %q is not one of PG, UG, EG, SG, MG, TG, FG, RG (or their full names)", where, raw.Plate)
			}
			plate = p
		}

		kcal := -1.0
		if raw.Kcal != nil {
			if *raw.Kcal < 0 || math.IsNaN(*raw.Kcal) {
				addf("%s: kcal cannot be negative", where)
			}
			kcal = *raw.Kcal
		}

		if len(problems) > entryProblems {
			continue
		}

		key := strings.ToLower(title)
		song, ok := songs[key]
		if !ok {
			song = &Song{Title: title}
			songs[key] = song
		}
		byChart := histories[song]
		if byChart == nil {
			byChart = map[Chart]*ChartHistory{}
			histories[song] = byChart
		}
		hist := byChart[chart]
		if hist == nil {
			hist = &ChartHistory{Song: song, Chart: chart, FewestMisses: -1, BestPerfectRate: -1}
			byChart[chart] = hist
			song.Charts = append(song.Charts, hist)
		}
		play := &Play{
			Song:      song,
			Chart:     chart,
			History:   hist,
			Date:      date,
			HasTime:   hasTime,
			Score:     score,
			Grade:     grade,
			Plate:     plate,
			Broken:    raw.Broken,
			Judgments: judgments,
			MaxCombo:  maxCombo,
			Kcal:      kcal,
			Note:      strings.TrimSpace(string(raw.Note)),
			index:     i,
		}
		hist.Plays = append(hist.Plays, play)
		song.Plays = append(song.Plays, play)
		t.Plays = append(t.Plays, play)
	}

	if len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}

	chronological := func(ps []*Play) {
		sort.SliceStable(ps, func(i, j int) bool {
			if !ps[i].Date.Equal(ps[j].Date) {
				return ps[i].Date.Before(ps[j].Date)
			}
			return ps[i].index < ps[j].index
		})
	}
	chronological(t.Plays)

	usedSlugs := map[string]bool{}
	titles := make([]string, 0, len(songs))
	for key, s := range songs {
		// Songs that only have metadata and were never played are left out.
		if len(s.Plays) > 0 {
			titles = append(titles, key)
		}
	}
	sort.Strings(titles)
	for _, key := range titles {
		s := songs[key]
		s.Slug = uniqueSlug(Slugify(s.Title), usedSlugs)
		t.songsBySlug[s.Slug] = s
		t.Songs = append(t.Songs, s)

		chronological(s.Plays)
		sort.Slice(s.Charts, func(i, j int) bool { return s.Charts[i].Chart.Less(s.Charts[j].Chart) })
		for _, h := range s.Charts {
			chronological(h.Plays)
			h.derive()
		}
	}
	sort.SliceStable(t.Songs, func(i, j int) bool {
		return strings.ToLower(t.Songs[i].Title) < strings.ToLower(t.Songs[j].Title)
	})

	t.Days = groupDays(t.Plays)
	return t, nil
}

func (h *ChartHistory) derive() {
	best := 0
	for _, p := range h.Plays {
		if p.Judgments != nil {
			if m := p.Judgments.Misses(); h.FewestMisses < 0 || m < h.FewestMisses {
				h.FewestMisses = m
			}
			if r := p.Judgments.PerfectRate(); r > h.BestPerfectRate {
				h.BestPerfectRate = r
			}
		}
		if p.Broken {
			h.Fails++
			continue
		}
		h.Clears++
		p.PrevBest = best
		if !h.Cleared {
			p.FirstClear = true
		}
		if !h.Cleared || p.Score > best {
			p.IsPB = true
			best = p.Score
		}
		h.Cleared = true
		if p.Plate.Rank() > h.BestPlate.Rank() {
			h.BestPlate = p.Plate
		}
		if h.Best == nil || p.Score > h.Best.Score {
			h.Best = p
		}
	}
	if h.Best == nil {
		for _, p := range h.Plays {
			if p.HasScore() && (h.Best == nil || p.Score > h.Best.Score) {
				h.Best = p
			}
		}
	}
}

func groupDays(plays []*Play) []*Day {
	var days []*Day
	byDate := map[string]*Day{}
	for _, p := range plays {
		key := p.Date.Format("2006-01-02")
		d := byDate[key]
		if d == nil {
			y, m, dd := p.Date.Date()
			d = &Day{Date: time.Date(y, m, dd, 0, 0, 0, 0, time.UTC), Kcal: -1}
			byDate[key] = d
			days = append(days, d)
		}
		d.Plays = append(d.Plays, p)
		if p.IsPB {
			d.PBs++
		}
		if p.Kcal >= 0 {
			d.Kcal = max(d.Kcal, 0) + p.Kcal
		}
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Date.After(days[j].Date) })
	return days
}

// Slugify turns a title into a URL path segment: "Big Daddy" becomes
// "big-daddy". Titles without any ASCII letters or digits get a stable
// hash-based slug instead.
func Slugify(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.TrimRight(b.String(), "-")
	if s == "" {
		h := fnv.New32a()
		h.Write([]byte(title))
		s = fmt.Sprintf("song-%08x", h.Sum32())
	}
	return s
}

func uniqueSlug(slug string, used map[string]bool) string {
	candidate := slug
	for n := 2; used[candidate]; n++ {
		candidate = fmt.Sprintf("%s-%d", slug, n)
	}
	used[candidate] = true
	return candidate
}
