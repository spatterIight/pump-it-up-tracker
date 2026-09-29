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
	"slices"
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
	// Lineages lists the song's step charts that are in more than one
	// version, each as a mapping from version to its chart in that version.
	Lineages []map[string]flexString `json:"lineages"`
}

type scoreData struct {
	Song      flexString     `json:"song"`
	Chart     flexString     `json:"chart"`
	Date      flexString     `json:"date"`
	Version   flexString     `json:"version"`
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

// Options configure how score data is read.
type Options struct {
	// Versions are the game versions to read, in release order; nil reads
	// the ones this build reads. Tests use it to stand in for a future
	// version.
	Versions []Version
	// ChartID identifies the step chart that a chart of a song is in a
	// version, "" when it is not known. Charts of a song with the same ID in
	// different versions are the same steps, and are linked as if a lineage
	// in the song's metadata said so, unless one of them is in a lineage
	// already. Nil links only the charts that lineages link.
	ChartID func(song string, v *Version, c Chart) string
}

// LoadFile reads and validates a data file.
func LoadFile(path string) (*Tracker, error) { return LoadFileWith(path, Options{}) }

// LoadFileWith reads and validates a data file with the given options.
func LoadFileWith(path string, o Options) (*Tracker, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LoadWith(f, o)
}

// Load reads and validates score data. A *ValidationError lists every problem
// found, so that they can all be fixed in one go.
func Load(r io.Reader) (*Tracker, error) { return LoadWith(r, Options{}) }

// LoadVersions is Load for the given game versions, in release order, rather
// than the ones this build reads.
func LoadVersions(r io.Reader, vs []Version) (*Tracker, error) {
	return LoadWith(r, Options{Versions: vs})
}

// LoadWith is Load with the given options.
func LoadWith(r io.Reader, o Options) (*Tracker, error) {
	if o.Versions == nil {
		o.Versions = versions
	}
	set, err := newVersionSet(o.Versions)
	if err != nil {
		return nil, err
	}
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
	return build(data, set, o)
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

// chartID identifies a chart: a song (by its lowercased title), a version
// and a chart of it.
type chartID struct {
	song    string
	version *Version
	chart   Chart
}

// lineage is a chart lineage as declared in a song's metadata.
type lineage struct {
	// where names the lineage in problems: `songs: "Katkoi": lineages[0]`.
	where string
	// index is the lineage's place in its song's list.
	index int
	// charts, oldest version first.
	charts []chartID
}

func build(data fileData, vs versionSet, o Options) (*Tracker, error) {
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
		Versions:    vs,
		songsBySlug: map[string]*Song{},
	}
	defaultVersion, _ := vs.lookup(DefaultVersion)

	// Songs are matched case-insensitively, so "Big daddy" in one entry and
	// "Big Daddy" in another are the same song. The spelling used in the song
	// metadata wins, otherwise the first spelling encountered.
	songs := map[string]*Song{}
	var lineages []lineage
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
		for i, decl := range meta.Lineages {
			if l, ok := parseLineage(decl, key, i, fmt.Sprintf("songs: %q: lineages[%d]", clean, i), vs, addf); ok {
				lineages = append(lineages, l)
			}
		}
	}

	histories := map[chartID]*ChartHistory{}
	// logged holds every chart named by an entry, even one rejected for
	// another mistake, so that a link to it is not reported as well.
	logged := map[chartID]bool{}

	for i, entry := range data.Scores {
		// The keys that could be read still name the entry.
		var raw scoreData
		decodeProblems := decodeObject(entry, &raw, "")
		title := strings.TrimSpace(string(raw.Song))
		where := fmt.Sprintf("scores[%d]", i)
		if title != "" {
			where = fmt.Sprintf("scores[%d] (%s %s)", i, title, strings.TrimSpace(string(raw.Chart)))
		}
		// The chart the entry is a play of, when it names one, even if the
		// entry has other mistakes.
		id, identified := identify(raw, vs, defaultVersion)
		if identified {
			logged[id] = true
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
		version := defaultVersion
		if strings.TrimSpace(string(raw.Version)) != "" {
			v, ok := vs.lookup(string(raw.Version))
			if !ok {
				// The rest of the entry depends on its version.
				addf("%s: version %q is not a supported game version (expected %s)", where, raw.Version, vs.list())
				continue
			}
			version = v
		}
		sys := version.Scoring
		canCompute := sys.ComputesScores()

		// A stage break with no score is a fail the result screen shows as
		// "-": nothing about it was recorded beyond when it happened.
		noScore := raw.Broken && raw.Score == nil && (!canCompute || raw.Judgments == nil || raw.MaxCombo == nil)
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
				hint := "if the result screen showed a score, add it, or both judgments and max_combo"
				if !canCompute {
					hint = "if the result screen showed a score, add it"
				}
				addf("%s: a broken play without a score is a fail with no result, so it cannot have %s (only song, chart, date, kcal and note); %s",
					where, strings.Join(extra, " or "), hint)
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
			if maxScore := sys.MaxScore(); maxScore > 0 && (score < 0 || score > maxScore) {
				addf("%s: score %d is outside 0 to %d", where, score, maxScore)
			} else if score < 0 {
				addf("%s: score cannot be negative", where)
			}
		}

		if len(problems) == entryProblems && !noScore {
			canReconcile := judgments != nil && maxCombo >= 0
			switch {
			case score < 0 && canReconcile && canCompute:
				score = sys.ComputeScore(*judgments, maxCombo)
			case score < 0 && canCompute:
				addf("%s: score is required, unless judgments and max_combo are given to work it out from "+
					"(for a failed play with no score, add broken: true instead)", where)
			case score < 0:
				addf("%s: score is required in %s, which cannot be worked out from judgments "+
					"(for a failed play with no score, add broken: true instead)", where, version.Name)
			default:
				// A broken stage stops counting notes part-way through, so its
				// result screen does not add up the same way.
				var against *Judgments
				if !raw.Broken {
					against = judgments
				}
				if err := sys.CheckScore(score, against, maxCombo); err != nil {
					addf("%s: %v", where, err)
				}
			}
		}

		var grade Grade
		gradeWorkedOut := false
		if score >= 0 {
			grade, gradeWorkedOut = sys.Grade(score, judgments)
		}
		if raw.Grade != "" && !noScore {
			g, ok := parseGrade(sys, raw.Grade)
			switch {
			case !ok:
				addf("%s: grade %q is not a %s grade (%s)", where, raw.Grade, version.Name, gradeList(sys.Grades()))
			case raw.Broken || !gradeWorkedOut:
				grade = g
			case score >= 0 && g != grade:
				if sys.GradeThresholds() != nil {
					addf("%s: grade %s does not match score %d, which earns %s; check for a typo", where, g, score, grade)
				} else {
					addf("%s: grade %s does not match the result, which earns %s in %s; check for a typo", where, g, grade, version.Name)
				}
			}
		}

		var plate Plate
		if raw.Plate != "" && !noScore {
			p, ok := parsePlate(raw.Plate)
			switch {
			case !sys.HasPlates():
				addf("%s: plate is not used in %s, which has no plates", where, version.Name)
			case !ok:
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

		song, ok := songs[id.song]
		if !ok {
			song = &Song{Title: title}
			songs[id.song] = song
		}
		hist := histories[id]
		if hist == nil {
			hist = &ChartHistory{Song: song, Version: version, Chart: chart}
			histories[id] = hist
			song.Charts = append(song.Charts, hist)
		}
		play := &Play{
			Song:      song,
			Version:   version,
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

	linked, lineageProblems := resolveLineages(lineages, logged, histories)
	problems = append(problems, lineageProblems...)
	if len(problems) > 0 {
		return nil, &ValidationError{Problems: problems}
	}
	if o.ChartID != nil {
		linkSameCharts(histories, linked, songs, o.ChartID)
	}

	chronological(t.Plays)
	t.Current = vs[len(vs)-1]
	if len(t.Plays) > 0 {
		t.Current = t.PlayedVersions()[0]
	}

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
		sort.Slice(s.Charts, func(i, j int) bool { return chartBefore(s.Charts[i], s.Charts[j]) })
		s.Version = s.Charts[0].Version
		for _, h := range s.Charts {
			chronological(h.Plays)
			for _, p := range h.Plays {
				if p.Broken {
					h.Fails++
				} else {
					h.Clears++
				}
			}
		}
		s.Lineages = buildLineages(s)
	}
	t.Days = groupDays(t.Plays)
	return t, nil
}

// chronological sorts plays by date, then by their order in the data file.
func chronological(ps []*Play) {
	sort.SliceStable(ps, func(i, j int) bool {
		if !ps[i].Date.Equal(ps[j].Date) {
			return ps[i].Date.Before(ps[j].Date)
		}
		return ps[i].index < ps[j].index
	})
}

// chartBefore orders a song's charts: newest version first, then by mode and
// level.
func chartBefore(a, b *ChartHistory) bool {
	if a.Version != b.Version {
		return b.Version.Before(a.Version)
	}
	return a.Chart.Less(b.Chart)
}

func gradeList(gs []Grade) string {
	names := make([]string, len(gs))
	for i, g := range gs {
		names[i] = string(g)
	}
	return strings.Join(names, ", ")
}

// identify works out the chart an entry is a play of: its song, version and
// chart. ok is false when the entry does not name all three correctly.
func identify(raw scoreData, vs versionSet, defaultVersion *Version) (id chartID, ok bool) {
	title := strings.TrimSpace(string(raw.Song))
	chart, err := ParseChart(string(raw.Chart))
	version := defaultVersion
	if strings.TrimSpace(string(raw.Version)) != "" {
		if version, ok = vs.lookup(string(raw.Version)); !ok {
			return chartID{}, false
		}
	}
	return chartID{strings.ToLower(title), version, chart}, title != "" && err == nil
}

// parseLineage reads one of the lineages in the metadata of the song with
// the lowercased title song.
func parseLineage(decl map[string]flexString, song string, index int, where string, vs versionSet, addf func(string, ...any)) (lineage, bool) {
	l := lineage{where: where, index: index}
	valid := true
	keys := make([]string, 0, len(decl))
	for k := range decl {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v, ok := vs.lookup(k)
		if !ok {
			addf("%s: %q is not a supported game version (expected %s)", where, k, vs.list())
			valid = false
			continue
		}
		chart, err := ParseChart(string(decl[k]))
		if err != nil {
			addf("%s: %s: %v", where, k, err)
			valid = false
			continue
		}
		if i := slices.IndexFunc(l.charts, func(c chartID) bool { return c.version == v }); i >= 0 {
			addf("%s: %s is listed twice; a lineage has one chart per version", where, v.Name)
			valid = false
			continue
		}
		l.charts = append(l.charts, chartID{song, v, chart})
	}
	if len(decl) < 2 {
		addf("%s: a lineage links the charts of at least two versions, such as {prime2: S7, phoenix: S8}", where)
		valid = false
	}
	slices.SortFunc(l.charts, func(a, b chartID) int { return a.version.order - b.version.order })
	return l, valid
}

// resolveLineages links the charts of each lineage, and reports lineages
// with a chart that was never played, or that is in another lineage too. It
// returns the charts it linked.
func resolveLineages(ls []lineage, logged map[chartID]bool, histories map[chartID]*ChartHistory) (linked map[chartID]bool, problems []string) {
	in := map[chartID]int{} // the lineage each chart is in, by index
	for _, l := range ls {
		valid := true
		for _, c := range l.charts {
			switch other, taken := in[c]; {
			case !logged[c]:
				problems = append(problems, fmt.Sprintf("%s: %s in %s has no plays; log at least one play of it, or check the version and chart",
					l.where, c.chart, c.version.Name))
				valid = false
			case taken:
				problems = append(problems, fmt.Sprintf("%s: %s in %s is already in lineages[%d]; a chart can only be in one lineage",
					l.where, c.chart, c.version.Name, other))
				valid = false
			}
		}
		if !valid {
			continue
		}
		for i, c := range l.charts {
			in[c] = l.index
			if i > 0 {
				if h, prev := histories[c], histories[l.charts[i-1]]; h != nil && prev != nil {
					h.Continues = prev
				}
			}
		}
	}
	linked = map[chartID]bool{}
	for c := range in {
		linked[c] = true
	}
	return linked, problems
}

// linkSameCharts links each chart that idOf identifies to the chart of the
// same song with the same ID in the newest earlier version it was played in,
// unless that one is already continued. A chart a lineage links to an
// earlier one keeps that link, but can still be continued.
func linkSameCharts(histories map[chartID]*ChartHistory, linked map[chartID]bool, songs map[string]*Song, idOf func(string, *Version, Chart) string) {
	continued := map[*ChartHistory]bool{}
	ids := make([]chartID, 0, len(histories))
	for c, h := range histories {
		ids = append(ids, c)
		if h.Continues != nil {
			continued[h.Continues] = true
		}
	}
	// Oldest version first, so that each chart finds the newest earlier one.
	slices.SortFunc(ids, func(a, b chartID) int {
		switch {
		case a.version != b.version:
			return a.version.order - b.version.order
		case a.song != b.song:
			return strings.Compare(a.song, b.song)
		case a.chart.Less(b.chart):
			return -1
		case b.chart.Less(a.chart):
			return 1
		}
		return 0
	})
	type key struct{ song, id string }
	// newest holds the chart with each ID in the newest version so far.
	newest := map[key]*ChartHistory{}
	for _, c := range ids {
		id := idOf(songs[c.song].Title, c.version, c.chart)
		if id == "" {
			continue
		}
		k := key{c.song, id}
		h := histories[c]
		if prev := newest[k]; prev != nil && prev.Version != h.Version && !linked[c] && !continued[prev] {
			h.Continues = prev
			continued[prev] = true
		}
		newest[k] = h
	}
}

// buildLineages follows the song's chart links into lineages, split into
// records where the score scale changes, and works out personal bests.
func buildLineages(s *Song) []*Lineage {
	continued := map[*ChartHistory]bool{}
	for _, h := range s.Charts {
		if h.Continues != nil {
			continued[h.Continues] = true
		}
	}
	var lineages []*Lineage
	// s.Charts is in the order lineages are listed in: by newest chart.
	for _, newest := range s.Charts {
		if continued[newest] {
			continue
		}
		l := &Lineage{Song: s, FewestMisses: -1, BestPerfectRate: -1}
		for h := newest; h != nil; h = h.Continues {
			l.Charts = append([]*ChartHistory{h}, l.Charts...)
		}
		var r *Record
		for _, h := range l.Charts {
			h.Lineage = l
			if r == nil || r.Scoring.Scale() != h.Version.Scoring.Scale() {
				r = &Record{Lineage: l}
				l.Records = append(l.Records, r)
			}
			// The charts are oldest first, so this leaves the newest one's.
			r.Scoring = h.Version.Scoring
			h.Record = r
			r.Charts = append(r.Charts, h)
			r.Plays = append(r.Plays, h.Plays...)
			l.Plays = append(l.Plays, h.Plays...)
		}
		chronological(l.Plays)
		for _, r := range l.Records {
			chronological(r.Plays)
			r.derive()
			l.Clears += r.Clears
			l.Fails += r.Fails
		}
		for _, p := range l.Plays {
			if p.Judgments != nil {
				if m := p.Judgments.Misses(); l.FewestMisses < 0 || m < l.FewestMisses {
					l.FewestMisses = m
				}
				if r := p.Judgments.PerfectRate(); r > l.BestPerfectRate {
					l.BestPerfectRate = r
				}
			}
		}
		lineages = append(lineages, l)
	}
	return lineages
}

func (r *Record) derive() {
	best := 0
	for _, p := range r.Plays {
		if p.Broken {
			r.Fails++
			continue
		}
		r.Clears++
		p.PrevBest = best
		if !r.Cleared {
			p.FirstClear = true
		}
		if !r.Cleared || p.Score > best {
			p.IsPB = true
			best = p.Score
		}
		r.Cleared = true
		if p.Plate.Rank() > r.BestPlate.Rank() {
			r.BestPlate = p.Plate
		}
		if r.Best == nil || p.Score > r.Best.Score {
			r.Best = p
		}
	}
	if r.Best == nil {
		for _, p := range r.Plays {
			if p.HasScore() && (r.Best == nil || p.Score > r.Best.Score) {
				r.Best = p
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
