package tracker

import (
	"strings"
	"testing"
)

// Phoenix 2 scores plays the way Phoenix does, but grades them differently.
func TestPhoenix2Grades(t *testing.T) {
	tr := load(t, `{"schema_version": 1, "scores": [
		{"song": "Conflict", "chart": "S15", "date": "2026-08-31", "version": "phoenix2", "score": 799000, "grade": "A"},
		{"song": "Conflict", "chart": "S15", "date": "2026-09-01", "version": "phoenix2", "score": 910000, "plate": "FG"},
		{"song": "Conflict", "chart": "S15", "date": "2026-09-01", "score": 910000},
		{"song": "Nemesis", "chart": "S16", "date": "2026-09-02", "version": "Phoenix 2", "score": 939999},
		{"song": "Nemesis", "chart": "S16", "date": "2026-09-03", "version": "phoenix2", "score": 800000},
		{"song": "DUEL", "chart": "S13", "date": "2026-09-04", "version": "phoenix2", "score": 799999},
		{"song": "Dignity", "chart": "S12", "date": "2026-09-05", "version": "phoenix2", "score": 650000, "grade": "c"},
		{"song": "Vook", "chart": "S10", "date": "2026-09-06", "version": "phoenix2",
		 "judgments": {"perfect": 506, "great": 31, "good": 11, "bad": 7, "miss": 6}, "max_combo": 294}
	]}`)
	want := map[string]Grade{
		// A is known to start at 800,000 only to within a few thousand points.
		"2026-08-31 phoenix2": "A",
		"2026-09-01 phoenix2": "A+", "2026-09-01 phoenix": "AA",
		"2026-09-02 phoenix2": "AA", "2026-09-03 phoenix2": "A",
		// Where B, C and D start is not known: a score under 800,000 is graded as logged.
		"2026-09-04 phoenix2": "", "2026-09-05 phoenix2": "C",
		// 938,204 is an AA+ in Phoenix, but an AA in Phoenix 2.
		"2026-09-06 phoenix2": "AA",
	}
	for _, p := range tr.Plays {
		key := p.Date.Format("2006-01-02") + " " + p.Version.ID
		if g, ok := want[key]; !ok || p.Grade != g {
			t.Errorf("%s: %s %d graded %q, want %q", key, p.Chart, p.Score, p.Grade, g)
		}
	}
	if p := tr.Plays[len(tr.Plays)-1]; p.Score != 938204 {
		t.Errorf("score worked out from the judgments = %d", p.Score)
	}
	if tr.Current.ID != "phoenix2" {
		t.Errorf("current = %s", tr.Current.ID)
	}

	wantProblem(t, `{"schema_version": 1, "scores": [
		{"song": "Conflict", "chart": "S15", "date": "2026-09-01", "version": "phoenix2", "score": 910000, "grade": "AA"},
		{"song": "Conflict", "chart": "S15", "date": "2026-09-02", "version": "phoenix2", "score": 1000001},
		{"song": "Conflict", "chart": "S15", "date": "2026-09-03", "version": "phoenix2", "score": 938000,
		 "judgments": {"perfect": 506, "great": 31, "good": 11, "bad": 7, "miss": 6}, "max_combo": 294},
		{"song": "Conflict", "chart": "S15", "date": "2026-09-04", "version": "phoenix2", "score": 600000, "grade": "SSS+"}
	]}`,
		"scores[0] (Conflict S15): grade AA does not match score 910000, which earns A+",
		"scores[1] (Conflict S15): score 1000001 is outside 0 to 1000000",
		"scores[2] (Conflict S15): score 938000 does not match the judgments and max combo, which give 938204",
		// A score under every cutoff is graded as logged, but cannot be better than A.
		"scores[3] (Conflict S15): grade SSS+ does not match score 600000: in Phoenix 2, a score under 800000 earns A at best; check for a typo")
}

// A Phoenix chart linked to Phoenix 2 carries its personal bests over, since
// both score plays the same way, and each play keeps its own version's grade.
func TestPhoenix2ContinuesPhoenix(t *testing.T) {
	tr := load(t, `{"schema_version": 1,
		"songs": {"Katkoi": {"lineages": [{"phoenix": "S9", "phoenix2": "S10"}]}},
		"scores": [
		{"song": "Katkoi", "chart": "S9", "date": "2026-08-01", "score": 930000, "plate": "TG"},
		{"song": "Katkoi", "chart": "S10", "date": "2026-10-01", "version": "phoenix2", "score": 925000},
		{"song": "Katkoi", "chart": "S10", "date": "2026-10-02", "version": "phoenix2", "score": 945000}
	]}`)
	l := tr.Songs[0].Lineages[0]
	if len(l.Records) != 1 || len(l.Charts) != 2 || l.Record().Scoring != Phoenix2Scoring {
		t.Fatalf("lineage = %d records, %d charts", len(l.Records), len(l.Charts))
	}
	first, second := l.Plays[1], l.Plays[2]
	if first.FirstClear || first.IsPB || first.PrevBest != 930000 || first.Grade != "AA" {
		t.Errorf("first Phoenix 2 play = %+v, want the Phoenix best to carry over", first)
	}
	if !second.IsPB || second.Grade != "AA+" || l.Record().Best != second || l.Record().BestPlate != "TG" {
		t.Errorf("second Phoenix 2 play = %+v", second)
	}
	if l.Plays[0].Grade != "AA+" {
		t.Errorf("the Phoenix play is graded %s, want its own version's AA+", l.Plays[0].Grade)
	}
}

// Charts that the chart IDs say are the same steps are linked as a lineage
// would link them. A lineage in the metadata takes precedence, and the chart
// it ends with can still be continued.
func TestSameChartsAreLinked(t *testing.T) {
	ids := map[string]string{
		"katkoi phoenix S9": "k1", "katkoi phoenix2 S10": "k1",
		"katkoi phoenix D18": "k2", "katkoi phoenix2 D18": "k2",
		"vook phoenix S10": "v1", "vook phoenix2 S10": "v2",
		"duel phoenix S13": "d1", "duel phoenix2 S13": "d1", "duel phoenix2 S14": "d2",
	}
	opts := Options{ChartID: func(song string, v *Version, c Chart) string {
		return ids[strings.ToLower(song)+" "+v.ID+" "+c.String()]
	}}
	tr, err := LoadWith(strings.NewReader(`{"schema_version": 1,
		"songs": {
			"Katkoi": {"lineages": [{"prime2": "D17", "phoenix": "D18"}]},
			"DUEL": {"lineages": [{"phoenix": "S13", "phoenix2": "S14"}]}
		},
		"scores": [
		{"song": "Katkoi", "chart": "S9", "date": "2026-08-01", "score": 930000},
		{"song": "Katkoi", "chart": "S10", "date": "2026-10-01", "version": "phoenix2", "score": 945000},
		{"song": "Katkoi", "chart": "D17", "date": "2025-08-01", "version": "prime2", "score": 500000},
		{"song": "Katkoi", "chart": "D18", "date": "2026-08-01", "score": 800000},
		{"song": "Katkoi", "chart": "D18", "date": "2026-10-01", "version": "phoenix2", "score": 810000},
		{"song": "Vook", "chart": "S10", "date": "2026-08-01", "score": 900000},
		{"song": "Vook", "chart": "S10", "date": "2026-10-01", "version": "phoenix2", "score": 900000},
		{"song": "DUEL", "chart": "S13", "date": "2026-08-01", "score": 900000},
		{"song": "DUEL", "chart": "S13", "date": "2026-10-01", "version": "phoenix2", "score": 900000},
		{"song": "DUEL", "chart": "S14", "date": "2026-10-01", "version": "phoenix2", "score": 900000}
	]}`), opts)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range tr.Songs {
		for _, l := range s.Lineages {
			got = append(got, s.Title+": "+strings.ReplaceAll(chartsKeys(l), " ", ","))
		}
	}
	want := strings.Join([]string{
		// The lineage says DUEL's Phoenix S13 is Phoenix 2's S14, whatever the IDs say.
		"DUEL: phoenix2-s13", "DUEL: s13,phoenix2-s14",
		// Katkoi's Phoenix D18 ends a lineage from Prime 2, which its Phoenix 2 D18 continues.
		"Katkoi: s9,phoenix2-s10", "Katkoi: prime2-d17,d18,phoenix2-d18",
		"Vook: phoenix2-s10", "Vook: s10",
	}, "|")
	if strings.Join(got, "|") != want {
		t.Errorf("lineages = %v", got)
	}
}

func chartsKeys(l *Lineage) string {
	keys := make([]string, len(l.Charts))
	for i, h := range l.Charts {
		keys[i] = h.Key()
	}
	return strings.Join(keys, " ")
}
