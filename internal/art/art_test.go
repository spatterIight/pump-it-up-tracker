package art

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var pngBytes = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func TestPIUScoresNames(t *testing.T) {
	cases := map[string][]string{
		"Big Daddy":                    {"BigDaddy"},
		"Rock the house":               {"Rockthehouse", "RockTheHouse"},
		"Kasou Shinja 仮装信者":            {"KasouShinja"},
		"Love is a Danger Zone":        {"LoveisaDangerZone", "LoveIsADangerZone"},
		"U Got 2 Know":                 {"UGot2Know"},
		"Ignis Fatuus (DM Ashura Mix)": {"IgnisFatuusDMAshuraMix"},
	}
	for title, want := range cases {
		urls, _ := PIUScores{BaseURL: "x/"}.URLs(context.Background(), Song{Title: title})
		var got []string
		for _, u := range urls {
			got = append(got, strings.TrimSuffix(strings.TrimPrefix(u, "x/"), ".png"))
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%q: got %v, want %v", title, got, want)
		}
	}
}

func TestFandomSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("titles") != "Conflict" || r.URL.Query().Get("prop") != "pageimages" {
			t.Errorf("unexpected query %s", r.URL.RawQuery)
		}
		w.Write([]byte(`{"query":{"pages":[{"title":"Conflict","original":{"source":"https://static.example/Conflict.png"}}]}}`))
	}))
	defer srv.Close()
	urls, err := Fandom{APIURL: srv.URL}.URLs(context.Background(), Song{Title: "Conflict"})
	if err != nil || len(urls) != 1 || urls[0] != "https://static.example/Conflict.png" {
		t.Fatalf("urls = %v, err = %v", urls, err)
	}
}

type fakeSource struct {
	name string
	urls []string
}

func (f fakeSource) Name() string                                 { return f.name }
func (f fakeSource) URLs(context.Context, Song) ([]string, error) { return f.urls, nil }

func TestResolverDownloadsCachesAndRemembersMisses(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/BigDaddy.png":
			w.Write(pngBytes)
		case "/html.png":
			w.Write([]byte("<html>not found</html>"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cache := t.TempDir()
	newResolver := func() *Resolver {
		return New(Options{
			CacheDir:     cache,
			FetchEnabled: true,
			Sources: []Source{fakeSource{"test", nil}, sourceFunc(func(s Song) []string {
				return []string{srv.URL + "/html.png", srv.URL + "/" + alnum(s.Title) + ".png"}
			})},
		})
	}
	songs := []Song{{Slug: "big-daddy", Title: "Big Daddy"}, {Slug: "nothing", Title: "Nothing"}}

	r := newResolver()
	r.Start(context.Background(), songs)
	r.Wait()
	if st := r.Status(); st.Resolved != 1 || st.Missing != 1 || st.Pending != 0 {
		t.Fatalf("status = %+v", st)
	}
	img, ok := r.Lookup("big-daddy")
	if !ok || img.Name() != "big-daddy.png" || img.Bundled() {
		t.Fatalf("lookup = %+v, %v", img, ok)
	}
	if b, _ := os.ReadFile(img.path); string(b) != string(pngBytes) {
		t.Fatalf("cached bytes = %q", b)
	}
	if r.Version("big-daddy") == "p" || r.Version("nothing") != "p" {
		t.Errorf("versions = %q, %q", r.Version("big-daddy"), r.Version("nothing"))
	}

	// A second start is served entirely from the cache, misses included.
	before := hits.Load()
	r2 := newResolver()
	r2.Start(context.Background(), songs)
	r2.Wait()
	if hits.Load() != before {
		t.Errorf("second start made %d requests, want none", hits.Load()-before)
	}
	if st := r2.Status(); st.Resolved != 1 || st.Missing != 1 {
		t.Errorf("second status = %+v", st)
	}

	// Configuring an image URL for the missing song looks it up again.
	r3 := newResolver()
	r3.Start(context.Background(), []Song{{Slug: "nothing", Title: "Nothing", Image: srv.URL + "/BigDaddy.png"}})
	r3.Wait()
	if _, ok := r3.Lookup("nothing"); !ok {
		t.Errorf("image URL override was not downloaded")
	}
}

type sourceFunc func(Song) []string

func (sourceFunc) Name() string                                       { return "func" }
func (f sourceFunc) URLs(_ context.Context, s Song) ([]string, error) { return f(s), nil }

func TestResolverDoesNotRememberNetworkFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	cache := t.TempDir()
	r := New(Options{CacheDir: cache, FetchEnabled: true, Sources: []Source{fakeSource{"test", []string{srv.URL + "/a.png"}}}})
	r.Start(context.Background(), []Song{{Slug: "a", Title: "A"}})
	r.Wait()
	if _, err := os.Stat(filepath.Join(cache, "a.json")); !os.IsNotExist(err) {
		t.Errorf("a 503 should not be remembered as a miss (stat err = %v)", err)
	}
}

func TestResolverCustomDirAndFetchDisabled(t *testing.T) {
	custom := t.TempDir()
	os.WriteFile(filepath.Join(custom, "big-daddy.jpg"), []byte("\xff\xd8\xff"), 0o644)
	os.WriteFile(filepath.Join(custom, "named.png"), pngBytes, 0o644)
	r := New(Options{CacheDir: t.TempDir(), CustomDir: custom, RetryMissAfter: time.Hour})
	r.Start(context.Background(), []Song{
		{Slug: "big-daddy", Title: "Big Daddy"},
		{Slug: "other", Title: "Other", Image: "named.png"},
		{Slug: "escape", Title: "Escape", Image: "../../etc/passwd"},
		{Slug: "none", Title: "None"},
	})
	r.Wait()
	if img, ok := r.Lookup("big-daddy"); !ok || img.Name() != "big-daddy.jpg" {
		t.Errorf("slug-named custom art: %q %v", img.Name(), ok)
	}
	if img, ok := r.Lookup("other"); !ok || img.Name() != "named.png" {
		t.Errorf("configured custom art: %q %v", img.Name(), ok)
	}
	if _, ok := r.Lookup("escape"); ok {
		t.Errorf("path traversal resolved")
	}
	if st := r.Status(); st.Resolved != 2 || st.Missing != 2 || st.Pending != 0 {
		t.Errorf("status = %+v", st)
	}
}

func TestPlaceholderIsValidSVG(t *testing.T) {
	for _, title := range []string{"Big Daddy", "Kasou Shinja 仮装信者", `<script>&"'`, "A Very Long Song Title That Needs Several Lines To Fit In"} {
		svg := Placeholder(title)
		if err := xml.Unmarshal(svg, new(struct{})); err != nil {
			t.Errorf("%q: invalid XML: %v", title, err)
		}
		if strings.Contains(string(svg), "<script>") {
			t.Errorf("%q: title not escaped", title)
		}
	}
	if string(Placeholder("A")) != string(Placeholder("A")) {
		t.Errorf("placeholder is not deterministic")
	}
}
