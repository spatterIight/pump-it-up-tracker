package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

// graph returns the markup of one metric's graph in the active chart panel
// (the first one) of a song page.
func graph(t *testing.T, body, metric string) string {
	t.Helper()
	i := strings.Index(body, `data-graph="`+metric+`"`)
	if i < 0 {
		t.Fatalf("no %s graph", metric)
	}
	g := body[i+1:]
	if end := strings.Index(g, `<div class="graph-legend">`); end >= 0 {
		g = g[:end]
	}
	if end := strings.Index(g, `data-graph="`); end >= 0 {
		g = g[:end]
	}
	return g
}

func wantAll(t *testing.T, where, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("%s does not contain %q", where, w)
		}
	}
}

func wantNone(t *testing.T, where, body string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(body, u) {
			t.Errorf("%s contains %q", where, u)
		}
	}
}

const prime2Badge = `<span class="ver" title="Pump It Up Prime 2">Prime 2</span>`

func TestSongWithOnlyPrime2Plays(t *testing.T) {
	h := newTestServer(t, "/")
	resp, body := get(t, h, "/song/le-grand-bleu")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	wantAll(t, "Le Grand Bleu", body,
		`<span class="tab-group" role="presentation">Prime 2</span>`, `data-tab="prime2-s7"`,
		`<span class="lineage-label">Version</span> S7 (Prime 2)`, "1,038,500", "Personal best", "First clear", prime2Badge)
	// Prime 2 scores go past 1,000,000 and have no grade lines or plates.
	score := graph(t, body, "score")
	wantAll(t, "score graph", score, ">1040k<", `class="g-grid"`)
	wantNone(t, "score graph", score, "g-grade-line")
	wantNone(t, "Le Grand Bleu", body, `title="No plate"`, "class=\"plate")
}

// A lineage across scoring systems is one chart whose score chart is split,
// while its misses and perfect rates run on across the link.
func TestLineageAcrossScoringSystems(t *testing.T) {
	h := newTestServer(t, "/")
	_, body := get(t, h, "/song/katkoi")
	if n := strings.Count(body, `role="tab"`); n != 1 {
		t.Errorf("%d tabs, want 1", n)
	}
	wantAll(t, "Katkoi", body,
		`data-tab="s7" data-aliases="prime2-s7"`, `<span class="chart-tab-lineage">S7 (Prime 2) → S7 (Phoenix)</span>`,
		`<span class="lineage-anchor" id="prime2-s7"></span>`, "Best in Prime 2", "646,600", "3 clears",
		`<span class="a-chart">S7`+prime2Badge+`</span>`, "Scores are drawn apart for each scoring system")
	score := graph(t, body, "score")
	if n := strings.Count(score, `<div class="graph-part">`); n != 2 {
		t.Errorf("%d score chart parts, want 2", n)
	}
	wantAll(t, "score graph", score, `<p class="graph-part-head">S7 (Prime 2)</p>`, `<p class="graph-part-head">S7 (Phoenix)</p>`)
	wantNone(t, "score graph", score, `class="g-version"`)
	// Each part is scored in its own system: grade lines only for Phoenix.
	parts := strings.Split(score, `<div class="graph-part">`)
	if strings.Contains(parts[1], "g-grade-line") || !strings.Contains(parts[2], "g-grade-line") {
		t.Error("grade lines should be drawn for the Phoenix part only")
	}
	for _, metric := range []string{"misses", "perfect"} {
		g := graph(t, body, metric)
		wantNone(t, metric+" graph", g, "graph-part")
		wantAll(t, metric+" graph", g, `class="g-version"`, ">S7 · Prime 2<", ">S7 · Phoenix<")
		// All three plays in each layout.
		if n := strings.Count(g, `<g class="pt`); n != 6 {
			t.Errorf("%s graph has %d points, want 6", metric, n)
		}
	}
}

// A lineage within one scoring system carries its personal bests across and
// draws one score line.
func TestLineageWithinAScoringSystem(t *testing.T) {
	versions := append(tracker.Versions(), tracker.Version{ID: "next", Name: "Next", Scoring: tracker.PhoenixScoring})
	tr, err := tracker.LoadVersions(strings.NewReader(`{"schema_version": 1, "scores": [
		{"song": "Katkoi", "chart": "S7", "date": "2026-08-01", "score": 900000},
		{"song": "Katkoi", "chart": "S7", "date": "2026-08-02", "score": 920000},
		{"song": "Katkoi", "chart": "S8", "date": "2027-03-01", "version": "next", "score": 910000, "continues": {"version": "phoenix", "chart": "S7"}},
		{"song": "Vook", "chart": "S12", "date": "2026-08-01", "score": 800000}
	]}`), versions)
	if err != nil {
		t.Fatal(err)
	}
	h := serve(t, tr, "/")
	_, body := get(t, h, "/song/katkoi")
	wantAll(t, "Katkoi", body, `data-tab="next-s8" data-aliases="s7"`, "S7 (Phoenix) → S8 (Next)", "920,000",
		"Sun 2 Aug 2026 · S7 Phoenix", `<span class="a-chart">S8</span>`, `<span class="a-chart">S7<span class="ver" title="Pump It Up Phoenix">Phoenix</span></span>`)
	wantNone(t, "Katkoi", body, "Best in", "Scores are drawn apart")
	// The Next play is compared with the Phoenix best.
	wantAll(t, "Katkoi", body, `<span class="delta" title="Compared with the personal best at the time">−10,000</span>`)
	score := graph(t, body, "score")
	wantNone(t, "score graph", score, "graph-part")
	wantAll(t, "score graph", score, `class="g-version"`, "g-grade-line", ">S8 · Next<")

	// Next is now the newest version played: the headline stats cover it, and
	// Phoenix plays get a badge.
	_, home := get(t, h, "/")
	wantAll(t, "home", home, "1 play in Next since", `<span class="card-ver"><span class="ver" title="Pump It Up Phoenix">Phoenix</span></span>`)
}

func TestVersionBadgesAndFilter(t *testing.T) {
	h := newTestServer(t, "/")
	_, body := get(t, h, "/activity")
	wantAll(t, "activity", body, `<nav class="chips version-filter" aria-label="Game version">`,
		`<a class="chip" href="/activity" aria-current="true">All versions</a>`, `<a class="chip" href="/activity?version=prime2">Prime 2</a>`)
	row := func(body, song string) string {
		i := strings.Index(body, `<span class="play-song">`+song+`</span>`)
		if i < 0 {
			t.Fatalf("no play of %s", song)
		}
		return body[i : i+strings.Index(body[i:], "</a>")]
	}
	wantAll(t, "Le Grand Bleu row", row(body, "Le Grand Bleu"), `<span class="play-sub">`+prime2Badge+`Single 7 · 18:15</span>`)
	wantAll(t, "Cannon X.1 row", row(body, "Cannon X.1"), `<span class="ver" title="Pump It Up XX">XX</span>`)
	wantNone(t, "Big Daddy row", row(body, "Big Daddy"), `class="ver"`)
	// A play links to its chart's lineage.
	wantAll(t, "activity", body, `href="/song/katkoi#s7"`, `href="/song/vook#prime2-s7"`)

	_, prime2 := get(t, h, "/activity?version=prime2")
	if n := strings.Count(prime2, `class="play-row`); n != 15 {
		t.Errorf("%d plays listed for Prime 2, want 15", n)
	}
	wantAll(t, "Prime 2 activity", prime2, "11 days at the cabinet · 15 plays in Prime 2", `<a class="chip" href="/activity?version=prime2" aria-current="true">Prime 2</a>`)
	wantNone(t, "Prime 2 activity", prime2, "Big Daddy", "Cannon X.1")
	// An unknown version shows everything.
	_, unknown := get(t, h, "/activity?version=fiesta")
	if n := strings.Count(unknown, `class="play-row`); n != 54 {
		t.Errorf("%d plays listed for an unknown version, want 54", n)
	}

	_, home := get(t, h, "/")
	if n := strings.Count(home, `<a class="card"`); n != 28 {
		t.Errorf("%d songs listed, want 28", n)
	}
	wantAll(t, "home", home, `<ul class="version-counts"`, `<a href="/?version=prime2#songs">`+prime2Badge+`<span>15 plays</span></a>`,
		`<a class="chip" href="/?version=xx#songs">XX</a>`)
	card := func(body, slug string) string {
		i := strings.Index(body, `<a class="card" href="/song/`+slug+`"`)
		if i < 0 {
			t.Fatalf("no card for %s", slug)
		}
		return body[i : i+strings.Index(body[i:], "</a>")]
	}
	// A song's card shows the newest version it was played in.
	wantAll(t, "Le Grand Bleu card", card(home, "le-grand-bleu"), `<span class="card-ver">`+prime2Badge+`</span>`, `>S</span>`)
	vook := card(home, "vook")
	wantNone(t, "Vook card", vook, `class="ver"`, `title="S7`)
	wantAll(t, "Vook card", vook, `title="S10 · best 969,192 AAA&#43;"`, `data-plays="5"`)

	_, filtered := get(t, h, "/?version=prime2")
	if n := strings.Count(filtered, `<a class="card"`); n != 14 {
		t.Errorf("%d songs listed for Prime 2, want 14", n)
	}
	wantAll(t, "Prime 2 songs", filtered, "<span data-count>14</span> of 14 songs played in Prime 2")
	// Filtered to Prime 2, a song shows its Prime 2 charts and grade.
	vook = card(filtered, "vook")
	wantAll(t, "Vook Prime 2 card", vook, prime2Badge, `title="S7 · best 419,400 A"`, `data-plays="1"`)
	wantNone(t, "Vook Prime 2 card", vook, `title="S10`)
	// Hero stats stay on the newest version.
	wantAll(t, "Prime 2 songs", filtered, "38 plays (7 failed) in Phoenix")
}

func TestAPIVersions(t *testing.T) {
	h := newTestServer(t, "/")
	resp, body := get(t, h, "/api/data.json")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	var data struct {
		Stats struct {
			Version string
			Plays   int
		}
		Versions []apiVersion
		Songs    []apiSong
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatal(err)
	}
	if data.Stats.Version != "phoenix" || len(data.Versions) != 3 || data.Versions[2] != (apiVersion{"prime2", "Prime 2", 15, 0}) {
		t.Errorf("stats %+v, versions %+v", data.Stats, data.Versions)
	}
	songs := map[string]apiSong{}
	for _, s := range data.Songs {
		songs[s.Slug] = s
	}
	katkoi := songs["katkoi"]
	if len(katkoi.Charts) != 2 {
		t.Fatalf("katkoi = %+v", katkoi)
	}
	phoenix, prime2 := katkoi.Charts[0], katkoi.Charts[1]
	lineage := []apiLineageChart{{"prime2", "S7"}, {"phoenix", "S7"}}
	for _, c := range []apiChart{phoenix, prime2} {
		if len(c.Lineage) != 2 || c.Lineage[0] != lineage[0] || c.Lineage[1] != lineage[1] {
			t.Errorf("%s %s lineage = %+v", c.Version, c.Chart, c.Lineage)
		}
	}
	if phoenix.Version != "phoenix" || phoenix.Plays != 2 || phoenix.Best.Score != 978502 || phoenix.Best.Version != "phoenix" || phoenix.History[0].Version != "phoenix" {
		t.Errorf("phoenix chart = %+v", phoenix)
	}
	if prime2.Version != "prime2" || prime2.Plays != 1 || prime2.Best.Score != 646600 || *prime2.Best.Grade != "A" || prime2.History[0].Version != "prime2" {
		t.Errorf("prime2 chart = %+v", prime2)
	}
}
