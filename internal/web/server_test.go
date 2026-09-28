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
		{"/", 200, []string{"PUMP IT UP", "Latest session", `href="/song/big-daddy"`, "Kasou Shinja 仮装信者", `data-modes="single"`, "Hardest clears"}},
		{"/song/big-daddy", 200, []string{"<h1 class=\"display\">Big Daddy</h1>", "938,204", "Talented Game", "graph-svg is-wide", "graph-svg is-narrow", "506", "31.1", "First clear"}},
		{"/song/nemesis", 200, []string{"Stage break", "died at the drill section", `data-tab="s16"`}},
		{"/song/destination", 200, []string{"CO-OP x2", "with Sam"}},
		{"/activity", 200, []string{"Sessions", "8 days at the cabinet", "134.0 kcal"}},
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

func TestChartsAreValidSVG(t *testing.T) {
	h := newTestServer(t, "/")
	_, body := get(t, h, "/song/conflict")
	n := 0
	for {
		start := strings.Index(body, `<svg class="graph-svg`)
		if start < 0 {
			break
		}
		end := strings.Index(body[start:], "</svg>") + start + len("</svg>")
		if err := xml.Unmarshal([]byte(body[start:end]), new(struct{})); err != nil {
			t.Fatalf("chart %d is not well-formed: %v", n, err)
		}
		body = body[end:]
		n++
	}
	// Two charts (S15, D13) × three metrics × two layouts.
	if n != 12 {
		t.Errorf("found %d chart SVGs, want 12", n)
	}
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
		Stats struct{ Plays, Songs int }
		Art   art.Status
		Songs []apiSong
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatal(err)
	}
	if data.Stats.Plays != 30 || data.Stats.Songs != 11 || data.Art.Resolved != 1 || data.Art.Missing != 10 {
		t.Errorf("api stats = %+v art = %+v", data.Stats, data.Art)
	}
	for _, s := range data.Songs {
		if s.Slug == "big-daddy" {
			if s.Art != "image" || len(s.Charts) != 1 || s.Charts[0].Best.Score != 938204 || s.Charts[0].Best.Plate != "TG" {
				t.Errorf("big daddy = %+v", s)
			}
			return
		}
	}
	t.Error("big-daddy missing from the API")
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
