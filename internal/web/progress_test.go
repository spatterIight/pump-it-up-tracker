package web

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spatterIight/pump-it-up-tracker/internal/art"
	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

func TestProgressPages(t *testing.T) {
	h := newTestServer(t, "/")
	_, body := get(t, h, "/progress")
	wantAll(t, "progress", body,
		`<h1 class="display">Phoenix</h1>`,
		`<span class="rating-value">1,803</span>`,
		`&#43;772.5</b> in the last 30 days`,
		`<dd>11 <small>of 50</small></dd>`,
		`href="/progress/pumbility"`,
		// The sample's hardest single cleared is Nemesis S16.
		`<b>Single 16</b><small>Your hardest level cleared</small>`,
		`<b>Single 17</b><small>One level up</small>`,
		`<b>Double 13</b><small>Your hardest level cleared</small>`,
		`href="/progress/s16" title="Single 16: 1 of `,
		`href="/progress/d28"`,
		// A clear at AA of an S16 adds its full value while the pool is not full.
		`What a clear at AA would add to your PUMBILITY">+310</span>`)
	wantNone(t, "progress", body, `class="chips version-filter"`, `href="/progress/coop2"`, "worked out")

	_, body = get(t, h, "/progress/s16")
	wantAll(t, "S16", body,
		`<h1 class="display">Single 16</h1>`,
		`<li class="frow is-cleared" data-cleared="true">`,
		`href="/song/nemesis#s16"`,
		`<span class="frow-new">Not played</span>`,
		`data-show="false">Not cleared</button>`,
		`href="/progress/s15"`, `href="/progress/s17"`)
	// The easiest charts to clear come first.
	if easier, harder := strings.Index(body, "diff-easier"), strings.LastIndex(body, "diff-very-hard"); easier < 0 || harder < easier {
		t.Errorf("S16 is not easiest first")
	}
	// Played charts the list does not have are listed after it.
	_, body = get(t, h, "/progress/s13")
	wantAll(t, "S13", body, `title="PIU Scores' chart list does not have this chart">Not in the list</span>`, `href="/song/nemesis#s13"`)

	for _, path := range []string{"/progress/s99", "/progress/coop2", "/progress/sp12", "/progress/nope"} {
		if resp, _ := get(t, h, path); resp.StatusCode != 404 {
			t.Errorf("%s: status %d", path, resp.StatusCode)
		}
	}

	_, body = get(t, h, "/progress/pumbility")
	wantAll(t, "PUMBILITY", body,
		`<h1 class="display">1,803</h1>`,
		`<span class="rated-song">Nemesis</span><span class="rated-sub">838,531 · Sun 27 Sep 2026</span>`,
		`<span class="rated-value">279</span>`,
		`AA <b>+31</b>`)
	wantNone(t, "PUMBILITY", body, "rated-cut", "est.", "worked out")
	if n := strings.Count(body, `<li class="rated">`); n != 11 {
		t.Errorf("%d rated charts, want 11", n)
	}
}

func TestPumbilityOnHomeAndAPI(t *testing.T) {
	h := newTestServer(t, "/")
	_, body := get(t, h, "/")
	wantAll(t, "home", body, `<a class="stat stat-link" href="/progress">`, `<span class="stat-value">1,803</span>`, `+773 in the last 30 days`)
	_, body = get(t, h, "/api/data.json")
	var data struct{ Stats struct{ Pumbility *float64 } }
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatal(err)
	}
	if data.Stats.Pumbility == nil || *data.Stats.Pumbility != 1803 {
		t.Errorf("api pumbility = %v", data.Stats.Pumbility)
	}
}

func TestChartFacts(t *testing.T) {
	h := newTestServer(t, "/")
	_, body := get(t, h, "/song/big-daddy")
	wantAll(t, "Big Daddy", body,
		`<ul class="chart-facts" aria-label="About the chart">`,
		`<li><b>561</b> notes</li>`,
		`<li>Steps by <b>`,
		`<li class="skill">`,
		`href="/progress/s11">Single 11 level`)
	wantNone(t, "Big Daddy", body, "a-warn")

	// The artist and BPM come from the chart list when the metadata has none.
	_, body = get(t, h, "/song/kasou-shinja")
	wantAll(t, "Kasou Shinja", body, `<p class="song-sub">MAX · 170 BPM</p>`)
	_, body = get(t, h, "/")
	wantAll(t, "home", body, `<p class="card-artist">MAX</p>`)

	// Judgments that do not add up to the chart's notes are pointed out.
	tr, err := tracker.Load(strings.NewReader(`{"schema_version": 1, "scores": [
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "judgments": {"perfect": 505, "great": 31, "good": 11, "bad": 7, "miss": 6}, "max_combo": 294}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	_, body = get(t, serve(t, tr, "/"), "/song/big-daddy")
	wantAll(t, "a typo", body, `<span class="a-warn" title="PIU Scores lists 561 notes for this chart: check the judgments for a typo">Judgments add up to 560 notes, not 561</span>`)
}

// Without a chart list, the progress page only has PUMBILITY.
func TestProgressWithoutChartList(t *testing.T) {
	tr, err := tracker.Load(strings.NewReader(`{"schema_version": 1, "scores": [
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 938204}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Options{Tracker: tr, Art: art.New(art.Options{CacheDir: t.TempDir()})})
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	_, body := get(t, h, "/progress")
	wantAll(t, "progress", body, `<span class="rating-value">115.5</span>`)
	wantNone(t, "progress", body, "Clear next", "folder-grid")
	if resp, _ := get(t, h, "/progress/s11"); resp.StatusCode != 404 {
		t.Errorf("a folder without a chart list: status %d", resp.StatusCode)
	}
	_, body = get(t, h, "/song/big-daddy")
	wantNone(t, "song", body, "chart-facts")
}

// Only Prime 2 and XX plays: nothing to measure.
func TestProgressOfOlderVersions(t *testing.T) {
	tr, err := tracker.Load(strings.NewReader(`{"schema_version": 1, "scores": [
		{"song": "Vook", "chart": "S7", "date": "2025-07-25", "version": "prime2", "score": 419400}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	h := serve(t, tr, "/")
	_, body := get(t, h, "/progress")
	wantAll(t, "progress", body, "Nothing to measure yet")
	if resp, _ := get(t, h, "/progress/pumbility"); resp.StatusCode != 404 {
		t.Errorf("PUMBILITY with no version that has one: status %d", resp.StatusCode)
	}
	_, body = get(t, h, "/")
	wantNone(t, "home", body, "stat-link")
}

func TestPhoenix2Progress(t *testing.T) {
	tr, err := tracker.LoadWith(strings.NewReader(`{"schema_version": 1, "scores": [
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-01", "score": 938204},
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-29", "version": "phoenix2", "score": 945000, "plate": "MG"},
		{"song": "Nemesis", "chart": "S16", "date": "2026-09-29", "version": "phoenix2", "score": 790000, "plate": "FG"}
	]}`), tracker.Options{ChartID: charts().ChartID})
	if err != nil {
		t.Fatal(err)
	}
	h := serve(t, tr, "/")
	_, body := get(t, h, "/progress")
	wantAll(t, "progress", body, `<h1 class="display">Phoenix 2</h1>`, `href="/progress?version=phoenix">Phoenix</a>`,
		// 190 × (1.39 + 0.006) and 215 × (1.20 + 0.002)
		`<span class="rating-value">523.67</span>`)
	_, body = get(t, h, "/progress?version=phoenix")
	wantAll(t, "Phoenix progress", body, `<h1 class="display">Phoenix</h1>`, `<span class="rating-value">115.5</span>`, `href="/progress/s11?version=phoenix"`)

	_, body = get(t, h, "/progress/pumbility")
	wantAll(t, "PUMBILITY", body, `est.</small>`, `alt="Marvelous Game"`, `<span class="rated-value">265.24</span>`)

	// The Phoenix chart the chart list says is the same steps carries on.
	_, body = get(t, h, "/song/big-daddy")
	wantAll(t, "Big Daddy", body, `S11 (Phoenix) → S11 (Phoenix 2)`)
}

func TestJackets(t *testing.T) {
	tr, err := tracker.Load(strings.NewReader(`{"schema_version": 1, "scores": [
		{"song": "Big Daddy", "chart": "S11", "date": "2026-09-28", "score": 938204}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	jackets, err := art.Jackets()
	if err != nil {
		t.Fatal(err)
	}
	resolver := art.New(art.Options{CacheDir: t.TempDir(), Bundle: jackets})
	resolver.Start(context.Background(), []art.Song{{Slug: "big-daddy", Title: "Big Daddy"}})
	srv, err := New(Options{Tracker: tr, Catalog: charts(), Art: resolver, Now: func() time.Time { return time.Date(2026, 9, 28, 21, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	// Songs never played show their built-in jacket in level folders.
	name, ok := resolver.BundledJacket("Conflict")
	if !ok {
		t.Fatal("no jacket for Conflict")
	}
	_, body := get(t, h, "/progress/s15")
	wantAll(t, "S15", body, `src="/jacket/`+name+`"`)
	resp, body := get(t, h, "/jacket/"+name)
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/webp" || !strings.HasPrefix(body, "RIFF") || !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("jacket: status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	for _, path := range []string{"/jacket/index.json", "/jacket/nope.webp", "/jacket/..%2Fart.go"} {
		if resp, _ := get(t, h, path); resp.StatusCode != 404 {
			t.Errorf("%s: status %d", path, resp.StatusCode)
		}
	}
}
