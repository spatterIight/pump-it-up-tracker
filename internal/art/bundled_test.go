package art

import (
	"context"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
)

// Every jacket built into the app is a WebP image that the index names, and
// every name in the index has its file.
func TestJacketsBuiltIn(t *testing.T) {
	b, err := Jackets()
	if err != nil {
		t.Fatal(err)
	}
	if b.Len() != 1080 {
		t.Errorf("%d jackets, want 1080", b.Len())
	}
	indexed := map[string]bool{}
	for name, file := range b.files {
		indexed[file] = true
		body, err := fs.ReadFile(b.fsys, file)
		if err != nil || sniff(body) != ".webp" {
			t.Errorf("%s: %s is not a WebP image (err %v)", name, file, err)
		}
	}
	files, _ := fs.Glob(b.fsys, "*.webp")
	for _, f := range files {
		if !indexed[f] {
			t.Errorf("%s is not in the index", f)
		}
	}
}

func TestBundledJacketLookup(t *testing.T) {
	b, err := Jackets()
	if err != nil {
		t.Fatal(err)
	}
	for title, want := range map[string]string{
		"Big Daddy":                              "BigDaddy.webp",
		"Rock the house":                         "Rockthehouse.webp",
		"Kasou Shinja 仮装信者":                      "KasouShinja.webp",
		"B3":                                     "B3.webp",
		"STEP":                                   "STEP.webp",
		"Step":                                   "Step-2.webp",
		"conflict":                               "Conflict.webp",
		"Nyan-turne":                             "Nyan-turne.webp",
		"Break Through Myself feat. Risa Yuzuki": "BreakThroughMyself.webp",
		"Bad Apple!! feat. Nomico":               "BadApplefeatNomico.webp",
		"P Maniac":                               "p_maniac.webp",
		"step":                                   "Step-2.webp", // capitalised, as PIU Scores names it
		"sTeP":                                   "",            // STEP or Step: too close to call
		"Not a Pump It Up song":                  "",
		"仮装信者":                                   "",
	} {
		if got, _ := b.byTitle(title); got != want {
			t.Errorf("%q: jacket %q, want %q", title, got, want)
		}
	}
	for u, want := range map[string]string{
		piuScoresSongs + "0594e1f3-9bdb-418c-991a-472454bed934.png": "0594e1f3-9bdb-418c-991a-472454bed934.webp",
		piuScoresSongs + "B3-p2.png":                                "B3-p2.webp",
		piuScoresSongs + "Step.png":                                 "Step-2.webp",
		piuScoresSongs + "Nothing.png":                              "",
		"https://example.com/songs/BigDaddy.png":                    "",
	} {
		if got, _ := b.byURL(u); got != want {
			t.Errorf("%s: jacket %q, want %q", u, got, want)
		}
	}
}

// The song's own settings come first, then the bundle, then the cache and
// the internet.
func TestResolverPrefersBundledJackets(t *testing.T) {
	webp := "RIFF\x00\x00\x00\x00WEBPVP8 "
	bundle, err := LoadBundle(fstest.MapFS{
		"index.json":    {Data: []byte(`{"BigDaddy": "BigDaddy.webp", "Conflict": "Conflict.webp", "0594e1f3": "0594e1f3.webp"}`)},
		"BigDaddy.webp": {Data: []byte(webp + "big daddy")},
		"Conflict.webp": {Data: []byte(webp + "conflict")},
		"0594e1f3.webp": {Data: []byte(webp + "guid")},
	})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var requested []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requested = append(requested, r.URL.Path)
		mu.Unlock()
		w.Write(pngBytes)
	}))
	defer srv.Close()

	custom, cache := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(custom, "conflict.png"), pngBytes, 0o644)
	// A jacket downloaded before the bundle existed.
	os.WriteFile(filepath.Join(cache, "big-daddy.png"), pngBytes, 0o644)
	os.WriteFile(filepath.Join(cache, "big-daddy.json"), []byte(`{"source": "fandom", "file": "big-daddy.png", "checked_at": "2026-09-01T00:00:00Z"}`), 0o644)

	newResolver := func(fetch bool) *Resolver {
		return New(Options{CacheDir: cache, CustomDir: custom, Bundle: bundle, FetchEnabled: fetch,
			Sources: []Source{sourceFunc(func(s Song) []string { return []string{srv.URL + "/" + alnum(s.Title) + ".png"} })}})
	}
	songs := []Song{
		{Slug: "big-daddy", Title: "Big Daddy"},
		{Slug: "conflict", Title: "Conflict"},
		{Slug: "pinned", Title: "Pinned", Image: piuScoresSongs + "0594e1f3.png"},
		{Slug: "own-url", Title: "Big Daddy", Image: srv.URL + "/own.png"},
		{Slug: "online", Title: "Online"},
	}
	r := newResolver(true)
	r.Start(context.Background(), songs)
	r.Wait()

	for slug, want := range map[string]string{
		"big-daddy": "big daddy", // the bundle, over the cached download
		"conflict":  "",          // the custom art directory, over the bundle
		"pinned":    "guid",      // a PIU Scores URL the bundle has
		"own-url":   "",          // any other URL, over the bundle
		"online":    "",          // the internet, for a song the bundle lacks
	} {
		img, ok := r.Lookup(slug)
		if !ok {
			t.Errorf("%s: no art", slug)
			continue
		}
		if img.Bundled() != (want != "") {
			t.Errorf("%s: bundled = %v, want %v", slug, img.Bundled(), want != "")
		}
		if want == "" {
			continue
		}
		f, err := img.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(f)
		f.Close()
		if !strings.HasSuffix(string(body), want) || !strings.HasSuffix(img.Name(), ".webp") || !strings.HasPrefix(r.Version(slug), "b") {
			t.Errorf("%s: served %q as %s, version %q", slug, body, img.Name(), r.Version(slug))
		}
	}
	if strings.Join(requested, " ") != "/own.png /Online.png" {
		t.Errorf("requested %v, want only /own.png and /Online.png", requested)
	}

	// With fetching off, the bundle still works.
	offline := newResolver(false)
	offline.Start(context.Background(), []Song{{Slug: "conflict-2", Title: "Conflict"}})
	if img, ok := offline.Lookup("conflict-2"); !ok || !img.Bundled() {
		t.Errorf("offline: %+v, %v", img, ok)
	}
}
