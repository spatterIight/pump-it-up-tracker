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
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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

// fileData is the top level of a data file. The player, each song and each
// score are decoded on their own, so that a mistake in one of them can be
// reported with its place in the file, alongside every other problem.
type fileData struct {
	SchemaVersion int                        `json:"schema_version"`
	Player        json.RawMessage            `json:"player"`
	Songs         map[string]json.RawMessage `json:"songs"`
	Scores        []json.RawMessage          `json:"scores"`
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
		return typeError(b, reflect.TypeFor[flexString]())
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
			return typeError(b, reflect.TypeFor[flexInt]())
		}
		*f = flexInt(i)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return typeError(b, reflect.TypeFor[flexInt]())
	}
	cleaned := strings.NewReplacer(",", "", "_", "", " ", "").Replace(strings.TrimSpace(s))
	i, err := strconv.Atoi(cleaned)
	if err != nil {
		return typeError(b, reflect.TypeFor[flexInt]())
	}
	*f = flexInt(i)
	return nil
}

// typeError reports a value that does not fit a flexible type the way
// encoding/json reports its own mismatches. Short values are quoted as written.
func typeError(b []byte, t reflect.Type) error {
	value := string(b)
	switch b[0] {
	case '{':
		value = "object"
	case '[':
		value = "array"
	}
	return &json.UnmarshalTypeError{Value: value, Type: t}
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
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	var doc json.RawMessage
	if err := dec.Decode(&doc); err != nil {
		return nil, &ValidationError{Problems: []string{describeFileError(b, err)}}
	}
	end := dec.InputOffset()
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		rest := bytes.TrimLeft(b[end:], " \t\r\n")
		return nil, &ValidationError{Problems: []string{
			fmt.Sprintf("%s: unexpected data after the end of the JSON document", position(b, len(b)-len(rest))),
		}}
	}
	var data fileData
	if problems := decodeObject(doc, &data, ""); len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	return build(data)
}

// decodeObject decodes the JSON object b into the struct v points to, one key
// at a time, and returns every problem found, each naming its key (path is
// the key b is under, if any). Going key by key reports every mistake rather
// than the first, and names the key whatever the Go version: newer versions
// of encoding/json leave it out of errors from UnmarshalJSON methods. Keys
// match fields as they do in encoding/json, ignoring case.
func decodeObject(b []byte, v any, path string) []string {
	var problems []string
	add := func(key, msg string) {
		if key != "" {
			msg = key + ": " + msg
		}
		problems = append(problems, msg)
	}
	// A key that is absent or null leaves the struct as it is.
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil
	}
	s := reflect.ValueOf(v).Elem()
	dec := json.NewDecoder(bytes.NewReader(b))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		add(path, describeDecodeError(typeError(b, s.Type())))
		return problems
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			add(path, err.Error())
			return problems
		}
		key := tok.(string)
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			add(path, err.Error())
			return problems
		}
		keyPath := key
		if path != "" {
			keyPath = path + "." + key
		}
		i, ok := fieldForKey(s.Type(), key)
		if !ok {
			add(path, fmt.Sprintf("unknown key %q (expected %s)", key, keyNames(s.Type())))
			continue
		}
		field := s.Field(i)
		if t := field.Type(); t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct && !bytes.Equal(value, []byte("null")) {
			nested := reflect.New(t.Elem())
			problems = append(problems, decodeObject(value, nested.Interface(), keyPath)...)
			field.Set(nested)
			continue
		}
		if err := json.Unmarshal(value, field.Addr().Interface()); err != nil {
			add(keyPath, describeDecodeError(err))
		}
	}
	return problems
}

// fieldForKey finds the field of the struct type t that a JSON key decodes
// into: the one of that name, otherwise one whose name matches ignoring case.
func fieldForKey(t reflect.Type, key string) (int, bool) {
	match := -1
	for i := range t.NumField() {
		name := keyName(t.Field(i))
		if name == key {
			return i, true
		}
		if match < 0 && strings.EqualFold(name, key) {
			match = i
		}
	}
	return match, match >= 0
}

func keyName(f reflect.StructField) string {
	name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
	if name == "" {
		name = f.Name
	}
	return name
}

// keyNames lists the keys the struct type t has, for a problem report.
func keyNames(t reflect.Type) string {
	names := make([]string, t.NumField())
	for i := range names {
		names[i] = keyName(t.Field(i))
	}
	return strings.Join(names, ", ")
}

// describeFileError explains why the data file as a whole could not be read.
func describeFileError(b []byte, err error) string {
	var syntaxErr *json.SyntaxError
	switch {
	case errors.Is(err, io.EOF):
		return "the data file is empty"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "the data file ends in the middle of the JSON document; it may have been cut short"
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("%s: %s", position(b, int(syntaxErr.Offset)-1), syntaxErr.Error())
	}
	return describeDecodeError(err)
}

// position describes where the byte at index i is, as "line 3, column 12".
func position(b []byte, i int) string {
	i = max(0, min(i, len(b)))
	lineStart := bytes.LastIndexByte(b[:i], '\n') + 1
	return fmt.Sprintf("line %d, column %d", bytes.Count(b[:i], []byte("\n"))+1, utf8.RuneCount(b[lineStart:i])+1)
}

// describeDecodeError explains why a value could not be decoded, in the terms
// of the YAML it was written in.
func describeDecodeError(err error) string {
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return fmt.Sprintf("expected %s, got %s", describeType(typeErr.Type), describeValue(typeErr.Value))
	}
	return strings.TrimPrefix(err.Error(), "json: ")
}

func describeType(t reflect.Type) string {
	switch t {
	case reflect.TypeFor[flexInt]():
		return "a whole number"
	case reflect.TypeFor[flexString]():
		return "text"
	}
	switch t.Kind() {
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "a whole number"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.String:
		return "text"
	case reflect.Struct, reflect.Map:
		return "a mapping"
	case reflect.Slice, reflect.Array:
		return "a list"
	case reflect.Pointer:
		return describeType(t.Elem())
	}
	return t.String()
}

// describeValue turns the kind of value encoding/json reports ("string",
// "number 1.5") into words. Values quoted by typeError are kept as they are.
func describeValue(v string) string {
	switch {
	case v == "string":
		return "text"
	case v == "number":
		return "a number"
	case v == "bool":
		return "a true/false value"
	case v == "array":
		return "a list"
	case v == "object":
		return "a mapping"
	case strings.HasPrefix(v, "number "):
		return strings.TrimPrefix(v, "number ")
	}
	return v
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
			// Keep the date and time as written, dropping any UTC offset, so
			// that plays with and without one are ordered by the player's clock.
			return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC), l.hasTime, nil
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

	var player playerData
	problems = append(problems, decodeObject(data.Player, &player, "player")...)
	t := &Tracker{
		Player:      strings.TrimSpace(string(player.Name)),
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
		clean := strings.TrimSpace(title)
		if clean == "" {
			addf("songs: a song has an empty title")
			continue
		}
		var meta songData
		if ps := decodeObject(data.Songs[title], &meta, ""); len(ps) > 0 {
			for _, p := range ps {
				addf("songs: %q: %s", clean, p)
			}
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

	for i, entry := range data.Scores {
		// The keys that could be read still name the entry.
		var raw scoreData
		decodeProblems := decodeObject(entry, &raw, "")
		title := strings.TrimSpace(string(raw.Song))
		where := fmt.Sprintf("scores[%d]", i)
		if title != "" {
			where = fmt.Sprintf("scores[%d] (%s %s)", i, title, strings.TrimSpace(string(raw.Chart)))
		}
		if len(decodeProblems) > 0 {
			for _, p := range decodeProblems {
				addf("%s: %s", where, p)
			}
			continue
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
	// The keys are lowercased titles, so this puts the songs in title order,
	// ignoring case, and gives them their slugs in a stable order.
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
