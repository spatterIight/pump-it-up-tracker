package tracker

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resultScreens reads testdata/result-screens.json: real Prime 2 and XX
// result screens, as logged.
func resultScreens(t *testing.T) (doc []byte, entries []map[string]any) {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("testdata", "result-screens.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file struct{ Scores []map[string]any }
	if err := json.Unmarshal(doc, &file); err != nil {
		t.Fatal(err)
	}
	return doc, file.Scores
}

// withNext stands in for a future version that keeps Phoenix scoring.
func withNext() []Version {
	return append(Versions(), Version{ID: "next", Name: "Next", Scoring: PhoenixScoring})
}

func loadVersions(t *testing.T, vs []Version, doc string) *Tracker {
	t.Helper()
	tr, err := LoadVersions(strings.NewReader(doc), vs)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return tr
}

func TestResultScreensOfOtherVersions(t *testing.T) {
	doc, entries := resultScreens(t)
	tr, err := Load(bytes.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Plays) != len(entries) {
		t.Fatalf("%d plays, want %d", len(tr.Plays), len(entries))
	}
	byDate := map[string]*Play{}
	for _, p := range tr.Plays {
		byDate[p.Date.Format("2006-01-02 15:04")] = p
	}
	for _, e := range entries {
		p := byDate[e["date"].(string)]
		if p == nil {
			t.Fatalf("no play on %s", e["date"])
		}
		// The cabinet is the record: score and grade are kept as logged.
		if p.Version.ID != e["version"] || p.Score != int(e["score"].(float64)) || string(p.Grade) != e["grade"] || p.Plate != "" {
			t.Errorf("%s %s: version %s score %d grade %q plate %q", p.Song.Title, p.Chart, p.Version.ID, p.Score, p.Grade, p.Plate)
		}
	}
	if p := byDate["2025-09-09 18:15"]; p.Song.Title != "Le Grand Bleu" || p.Score != 1_038_500 {
		t.Errorf("Le Grand Bleu = %s %d", p.Song.Title, p.Score)
	}
	if p := byDate["2026-07-30 20:50"]; p.Version.ID != "xx" || !p.Broken || p.Grade != "C" {
		t.Errorf("Cannon X.1 = %+v", p)
	}
	// Campanella S9 was played twice in Prime 2: one chart, with the
	// later play a new best.
	for _, s := range tr.Songs {
		if s.Title != "Campanella" {
			continue
		}
		if len(s.Charts) != 1 || len(s.Charts[0].Plays) != 2 || !s.Charts[0].Plays[1].IsPB || s.Charts[0].Plays[1].Delta() != 182_500 {
			t.Errorf("Campanella = %+v", s.Charts)
		}
	}
	if tr.Current.ID != "xx" {
		t.Errorf("current version = %s, want xx, the newest played", tr.Current.ID)
	}
}

// Without their version the result screens read as Phoenix ones, which
// their scores do not fit.
func TestResultScreensNeedTheirVersion(t *testing.T) {
	_, entries := resultScreens(t)
	for _, e := range entries {
		delete(e, "version")
		b, err := json.Marshal(map[string]any{"schema_version": 1, "scores": []any{e}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = Load(bytes.NewReader(b))
		if e["broken"] == true {
			// A stage break's result screen does not add up, so Phoenix does
			// not check its score against its judgments: only its version
			// tells it apart.
			if err != nil {
				t.Errorf("%s: %v", e["song"], err)
			}
			continue
		}
		if err == nil {
			t.Errorf("%s %s loaded as a Phoenix play", e["song"], e["chart"])
		} else if !strings.Contains(err.Error(), "does not match the judgments") && !strings.Contains(err.Error(), "outside 0 to 1000000") {
			t.Errorf("%s: %v", e["song"], err)
		}
	}
}

func TestPrime2Rules(t *testing.T) {
	const judg = `"judgments": {"perfect": 496, "great": 31, "good": 2, "bad": 1, "miss": 0}, "max_combo": 460`
	entry := func(extra string) string {
		return `{"schema_version": 1, "scores": [{"song": "Le Grand Bleu", "chart": "S7", "date": "2025-09-09", "version": "prime2", ` + extra + `}]}`
	}
	tr := load(t, entry(`"score": 1038500, `+judg))
	if p := tr.Plays[0]; p.Score != 1_038_500 || p.Grade != "" {
		t.Errorf("play = %+v, want the score as logged and no grade", p)
	}
	wantProblem(t, entry(`"score": 1038500, "plate": "TG"`), "scores[0] (Le Grand Bleu S7): plate is not used in Prime 2")
	wantProblem(t, entry(judg), "score is required in Prime 2, which cannot be worked out from judgments")
	wantProblem(t, entry(`"score": 1038550`), "score 1038550 is not a multiple of 100, as every Prime 2 score is")
	wantProblem(t, entry(`"score": -100`), "score cannot be negative")
	wantProblem(t, entry(`"score": 1038500, "grade": "SSS"`), `grade "SSS" is not a Prime 2 grade (SS, S, A, B, C, D, F)`)
	wantProblem(t, entry(`"score": 1038500, "grade": "AA+"`), `grade "AA+" is not a Prime 2 grade`)
	// Grades are not checked: an unverified table must not reject a real
	// result screen.
	if p := load(t, entry(`"score": 1038500, "grade": "b", `+judg)).Plays[0]; p.Grade != "B" {
		t.Errorf("grade = %q, want B as logged", p.Grade)
	}
	// XX renamed the grades, adding SSS.
	if p := load(t, strings.Replace(entry(`"score": 1038500, "grade": "SSS"`), "prime2", "xx", 1)).Plays[0]; p.Grade != "SSS" {
		t.Errorf("XX grade = %q", p.Grade)
	}
}

func TestVersionKey(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [
		{"song": "Vook", "chart": "S7", "date": "2025-07-25", "version": "Prime 2", "score": 419400},
		{"song": "Vook", "chart": "S7", "date": "2025-07-26", "version": "PRIME2", "score": 419500},
		{"song": "Vook", "chart": "S10", "date": "2026-09-01", "version": "phoenix", "score": 900000},
		{"song": "Vook", "chart": "S12", "date": "2026-09-01", "score": 800000}
	]}`)
	var got []string
	for _, p := range tr.Plays {
		got = append(got, p.Version.ID)
	}
	if strings.Join(got, " ") != "prime2 prime2 phoenix phoenix" {
		t.Errorf("versions = %v", got)
	}
	wantProblem(t, `{"schema_version": 1, "scores": [
		{"song": "Vook", "chart": "S7", "date": "2025-07-25", "version": "prime3", "score": 419400}
	]}`, `scores[0] (Vook S7): version "prime3" is not a supported game version (expected phoenix, prime2 or xx)`)
}

// A fail with no result is logged the same way in every version.
func TestNoScoreFailInEveryVersion(t *testing.T) {
	for _, v := range []string{"phoenix", "prime2", "xx"} {
		tr := load(t, `{"schema_version": 1, "scores": [
			{"song": "DUEL", "chart": "S13", "date": "2026-09-08", "version": "`+v+`", "broken": true}
		]}`)
		if p := tr.Plays[0]; p.HasScore() || p.Version.ID != v {
			t.Errorf("%s: play = %+v", v, p)
		}
	}
	// Prime 2 cannot work a score out, so judgments do not make one.
	ps := problems(t, `{"schema_version": 1, "scores": [
		{"song": "DUEL", "chart": "S13", "date": "2026-09-08", "version": "prime2", "broken": true,
		 "judgments": {"perfect": 300, "great": 31, "good": 11, "bad": 7, "miss": 60}, "max_combo": 120}
	]}`)
	if len(ps) != 1 || !strings.Contains(ps[0], "cannot have judgments or max_combo") || !strings.HasSuffix(ps[0], "if the result screen showed a score, add it") {
		t.Errorf("problems = %q", ps)
	}
}

// Charts of different versions are different charts unless linked, even at
// the same level, and the headline stats cover the newest version played.
func TestVersionsKeepSeparateCharts(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [
		{"song": "Vook", "chart": "S7", "date": "2025-07-25", "version": "prime2", "score": 419400, "grade": "A", "kcal": 18},
		{"song": "Vook", "chart": "S7", "date": "2025-07-26", "version": "prime2", "score": 999900, "grade": "S"},
		{"song": "Vook", "chart": "S18", "date": "2025-07-26", "version": "prime2", "broken": true},
		{"song": "Katkoi", "chart": "D20", "date": "2025-08-06", "version": "prime2", "score": 646600},
		{"song": "Vook", "chart": "S7", "date": "2026-09-01", "score": 900000, "kcal": 10},
		{"song": "Vook", "chart": "S7", "date": "2026-09-02", "score": 880000}
	]}`)
	vook := tr.Songs[1]
	if vook.Title != "Vook" || len(vook.Charts) != 3 || len(vook.Lineages) != 3 || vook.Version.ID != "phoenix" {
		t.Fatalf("Vook: %d charts, %d lineages, version %s", len(vook.Charts), len(vook.Lineages), vook.Version.ID)
	}
	phoenix, prime2 := vook.Charts[0], vook.Charts[1]
	if phoenix.Version.ID != "phoenix" || prime2.Version.ID != "prime2" || phoenix.Chart != prime2.Chart {
		t.Fatalf("charts = %s %s, %s %s", phoenix.Version.ID, phoenix.Chart, prime2.Version.ID, prime2.Chart)
	}
	if phoenix.Key() != "s7" || prime2.Key() != "prime2-s7" {
		t.Errorf("keys = %s, %s", phoenix.Key(), prime2.Key())
	}
	if phoenix.Record == prime2.Record || phoenix.Lineage == prime2.Lineage {
		t.Fatal("unlinked charts of two versions share a record or lineage")
	}
	if b := phoenix.Record.Best; b.Score != 900000 || len(phoenix.Record.Plays) != 2 || !phoenix.Plays[0].FirstClear || phoenix.Plays[0].PrevBest != 0 {
		t.Errorf("Phoenix S7: best %d, first play %+v", b.Score, phoenix.Plays[0])
	}
	if b := prime2.Record.Best; b.Score != 999900 || !prime2.Plays[1].IsPB || prime2.Plays[1].PrevBest != 419400 {
		t.Errorf("Prime 2 S7: best %d, second play %+v", b.Score, prime2.Plays[1])
	}
	// The song's best grade is its newest version's.
	if g := vook.BestGrade(); g != "AA" {
		t.Errorf("Vook best grade = %s, want Phoenix's AA", g)
	}
	if g := vook.BestGradeIn(prime2.Version); g != "S" {
		t.Errorf("Vook best Prime 2 grade = %s", g)
	}
	groups := vook.LineageGroups()
	if len(groups) != 2 || groups[0].Version.ID != "phoenix" || len(groups[1].Lineages) != 2 {
		t.Errorf("groups = %+v", groups)
	}

	s := tr.Stats(mustDate(t, "2026-09-28"))
	if s.Version.ID != "phoenix" || s.Plays != 2 || s.Songs != 1 || s.Charts != 1 || s.Fails != 0 || s.HighestSingle != 7 || s.HighestDouble != 0 || s.Kcal != 10 {
		t.Errorf("headline stats = %+v, want only Phoenix's", s)
	}
	if s.FirstPlay.Year() != 2026 || len(s.BestGrades) != 1 || s.BestGrades[0] != (GradeCount{"AA", 1}) {
		t.Errorf("headline stats = %+v", s)
	}
	if len(s.Versions) != 2 || s.Versions[0] != (VersionCount{tr.Current, 2, 0}) || s.Versions[1].Version.ID != "prime2" || s.Versions[1].Plays != 4 || s.Versions[1].Fails != 1 {
		t.Errorf("per-version counts = %+v", s.Versions)
	}
	if n := len(tr.DaysIn(prime2.Version)); n != 3 {
		t.Errorf("Prime 2 days = %d, want 3", n)
	}
}

// Across a link between versions that share a scoring system, the chart's
// history carries over: personal bests, first clear and best plate.
func TestLinkWithinAScoringSystem(t *testing.T) {
	tr := loadVersions(t, withNext(), `{"schema_version": 1,
		"songs": {"Katkoi": {"lineages": [{"phoenix": "S7", "next": "S8"}]}},
		"scores": [
		{"song": "Katkoi", "chart": "S7", "date": "2026-08-01", "score": 900000, "plate": "TG",
		 "judgments": {"perfect": 300, "great": 20, "good": 5, "bad": 3, "miss": 2}},
		{"song": "Katkoi", "chart": "S7", "date": "2026-08-02", "broken": true},
		{"song": "Katkoi", "chart": "S8", "date": "2027-03-01", "version": "next", "score": 890000, "plate": "FG"},
		{"song": "Katkoi", "chart": "S8", "date": "2027-03-02", "version": "next", "score": 950000,
		 "judgments": {"perfect": 310, "great": 15, "good": 3, "bad": 1, "miss": 1}}
	]}`)
	song := tr.Songs[0]
	if len(song.Charts) != 2 || len(song.Lineages) != 1 || song.Version.ID != "next" {
		t.Fatalf("%d charts, %d lineages, version %s", len(song.Charts), len(song.Lineages), song.Version.ID)
	}
	l := song.Lineages[0]
	if len(l.Records) != 1 || len(l.Charts) != 2 || l.Charts[0].Version.ID != "phoenix" || l.Newest().Chart.String() != "S8" || l.Key() != "next-s8" {
		t.Fatalf("lineage = %d records, charts %v", len(l.Records), l.Charts)
	}
	next := l.Newest()
	if next.Continues != l.Charts[0] || next.Clears != 2 || next.Record != l.Charts[0].Record {
		t.Errorf("S8 = %+v", next)
	}
	first, second := next.Plays[0], next.Plays[1]
	if first.FirstClear || first.IsPB || first.PrevBest != 900000 || first.Delta() != -10000 {
		t.Errorf("first Next play = %+v, want the Phoenix best to carry over", first)
	}
	if !second.IsPB || second.PrevBest != 900000 || second.Delta() != 50000 {
		t.Errorf("second Next play = %+v", second)
	}
	r := l.Record()
	if r.Best != second || r.BestPlate != "TG" || !r.Cleared || r.Clears != 3 || r.Fails != 1 || len(r.Plays) != 4 {
		t.Errorf("record = best %v plate %s cleared %v clears %d fails %d", r.Best, r.BestPlate, r.Cleared, r.Clears, r.Fails)
	}
	if l.FewestMisses != 2 || l.Clears != 3 || l.Fails != 1 || l.BestPerfectRate < 93 {
		t.Errorf("lineage = fewest misses %d clears %d fails %d", l.FewestMisses, l.Clears, l.Fails)
	}
	// Making Next the newest version played moves the headline stats to it.
	s := tr.Stats(mustDate(t, "2027-03-02"))
	if tr.Current.ID != "next" || s.Version.ID != "next" || s.Plays != 2 || s.Songs != 1 || s.Charts != 1 || s.HighestSingle != 8 || s.PBsRecent != 1 {
		t.Errorf("headline stats = %+v", s)
	}
	if len(s.BestGrades) != 1 || s.BestGrades[0].Grade != "AAA" {
		t.Errorf("best grades = %+v", s.BestGrades)
	}
}

// Across a link between versions with different scoring systems, personal
// bests stay with each version, while the lineage spans both.
func TestLinkAcrossScoringSystems(t *testing.T) {
	tr := load(t, `{"schema_version": 1,
		"songs": {"katkoi": {"lineages": [{"Prime 2": "s7", "phoenix": "S8"}]}},
		"scores": [
		{"song": "Katkoi", "chart": "S7", "date": "2025-08-06 20:38", "version": "prime2", "score": 646600, "grade": "A",
		 "judgments": {"perfect": 397, "great": 23, "good": 3, "bad": 2, "miss": 2}, "max_combo": 245},
		{"song": "Katkoi", "chart": "S8", "date": "2026-09-01", "score": 900000,
		 "judgments": {"perfect": 380, "great": 30, "good": 10, "bad": 4, "miss": 3}},
		{"song": "Katkoi", "chart": "S8", "date": "2026-09-02", "score": 880000}
	]}`)
	song := tr.Songs[0]
	if len(song.Lineages) != 1 {
		t.Fatalf("%d lineages, want 1", len(song.Lineages))
	}
	l := song.Lineages[0]
	if len(l.Records) != 2 || l.Records[0].Scoring != Prime2Scoring || l.Records[1].Scoring != PhoenixScoring || len(l.Plays) != 3 {
		t.Fatalf("lineage = %d records, %d plays", len(l.Records), len(l.Plays))
	}
	p := l.Records[1].Plays[0]
	if !p.FirstClear || !p.IsPB || p.PrevBest != 0 {
		t.Errorf("first Phoenix play = %+v, want a first clear of its own", p)
	}
	if l.Records[0].Best.Score != 646600 || l.Record().Best.Score != 900000 || l.Records[1].Clears != 2 {
		t.Errorf("bests = %d, %d", l.Records[0].Best.Score, l.Record().Best.Score)
	}
	// Misses do not depend on scoring, so they span the lineage.
	if l.FewestMisses != 4 || l.BestPerfectRate != float64(397)*100/427 {
		t.Errorf("fewest misses = %d, best perfect rate %.2f, want Prime 2's", l.FewestMisses, l.BestPerfectRate)
	}
	if g := song.BestGrade(); g != "AA" {
		t.Errorf("best grade = %s", g)
	}
}

// A lineage can run across several versions, and a song can have several.
func TestLineagesChainAndCoexist(t *testing.T) {
	tr := loadVersions(t, withNext(), `{"schema_version": 1,
		"songs": {"Katkoi": {"lineages": [{"next": "S9", "prime2": "S7", "phoenix": "S8"}, {"phoenix": "D18", "xx": "D17"}]}},
		"scores": [
		{"song": "Katkoi", "chart": "S7", "date": "2025-08-06", "version": "prime2", "score": 646600},
		{"song": "Katkoi", "chart": "D17", "date": "2026-07-30", "version": "xx", "score": 700000},
		{"song": "Katkoi", "chart": "S8", "date": "2026-09-01", "score": 900000},
		{"song": "Katkoi", "chart": "D18", "date": "2026-09-01", "score": 800000},
		{"song": "Katkoi", "chart": "S9", "date": "2027-03-01", "version": "next", "score": 910000}
	]}`)
	var got []string
	for _, l := range tr.Songs[0].Lineages {
		var charts []string
		for _, h := range l.Charts {
			charts = append(charts, h.Chart.String()+" "+h.Version.ID)
		}
		got = append(got, strings.Join(charts, " → ")+" in "+
			map[bool]string{true: "one record", false: "records"}[len(l.Records) == 1])
	}
	want := "S7 prime2 → S8 phoenix → S9 next in records; D17 xx → D18 phoenix in records"
	if strings.Join(got, "; ") != want {
		t.Errorf("lineages = %s, want %s", strings.Join(got, "; "), want)
	}
	if n := len(tr.Songs[0].Lineages[0].Records); n != 2 {
		t.Errorf("S9's lineage has %d records, want Prime 2's and Phoenix's with Next's", n)
	}
}

// Lineages belong to songs, not plays.
func TestLineagesAreSongMetadata(t *testing.T) {
	wantProblem(t, `{"schema_version": 1, "scores": [
		{"song": "Katkoi", "chart": "S8", "date": "2026-09-01", "score": 900000, "continues": {"version": "prime2", "chart": "S7"}}
	]}`, `scores[0] (Katkoi S8): unknown key "continues" (expected song, chart, date, version, score, grade, plate, broken, judgments, max_combo, kcal, note)`)
	wantProblem(t, `{"schema_version": 1, "songs": {"Katkoi": {"lineage": [{"prime2": "S7", "phoenix": "S8"}]}}}`,
		`songs: "Katkoi": unknown key "lineage" (expected artist, bpm, image, lineages)`)
}

func TestInvalidLineages(t *testing.T) {
	doc := func(lineages string) string {
		return `{"schema_version": 1, "songs": {"Katkoi": {"lineages": ` + lineages + `}}, "scores": [
			{"song": "Katkoi", "chart": "S7", "date": "2025-08-06", "version": "prime2", "score": 646600},
			{"song": "Katkoi", "chart": "S6", "date": "2025-08-06", "version": "prime2", "score": 700000},
			{"song": "Katkoi", "chart": "S8", "date": "2026-09-01", "score": 900000},
			{"song": "Katkoi", "chart": "S9", "date": "2026-09-01", "score": 900000},
			{"song": "Vook", "chart": "S7", "date": "2025-08-06", "version": "prime2", "score": 646600}
		]}`
	}
	cases := map[string]string{
		// One chart in two lineages.
		`[{"prime2": "S7", "phoenix": "S8"}, {"prime2": "S7", "phoenix": "S9"}]`: `songs: "Katkoi": lineages[1]: S7 in Prime 2 is already in lineages[0]; a chart can only be in one lineage`,
		// A chart with no plays, which is how a typo shows.
		`[{"prime2": "S7", "phoenix": "S10"}]`: `songs: "Katkoi": lineages[0]: S10 in Phoenix has no plays; log at least one play of it, or check the version and chart`,
		`[{"xx": "S7", "phoenix": "S8"}]`:      `songs: "Katkoi": lineages[0]: S7 in XX has no plays`,
		// One version.
		`[{"phoenix": "S8"}]`: `songs: "Katkoi": lineages[0]: a lineage links the charts of at least two versions, such as {prime2: S7, phoenix: S8}`,
		`[{}]`:                `a lineage links the charts of at least two versions`,
		// The same version twice, however it is written.
		`[{"prime2": "S7", "Prime 2": "S6", "phoenix": "S8"}]`: `songs: "Katkoi": lineages[0]: Prime 2 is listed twice; a lineage has one chart per version`,
		`[{"prime3": "S7", "phoenix": "S8"}]`:                  `songs: "Katkoi": lineages[0]: "prime3" is not a supported game version (expected phoenix, prime2 or xx)`,
		`[{"prime2": "Q7", "phoenix": "S8"}]`:                  `songs: "Katkoi": lineages[0]: prime2: chart "Q7" is not in a recognised format`,
		`[["S7", "S8"]]`:                                       `songs: "Katkoi": lineages: expected a mapping, got a list`,
	}
	for lineages, want := range cases {
		wantProblem(t, doc(lineages), want)
	}
	// Another song's chart has no plays of this song.
	wantProblem(t, `{"schema_version": 1, "songs": {"Vook": {"lineages": [{"prime2": "S7", "phoenix": "S8"}]}}, "scores": [
		{"song": "Vook", "chart": "S7", "date": "2025-08-06", "version": "prime2", "score": 646600},
		{"song": "Katkoi", "chart": "S8", "date": "2026-09-01", "score": 900000}
	]}`, `songs: "Vook": lineages[0]: S8 in Phoenix has no plays`)
	// A lineage whose chart's only play has a mistake names just the mistake,
	// whatever the mistake is.
	for play, mistake := range map[string]string{
		`"score": 646650`:                       "not a multiple of 100",
		`"score": 646600, "date": "2025-13-45"`: `date "2025-13-45" is not in a recognised format`,
		`"score": "abc"`:                        `score: expected a whole number, got "abc"`,
		`"score": 646600, "plate": "TG"`:        "plate is not used in Prime 2",
	} {
		ps := problems(t, `{"schema_version": 1, "songs": {"Katkoi": {"lineages": [{"prime2": "S7", "phoenix": "S8"}]}}, "scores": [
			{"song": "Katkoi", "chart": "S7", "date": "2025-08-06", "version": "prime2", `+play+`},
			{"song": "Katkoi", "chart": "S8", "date": "2026-09-01", "score": 900000}
		]}`)
		if len(ps) != 1 || !strings.Contains(ps[0], mistake) {
			t.Errorf("%s: problems = %q", play, ps)
		}
	}
}

// A clear logged without a grade still counts as a cleared chart.
func TestUngradedClears(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [
		{"song": "Vook", "chart": "S7", "date": "2026-07-30", "version": "xx", "score": 600000, "grade": "S"},
		{"song": "Katkoi", "chart": "S7", "date": "2026-07-30", "version": "xx", "score": 600000},
		{"song": "DUEL", "chart": "S13", "date": "2026-07-30", "version": "xx", "broken": true}
	]}`)
	s := tr.Stats(mustDate(t, "2026-07-30"))
	if s.Charts != 3 || len(s.BestGrades) != 1 || s.BestGrades[0] != (GradeCount{"S", 1}) || s.Ungraded != 1 || s.ClearedCharts() != 2 {
		t.Errorf("stats = %+v, cleared %d", s, s.ClearedCharts())
	}
	if g := tr.Songs[1].BestGrade(); tr.Songs[1].Title != "Katkoi" || g != "" || !tr.Songs[1].Cleared() {
		t.Errorf("Katkoi = cleared %v, grade %q", tr.Songs[1].Cleared(), g)
	}
}

func TestVersionList(t *testing.T) {
	if _, err := LoadVersions(strings.NewReader(`{"schema_version": 1}`), []Version{{ID: "prime2", Name: "Prime 2", Scoring: Prime2Scoring}}); err == nil {
		t.Error("a version list without Phoenix was accepted")
	}
	if _, err := LoadVersions(strings.NewReader(`{"schema_version": 1}`), append(Versions(), Version{ID: "xx", Name: "XX again", Scoring: XXScoring})); err == nil {
		t.Error("a version listed twice was accepted")
	}
	// A scoring system with a slice in it would make comparing two panic.
	sliced := Version{ID: "next", Name: "Next", Scoring: slicedScoring{PhoenixScoring, []int{1}}}
	if _, err := LoadVersions(strings.NewReader(`{"schema_version": 1}`), append(Versions(), sliced)); err == nil || !strings.Contains(err.Error(), "make it a pointer") {
		t.Errorf("a scoring system that cannot be compared: %v", err)
	}
	sliced.Scoring = &slicedScoring{PhoenixScoring, []int{1}}
	if _, err := LoadVersions(strings.NewReader(`{"schema_version": 1}`), append(Versions(), sliced)); err != nil {
		t.Errorf("a pointer to it: %v", err)
	}
	// An empty log still has a current version.
	tr := load(t, `{"schema_version": 1}`)
	if tr.Current.ID != "phoenix" || len(tr.PlayedVersions()) != 0 {
		t.Errorf("current = %s", tr.Current.ID)
	}
}

type slicedScoring struct {
	ScoringSystem
	extra []int
}
