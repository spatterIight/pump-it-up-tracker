package web

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spatterIight/pump-it-up-tracker/internal/art"
	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

func newTestServer(t *testing.T, basePath string) http.Handler {
	t.Helper()
	tr, err := tracker.LoadFile(filepath.Join("..", "..", "sample", "tracker.json"))
	if err != nil {
		t.Fatal(err)
	}
	custom := t.TempDir()
	os.WriteFile(filepath.Join(custom, "big-daddy.png"), []byte("\x89PNG\r\n\x1a\nfake"), 0o644)
	resolver := art.New(art.Options{CacheDir: t.TempDir(), CustomDir: custom})
	var songs []art.Song
	for _, s := range tr.Songs {
		songs = append(songs, art.Song{Slug: s.Slug, Title: s.Title, Image: s.Image})
	}
	resolver.Start(context.Background(), songs)
	srv, err := New(Options{
		Tracker:  tr,
		Art:      resolver,
		BasePath: basePath,
		Version:  "test",
		Now:      func() time.Time { return time.Date(2026, 9, 28, 21, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv.Handler()
}

func get(t *testing.T, h http.Handler, path string) (*http.Response, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Result(), string(body)
}

func TestPages(t *testing.T) {
	h := newTestServer(t, "/")
	cases := []struct {
		path   string
		status int
		want   []string
	}{
		{"/", 200, []string{"PUMP IT UP", "Latest session", `href="/song/big-daddy"`, "Kasou Shinja 仮装信者", `data-modes="single"`, "Hardest clears", "36 plays (7 failed)"}},
		{"/song/big-daddy", 200, []string{"<h1 class=\"display\">Big Daddy</h1>", "938,204", "Talented Game", "graph-svg is-wide", "graph-svg is-narrow", "506", "31.1", "First clear"}},
		{"/song/nemesis", 200, []string{"Stage break", "died at the drill section", `data-tab="s16"`}},
		{"/song/destination", 200, []string{"CO-OP x2", "with Sam"}},
		{"/activity", 200, []string{"Sessions", "11 days at the cabinet", "148.2 kcal", "36 plays (7 failed)", `<span class="tag tag-fail">Failed</span>`}},
		{"/song/nope", 404, []string{"Stage break"}},
		{"/nope", 404, []string{"nothing at this address"}},
	}
	for _, c := range cases {
		resp, body := get(t, h, c.path)
		if resp.StatusCode != c.status {
			t.Errorf("%s: status %d, want %d", c.path, resp.StatusCode, c.status)
		}
		for _, w := range c.want {
			if !strings.Contains(body, w) {
				t.Errorf("%s: body does not contain %q", c.path, w)
			}
		}
		if resp.Header.Get("Content-Security-Policy") == "" {
			t.Errorf("%s: no Content-Security-Policy header", c.path)
		}
	}
}

// A fail with no result must never show its internal score of -1.
func TestNoScoreIsNeverShown(t *testing.T) {
	h := newTestServer(t, "/")
	for _, path := range []string{"/", "/activity", "/song/duel", "/song/vook", "/song/wither-garden"} {
		_, body := get(t, h, path)
		for _, bad := range []string{"−1<", ">-1<", "−1 ", "<nil>"} {
			if strings.Contains(body, bad) {
				t.Errorf("%s: body contains %q", path, bad)
			}
		}
	}
}

func TestSongWithOnlyFails(t *testing.T) {
	h := newTestServer(t, "/")
	resp, body := get(t, h, "/song/duel")
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	for _, want := range []string{
		`<span class="nc-badge">Not cleared</span><span class="nc-count">2 attempts</span>`,
		"No result yet · not cleared", "Last tried Sun 27 Sep 2026", "not cleared yet · 2 fails",
		`<li class="attempt is-broken is-failed">`, `<span class="tag tag-fail">Failed</span>`,
		`title="No grade">–</span>`, `title="No plate">–</span>`, `title="No judgments">–</span>`,
		"Failed, no score", "Not cleared yet", "lost it in the last run again",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
	for _, unwanted := range []string{"Each play", "Stage break", `class="pt-dot" cx=`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("body contains %q", unwanted)
		}
	}
	// Two fails, each drawn in both chart layouts.
	if n := strings.Count(body, `class="pt is-failed"`); n != 4 {
		t.Errorf("%d fail markers, want 4", n)
	}
	if !strings.Contains(body, "data-tip=\"Tue 8 Sep 2026\nFailed\"") {
		t.Errorf("fail marker tooltip missing")
	}

	_, home := get(t, h, "/")
	card := home[strings.Index(home, `<a class="card" href="/song/duel"`):]
	card = card[:strings.Index(card, "</a>")]
	for _, want := range []string{`data-grade="0"`, `<span class="card-grade card-nc">`, `<span class="nc-count">2 attempts</span>`, `title="S13 · not cleared, 2 attempts"`} {
		if !strings.Contains(card, want) {
			t.Errorf("songs list card does not contain %q:\n%s", want, card)
		}
	}
}

// A fail after a clear is a marker at the bottom of the chart, on its date,
// after the scored plays.
func TestFailMarkersAfterAClear(t *testing.T) {
	h := newTestServer(t, "/")
	_, body := get(t, h, "/song/vook")
	panel := body[strings.Index(body, `id="s12"`):]
	wide := panel[strings.Index(panel, `<svg class="graph-svg is-wide"`):]
	wide = wide[:strings.Index(wide, "</svg>")]
	fail := strings.Index(wide, `class="pt is-failed"`)
	if fail < 0 || !strings.Contains(wide[fail:], `cy="262.0"`) {
		t.Errorf("no fail marker on the bottom edge (y 262) of the wide chart:\n%s", wide)
	}
	if !strings.Contains(panel, "1 clear · 1 fail") || !strings.Contains(panel, "Personal best") {
		t.Errorf("Vook S12 should still show its clear")
	}
}

func TestChartsAreValidSVG(t *testing.T) {
	h := newTestServer(t, "/")
	// Two charts (S15, D13) × three metrics × two layouts.
	if n := countValidSVGs(t, h, "/song/conflict"); n != 12 {
		t.Errorf("found %d chart SVGs, want 12", n)
	}
	// One chart with only fails, and no judgments to switch metric by.
	if n := countValidSVGs(t, h, "/song/duel"); n != 2 {
		t.Errorf("found %d chart SVGs, want 2", n)
	}
	// S10 and S12 × three metrics × two layouts; S12 ends with a fail.
	if n := countValidSVGs(t, h, "/song/vook"); n != 12 {
		t.Errorf("found %d chart SVGs, want 12", n)
	}
}

func countValidSVGs(t *testing.T, h http.Handler, path string) int {
	t.Helper()
	_, body := get(t, h, path)
	n := 0
	for {
		start := strings.Index(body, `<svg class="graph-svg`)
		if start < 0 {
			break
		}
		end := strings.Index(body[start:], "</svg>") + start + len("</svg>")
		if err := xml.Unmarshal([]byte(body[start:end]), new(struct{})); err != nil {
			t.Fatalf("%s: chart %d is not well-formed: %v", path, n, err)
		}
		body = body[end:]
		n++
	}
	return n
}

func TestArt(t *testing.T) {
	h := newTestServer(t, "/")
	resp, body := get(t, h, "/art/big-daddy")
	if resp.StatusCode != 200 || !strings.HasPrefix(body, "\x89PNG") || resp.Header.Get("Cache-Control") == "no-cache" {
		t.Errorf("custom art: status %d, cache %q", resp.StatusCode, resp.Header.Get("Cache-Control"))
	}
	resp, body = get(t, h, "/art/conflict")
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/svg+xml" || !strings.Contains(body, "Conflict") {
		t.Errorf("placeholder: status %d, type %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if resp, _ := get(t, h, "/art/nope"); resp.StatusCode != 404 {
		t.Errorf("unknown song art: status %d", resp.StatusCode)
	}
}

func TestAPIAndHealth(t *testing.T) {
	h := newTestServer(t, "/")
	resp, body := get(t, h, "/healthz")
	if resp.StatusCode != 200 || !strings.Contains(body, `"status": "ok"`) {
		t.Errorf("healthz: %d %s", resp.StatusCode, body)
	}
	_, body = get(t, h, "/api/data.json")
	var data struct {
		Stats struct{ Plays, Fails, Songs int }
		Art   art.Status
		Songs []apiSong
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatal(err)
	}
	if data.Stats.Plays != 36 || data.Stats.Fails != 7 || data.Stats.Songs != 15 || data.Art.Resolved != 1 || data.Art.Missing != 14 {
		t.Errorf("api stats = %+v art = %+v", data.Stats, data.Art)
	}
	songs := map[string]apiSong{}
	for _, s := range data.Songs {
		songs[s.Slug] = s
	}
	bd := songs["big-daddy"]
	if bd.Art != "image" || len(bd.Charts) != 1 || bd.Charts[0].Best.Score != 938204 || bd.Charts[0].Best.Plate != "TG" || len(bd.Charts[0].History) != 3 {
		t.Errorf("big daddy = %+v", bd)
	}
	duel := songs["duel"]
	if len(duel.Charts) != 1 {
		t.Fatalf("duel = %+v", duel)
	}
	c := duel.Charts[0]
	if c.Best != nil || c.Cleared || c.Plays != 2 || c.Clears != 0 || c.Fails != 2 || len(c.History) != 2 {
		t.Errorf("duel S13 = %+v", c)
	}
	// The raw JSON has explicit nulls for a fail with no result.
	if !strings.Contains(body, `"date": "2026-09-08",
              "score": null,
              "grade": null,
              "broken": true`) {
		t.Errorf("no-score play is not in the API with score and grade null")
	}
}

func TestBasePath(t *testing.T) {
	h := newTestServer(t, "/piu/")
	resp, body := get(t, h, "/piu/")
	if resp.StatusCode != 200 || !strings.Contains(body, `href="/piu/song/big-daddy"`) || !strings.Contains(body, `href="/piu/static/app.css?v=`) {
		t.Errorf("prefixed home: status %d", resp.StatusCode)
	}
	if resp, _ := get(t, h, "/piu"); resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "/piu/" {
		t.Errorf("/piu: status %d location %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, _ := get(t, h, "/piu/static/app.js"); resp.StatusCode != 200 {
		t.Errorf("prefixed static: status %d", resp.StatusCode)
	}
	if resp, _ := get(t, h, "/healthz"); resp.StatusCode != 200 {
		t.Errorf("unprefixed healthz: status %d", resp.StatusCode)
	}
	if resp, _ := get(t, h, "/song/big-daddy"); resp.StatusCode != 404 {
		t.Errorf("unprefixed page: status %d, want 404", resp.StatusCode)
	}
}

func TestStaticCaching(t *testing.T) {
	h := newTestServer(t, "/")
	_, home := get(t, h, "/")
	i := strings.Index(home, "/static/app.css?v=")
	versioned := home[i : i+len("/static/app.css?v=")+10]
	if resp, _ := get(t, h, versioned); !strings.Contains(resp.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("versioned asset cache = %q", resp.Header.Get("Cache-Control"))
	}
	if resp, _ := get(t, h, "/static/app.css"); resp.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("unversioned asset cache = %q", resp.Header.Get("Cache-Control"))
	}
}

func TestFormatting(t *testing.T) {
	if got := formatInt(938204); got != "938,204" {
		t.Errorf("formatInt = %q", got)
	}
	if got := signed(-4447); got != "−4,447" {
		t.Errorf("signed = %q", got)
	}
	now := time.Date(2026, 9, 28, 21, 0, 0, 0, time.UTC)
	for days, want := range map[int]string{0: "today", 1: "yesterday", 8: "8 days ago", 21: "3 weeks ago"} {
		if got := ago(now.AddDate(0, 0, -days), now); got != want {
			t.Errorf("ago(-%d) = %q, want %q", days, got, want)
		}
	}
}
