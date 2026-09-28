package tracker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// bigDaddy is a real result screen: Big Daddy S11, 938,204, AA+, Talented Game.
var bigDaddy = Judgments{Perfect: 506, Great: 31, Good: 11, Bad: 7, Miss: 6}

func TestComputeScoreMatchesCabinet(t *testing.T) {
	if got := ComputeScore(bigDaddy, 294); got != 938204 {
		t.Fatalf("ComputeScore = %d, want 938204", got)
	}
	perfect := Judgments{Perfect: 400}
	if got := ComputeScore(perfect, 400); got != MaxScore {
		t.Fatalf("all-perfect ComputeScore = %d, want %d", got, MaxScore)
	}
}

func TestGradeForScore(t *testing.T) {
	cases := map[int]Grade{
		1_000_000: "SSS+", 995_000: "SSS+", 994_999: "SSS", 975_000: "S+", 969_999: "AAA+",
		938_204: "AA+", 900_000: "AA", 899_999: "A+", 750_000: "A", 649_999: "C", 449_999: "F", 0: "F",
	}
	for score, want := range cases {
		if got := GradeForScore(score); got != want {
			t.Errorf("GradeForScore(%d) = %s, want %s", score, got, want)
		}
	}
}

func TestParseChart(t *testing.T) {
	cases := map[string]string{
		"S11": "S11", "s11": "S11", "D21": "D21", " d 21 ": "D21", "SP12": "SP12", "dp20": "DP20",
		"CoOp2": "CO-OP x2", "co-op x3": "CO-OP x3", "COOPX5": "CO-OP x5",
	}
	for in, want := range cases {
		c, err := ParseChart(in)
		if err != nil {
			t.Errorf("ParseChart(%q): %v", in, err)
			continue
		}
		if c.String() != want {
			t.Errorf("ParseChart(%q) = %s, want %s", in, c, want)
		}
	}
	for _, bad := range []string{"", "11", "X11", "S0", "S31", "CoOp1", "CoOp6", "Single 11"} {
		if _, err := ParseChart(bad); err == nil {
			t.Errorf("ParseChart(%q) succeeded, want error", bad)
		}
	}
	if k := mustChart(t, "CoOp2").Key(); k != "coop2" {
		t.Errorf("co-op key = %q", k)
	}
	if k := mustChart(t, "D21").Key(); k != "d21" {
		t.Errorf("double key = %q", k)
	}
}

func mustChart(t *testing.T, s string) Chart {
	t.Helper()
	c, err := ParseChart(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func load(t *testing.T, doc string) *Tracker {
	t.Helper()
	tr, err := Load(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return tr
}

func problems(t *testing.T, doc string) []string {
	t.Helper()
	_, err := Load(strings.NewReader(doc))
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("Load error = %v, want *ValidationError", err)
	}
	return verr.Problems
}

func wantProblem(t *testing.T, doc string, substrings ...string) {
	t.Helper()
	ps := problems(t, doc)
	joined := strings.Join(ps, "\n")
	for _, s := range substrings {
		if !strings.Contains(joined, s) {
			t.Errorf("problems %q do not mention %q", ps, s)
		}
	}
}

func TestLoadResultScreen(t *testing.T) {
	tr := load(t, `{
		"schema_version": 1,
		"player": {"name": "PUMPITUP"},
		"songs": {"Big Daddy": {"artist": "Heavy Hitters", "bpm": 128}},
		"scores": [{
			"song": "Big Daddy", "chart": "S11", "date": "2026-09-28",
			"score": 938204, "grade": "AA+", "plate": "Talented Game",
			"judgments": {"perfect": 506, "great": 31, "good": 11, "bad": 7, "miss": 6},
			"max_combo": 294, "kcal": 31.137
		}]
	}`)
	if len(tr.Songs) != 1 || len(tr.Plays) != 1 {
		t.Fatalf("songs=%d plays=%d", len(tr.Songs), len(tr.Plays))
	}
	s := tr.Songs[0]
	if s.Title != "Big Daddy" || s.Slug != "big-daddy" || s.Artist != "Heavy Hitters" || s.BPM != "128" {
		t.Errorf("song = %+v", s)
	}
	p := tr.Plays[0]
	if p.Grade != "AA+" || p.Plate != "TG" || p.MaxCombo != 294 || p.Kcal != 31.137 || !p.IsPB || !p.FirstClear {
		t.Errorf("play = %+v", p)
	}
	if p.Judgments.Misses() != 13 {
		t.Errorf("misses = %d", p.Judgments.Misses())
	}
	if got, ok := tr.Song("big-daddy"); !ok || got != s {
		t.Errorf("Song(big-daddy) = %v, %v", got, ok)
	}
}

func TestScoreWorkedOutFromJudgments(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [{
		"song": "Big Daddy", "chart": "S11", "date": "2026-09-28",
		"judgments": {"perfect": 506, "great": 31, "good": 11, "bad": 7, "miss": 6}, "max_combo": 294
	}]}`)
	if got := tr.Plays[0].Score; got != 938204 {
		t.Fatalf("score = %d, want 938204", got)
	}
}

func TestScoreAsCabinetString(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": "0938204"},
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-29", "score": "940,000"}
	]}`)
	if tr.Plays[0].Score != 938204 || tr.Plays[1].Score != 940000 {
		t.Fatalf("scores = %d, %d", tr.Plays[0].Score, tr.Plays[1].Score)
	}
}

func TestOctalTypoIsCaught(t *testing.T) {
	// `great: 031` in YAML arrives as 25.
	wantProblem(t, `{"schema_version": 1, "scores": [{
		"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 938204,
		"judgments": {"perfect": 506, "great": 25, "good": 9, "bad": 7, "miss": 6}, "max_combo": 294
	}]}`, "scores[0] (Big Daddy S11)", "does not match", "octal")
}

func TestGradeTypoIsCaught(t *testing.T) {
	wantProblem(t, `{"schema_version": 1, "scores": [{
		"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 938204, "grade": "AAA"
	}]}`, "grade AAA does not match score 938204, which earns AA+")
}

func TestBrokenPlaysSkipChecks(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [{
		"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 612000, "grade": "F", "broken": true,
		"judgments": {"perfect": 300, "great": 31, "good": 11, "bad": 7, "miss": 60}, "max_combo": 120
	}]}`)
	p := tr.Plays[0]
	if p.Grade != "F" || p.IsPB || tr.Songs[0].Charts[0].Cleared {
		t.Errorf("broken play = %+v", p)
	}
	if tr.Songs[0].Charts[0].Best != p {
		t.Errorf("an uncleared chart's best should be its best broken play")
	}
}

func TestNoScoreFail(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [
		{"song": "DUEL", "chart": "S13", "date": "2026-09-08", "broken": true, "kcal": 12.5, "note": "life bar gone at the break"}
	]}`)
	p := tr.Plays[0]
	if p.HasScore() || p.Score != -1 || p.Grade != "" || p.Plate != "" || p.Judgments != nil || p.MaxCombo != -1 || !p.Broken {
		t.Errorf("no-score fail = %+v", p)
	}
	if p.Kcal != 12.5 || p.Note != "life bar gone at the break" || p.IsPB || p.FirstClear {
		t.Errorf("no-score fail = %+v", p)
	}
	h := tr.Songs[0].Charts[0]
	if h.Best != nil || h.Cleared || h.Clears != 0 || h.Fails != 1 || h.BestPlate != "" {
		t.Errorf("chart = best %v cleared %v clears %d fails %d plate %q", h.Best, h.Cleared, h.Clears, h.Fails, h.BestPlate)
	}
	if g := tr.Songs[0].BestGrade(); g != "" {
		t.Errorf("BestGrade = %q, want none", g)
	}
}

func TestNoScoreFailRejectsResultKeys(t *testing.T) {
	for key, value := range map[string]string{
		"grade":     `"F"`,
		"plate":     `"RG"`,
		"judgments": `{"perfect": 300, "great": 31, "good": 11, "bad": 7, "miss": 60}`,
		"max_combo": `120`,
	} {
		wantProblem(t, `{"schema_version": 1, "scores": [
			{"song": "DUEL", "chart": "S13", "date": "2026-09-08", "broken": true, "`+key+`": `+value+`}
		]}`, "scores[0] (DUEL S13): a broken play without a score", "cannot have "+key+" ")
	}
	// Every extra key is named in one problem.
	ps := problems(t, `{"schema_version": 1, "scores": [
		{"song": "DUEL", "chart": "S13", "date": "2026-09-08", "broken": true, "grade": "F", "max_combo": 120}
	]}`)
	if len(ps) != 1 || !strings.Contains(ps[0], "cannot have grade or max_combo") {
		t.Errorf("problems = %q", ps)
	}
}

func TestScoreIsStillRequiredUnlessBroken(t *testing.T) {
	wantProblem(t, `{"schema_version": 1, "scores": [
		{"song": "DUEL", "chart": "S13", "date": "2026-09-08"}
	]}`, "scores[0] (DUEL S13): score is required", "add broken: true")
	wantProblem(t, `{"schema_version": 1, "scores": [
		{"song": "DUEL", "chart": "S13", "date": "2026-09-08", "broken": false}
	]}`, "score is required")
}

// A stage break whose judgments and max combo are both given still has a
// score, worked out from them.
func TestBrokenPlayWithJudgmentsHasAScore(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [{
		"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "broken": true,
		"judgments": {"perfect": 506, "great": 31, "good": 11, "bad": 7, "miss": 6}, "max_combo": 294
	}]}`)
	if p := tr.Plays[0]; p.Score != 938204 || p.Grade != "AA+" || tr.Songs[0].Charts[0].Best != p {
		t.Errorf("play = %+v", p)
	}
}

func TestFileFromVersion1_0LoadsUnchanged(t *testing.T) {
	tr, err := LoadFile(filepath.Join("testdata", "tracker-1.0.json"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "tracker-1.0.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot(tr); got != string(want) {
		t.Errorf("derived data changed:\n%s", got)
	}
}

// snapshot prints everything derived from a data file, in the format of
// testdata/tracker-1.0.golden, which was written by version 1.0.1.
func snapshot(t *Tracker) string {
	var b strings.Builder
	for _, s := range t.Songs {
		fmt.Fprintf(&b, "song %s slug=%s best=%s\n", s.Title, s.Slug, s.BestGrade())
		for _, h := range s.Charts {
			fmt.Fprintf(&b, "  chart %s best=%d/%s cleared=%v plate=%s fewest=%d perfect=%.3f\n", h.Chart, h.Best.Score, h.Best.Grade, h.Cleared, h.BestPlate, h.FewestMisses, h.BestPerfectRate)
			for _, p := range h.Plays {
				fmt.Fprintf(&b, "    %s score=%d grade=%s plate=%s broken=%v pb=%v first=%v prev=%d combo=%d kcal=%g\n", p.Date.Format("2006-01-02T15:04"), p.Score, p.Grade, p.Plate, p.Broken, p.IsPB, p.FirstClear, p.PrevBest, p.MaxCombo, p.Kcal)
			}
		}
	}
	for _, d := range t.Days {
		fmt.Fprintf(&b, "day %s plays=%d pbs=%d kcal=%g\n", d.Date.Format("2006-01-02"), len(d.Plays), d.PBs, d.Kcal)
	}
	return b.String()
}

func TestEveryProblemIsReported(t *testing.T) {
	ps := problems(t, `{"schema_version": 1, "scores": [
		{"song": "", "chart": "Q9", "date": "yesterday"},
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "plate": "XG",
		 "judgments": {"perfect": 506, "great": 31}},
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 1000001}
	]}`)
	for _, want := range []string{"song is required", "not in a recognised format", "date \"yesterday\"",
		"plate \"XG\"", "missing: good, bad, miss", "outside 0 to 1000000"} {
		found := false
		for _, p := range ps {
			found = found || strings.Contains(p, want)
		}
		if !found {
			t.Errorf("no problem mentions %q in %q", want, ps)
		}
	}
}

// A value of the wrong type names its entry and key, and does not hide the
// problems in other entries.
func TestTypeMistakesNameTheEntry(t *testing.T) {
	ps := problems(t, `{"schema_version": 1,
		"player": {"name": {}},
		"songs": {"Big Daddy": {"bpm": [128]}},
		"scores": [
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": "93x"},
		{"song": "Nemesis", "chart": "S16", "date": "2026-09-28", "score": 1, "note": true},
		{"song": "Conflict", "chart": "S15", "date": "2026-09-28", "score": 1, "kcal": "31 kcal"},
		{"song": "DUEL", "chart": "S13", "date": "2026-09-08", "broken": "yes"},
		{"song": "Vook", "chart": "S12", "date": "2026-09-28", "score": 1, "judgments": {"perfect": 1, "great": [31]}},
		{"song": "Dignity", "chart": "Q1", "date": "2026-09-28", "score": 1},
		5,
		{"score": "93x", "max_combo": 2.5, "song": "Imprinting", "chart": "S11", "date": "2026-09-28"}
	]}`)
	want := []string{
		`player.name: expected text, got a mapping`,
		`songs: "Big Daddy": bpm: expected text, got a list`,
		`scores[0] (Big Daddy S11): score: expected a whole number, got "93x"`,
		`scores[1] (Nemesis S16): note: expected text, got true`,
		`scores[2] (Conflict S15): kcal: expected a number, got text`,
		`scores[3] (DUEL S13): broken: expected true or false, got text`,
		`scores[4] (Vook S12): judgments.great: expected a whole number, got a list`,
		`scores[5] (Dignity Q1): chart "Q1" is not in a recognised format`,
		`scores[6]: expected a mapping, got 5`,
		`scores[7] (Imprinting S11): score: expected a whole number, got "93x"`,
		`scores[7] (Imprinting S11): max_combo: expected a whole number, got 2.5`,
	}
	if len(ps) != len(want) {
		t.Fatalf("problems = %q, want %d", ps, len(want))
	}
	for i := range want {
		if !strings.HasPrefix(ps[i], want[i]) {
			t.Errorf("problem %d = %q, want %q", i, ps[i], want[i])
		}
	}
}

func TestUnreadableFiles(t *testing.T) {
	for doc, want := range map[string]string{
		"":                                  "the data file is empty",
		" \n":                               "the data file is empty",
		`{"schema_version": 1, "scores": [`: "the data file ends in the middle of the JSON document",
		"{\"schema_version\": 1,\n  \"scores\": [{\"song\": \"Big Daddy\" \"chart\": \"S11\"}]}": `line 2, column 35: invalid character '"' after object key:value pair`,
		"{\"schema_version\": 1, \"scores\": []}\n}":                                             "line 2, column 1: unexpected data after the end of the JSON document",
		`{"schema_version": "1", "scores": []}`:                                                  "schema_version: expected a whole number, got text",
	} {
		ps := problems(t, doc)
		if len(ps) != 1 || !strings.HasPrefix(ps[0], want) {
			t.Errorf("%q: problems = %q, want %q", doc, ps, want)
		}
	}
}

func TestUnknownFieldsAreRejected(t *testing.T) {
	wantProblem(t, `{"schema_version": 1, "scores": [{
		"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 938204, "perfects": 506
	}]}`, `scores[0] (Big Daddy S11): unknown key "perfects" (expected song, chart, date, score, grade, plate, broken, judgments, max_combo, kcal, note)`)
	wantProblem(t, `{"schema_version": 1, "scores": [{
		"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 938204,
		"judgments": {"perfects": 506, "great": 31, "good": 11, "bad": 7, "miss": 6}
	}]}`, `scores[0] (Big Daddy S11): judgments: unknown key "perfects" (expected perfect, great, good, bad, miss)`)
	wantProblem(t, `{"schema_version": 1, "score": []}`, `unknown key "score" (expected schema_version, player, songs, scores)`)
}

func TestSchemaVersion(t *testing.T) {
	wantProblem(t, `{"schema_version": 2, "scores": []}`, "schema_version 1")
}

func TestHistoryAndPersonalBests(t *testing.T) {
	tr := load(t, `{"schema_version": 1,
		"songs": {"Big Daddy": {"image": "big-daddy.png"}},
		"scores": [
		{"song": "big daddy", "chart": "S11", "date": "2026-09-20", "score": 910000},
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-20T21:00", "score": 905000},
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-01", "score": 890000, "plate": "RG"},
		{"song": "BIG DADDY", "chart": "S11", "date": "2026-09-28", "score": 938204, "plate": "TG"},
		{"song": "Big Daddy", "chart": "D12", "date": "2026-09-28", "score": 700000, "broken": true},
		{"song": "Conflict", "chart": "S15", "date": "2026-09-28", "score": 850000, "kcal": 20},
		{"song": "Conflict", "chart": "S15", "date": "2026-09-28", "score": 860000, "kcal": 21.5}
	]}`)
	if len(tr.Songs) != 2 {
		t.Fatalf("songs = %d, want 2 (titles should merge case-insensitively)", len(tr.Songs))
	}
	bd := tr.Songs[0]
	if bd.Title != "Big Daddy" || bd.Image != "big-daddy.png" {
		t.Errorf("song metadata title should win: %+v", bd)
	}
	if len(bd.Charts) != 2 || bd.Charts[0].Chart.String() != "S11" || bd.Charts[1].Chart.String() != "D12" {
		t.Fatalf("charts = %v", bd.Charts)
	}
	s11 := bd.Charts[0]
	var scores []int
	var pbs []bool
	for _, p := range s11.Plays {
		scores = append(scores, p.Score)
		pbs = append(pbs, p.IsPB)
	}
	if want := []int{890000, 910000, 905000, 938204}; !equal(scores, want) {
		t.Errorf("chronological scores = %v, want %v", scores, want)
	}
	if want := []bool{true, true, false, true}; !equalBool(pbs, want) {
		t.Errorf("PB flags = %v, want %v", pbs, want)
	}
	if last := s11.Plays[3]; last.PrevBest != 910000 || last.Delta() != 28204 {
		t.Errorf("last play prev best %d delta %d", last.PrevBest, last.Delta())
	}
	if s11.Best.Score != 938204 || s11.BestPlate != "TG" || !s11.Cleared {
		t.Errorf("S11 summary = best %d plate %s cleared %v", s11.Best.Score, s11.BestPlate, s11.Cleared)
	}
	if bd.TopChart().Chart.String() != "D12" {
		t.Errorf("top chart = %s", bd.TopChart().Chart)
	}
	if len(tr.Days) != 3 || tr.Days[0].Date.Format("2006-01-02") != "2026-09-28" {
		t.Fatalf("days = %d, first %v", len(tr.Days), tr.Days[0].Date)
	}
	if d := tr.Days[0]; len(d.Plays) != 4 || d.PBs != 3 || d.Kcal != 41.5 {
		t.Errorf("latest day: plays %d PBs %d kcal %v", len(d.Plays), d.PBs, d.Kcal)
	}
}

func TestFailsAroundClears(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [
		{"song": "Only Fails", "chart": "S12", "date": "2026-08-24", "broken": true},
		{"song": "Only Fails", "chart": "S12", "date": "2026-09-03", "broken": true},
		{"song": "Fails Then Clear", "chart": "S13", "date": "2026-08-01", "broken": true},
		{"song": "Fails Then Clear", "chart": "S13", "date": "2026-08-02", "score": 612000, "grade": "F", "broken": true},
		{"song": "Fails Then Clear", "chart": "S13", "date": "2026-08-03", "score": 801000, "plate": "RG"},
		{"song": "Clear Then Fails", "chart": "D14", "date": "2026-08-01", "score": 850000, "plate": "FG"},
		{"song": "Clear Then Fails", "chart": "D14", "date": "2026-08-10", "broken": true},
		{"song": "Clear Then Fails", "chart": "D14", "date": "2026-08-11", "score": 700000, "broken": true},
		{"song": "Clear Then Fails", "chart": "D14", "date": "2026-08-12", "broken": true}
	]}`)
	byTitle := map[string]*Song{}
	for _, s := range tr.Songs {
		byTitle[s.Title] = s
	}

	only := byTitle["Only Fails"]
	h := only.Charts[0]
	if h.Best != nil || h.Cleared || h.Clears != 0 || h.Fails != 2 || only.Cleared() || only.BestGrade() != "" {
		t.Errorf("only fails: best %v cleared %v clears %d fails %d", h.Best, h.Cleared, h.Clears, h.Fails)
	}

	before := byTitle["Fails Then Clear"]
	h = before.Charts[0]
	clear := h.Plays[2]
	if h.Best != clear || !h.Cleared || h.Clears != 1 || h.Fails != 2 || h.BestPlate != "RG" {
		t.Errorf("fails then clear: best %v cleared %v clears %d fails %d", h.Best, h.Cleared, h.Clears, h.Fails)
	}
	if !clear.FirstClear || !clear.IsPB || clear.PrevBest != 0 || before.BestGrade() != "A" {
		t.Errorf("first clear after fails = %+v, song best grade %s", clear, before.BestGrade())
	}
	for _, p := range h.Plays[:2] {
		if p.IsPB || p.FirstClear {
			t.Errorf("a fail counted as a PB or clear: %+v", p)
		}
	}

	after := byTitle["Clear Then Fails"]
	h = after.Charts[0]
	if h.Best != h.Plays[0] || !h.Cleared || h.Clears != 1 || h.Fails != 3 || h.BestPlate != "FG" {
		t.Errorf("clear then fails: best %v cleared %v clears %d fails %d", h.Best, h.Cleared, h.Clears, h.Fails)
	}
	for _, p := range h.Plays[1:] {
		if p.IsPB || p.FirstClear {
			t.Errorf("a fail after a clear counted as a PB or clear: %+v", p)
		}
	}
	if after.BestGrade() != "A+" {
		t.Errorf("clear then fails: song best grade %s, want the cleared A+", after.BestGrade())
	}

	s := tr.Stats(mustDate(t, "2026-09-28"))
	if s.Plays != 9 || s.Fails != 7 || s.Charts != 3 {
		t.Errorf("stats = %+v", s)
	}
	if s.HighestSingle != 13 || s.HighestDouble != 14 {
		t.Errorf("highest cleared = S%d D%d, want S13 D14", s.HighestSingle, s.HighestDouble)
	}
	if want := []GradeCount{{"A+", 1}, {"A", 1}}; len(s.BestGrades) != 2 || s.BestGrades[0] != want[0] || s.BestGrades[1] != want[1] {
		t.Errorf("best grades = %v, want %v (charts with only fails have none)", s.BestGrades, want)
	}
}

// A song's best grade comes from its cleared charts, even when a chart that
// was never cleared has a better stage-break grade.
func TestBestGradeIgnoresBreaksOnceCleared(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-01", "score": 780000},
		{"song": "Big Daddy", "chart": "D18", "date": "2026-09-02", "score": 830000, "broken": true}
	]}`)
	if g := tr.Songs[0].BestGrade(); g != "A" {
		t.Errorf("BestGrade = %s, want A from the cleared S11", g)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Big Daddy": "big-daddy", "Kasou Shinja 仮装信者": "kasou-shinja", "U Got 2 Know": "u-got-2-know",
		"Can't Stop!!": "can-t-stop", "  Nemesis  ": "nemesis",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	if got := Slugify("仮装信者"); !strings.HasPrefix(got, "song-") || got != Slugify("仮装信者") {
		t.Errorf("non-ASCII slug = %q", got)
	}
}

func equal(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalBool(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestStats(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [
		{"song": "Big Daddy", "chart": "S11", "date": "2026-08-01", "score": 890000},
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 938204, "kcal": 31.137},
		{"song": "Big Daddy", "chart": "D14", "date": "2026-09-28", "score": 700000, "broken": true},
		{"song": "Conflict", "chart": "D13", "date": "2026-09-27", "score": 972000},
		{"song": "Conflict", "chart": "SP12", "date": "2026-09-27", "score": 960000}
	]}`)
	s := tr.Stats(mustDate(t, "2026-09-28"))
	if s.Plays != 5 || s.Songs != 2 || s.Charts != 4 {
		t.Errorf("counts = %+v", s)
	}
	if s.HighestSingle != 12 || s.HighestDouble != 13 {
		t.Errorf("highest cleared = S%d D%d, want S12 D13 (broken D14 does not count)", s.HighestSingle, s.HighestDouble)
	}
	if s.PBsRecent != 3 {
		t.Errorf("recent PBs = %d, want 3", s.PBsRecent)
	}
	if s.Kcal != 31.137 {
		t.Errorf("kcal = %v", s.Kcal)
	}
	want := []GradeCount{{"S", 1}, {"AAA+", 1}, {"AA+", 1}}
	if len(s.BestGrades) != len(want) {
		t.Fatalf("best grades = %v", s.BestGrades)
	}
	for i := range want {
		if s.BestGrades[i] != want[i] {
			t.Errorf("best grades = %v, want %v", s.BestGrades, want)
		}
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, _, err := parseDate(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
