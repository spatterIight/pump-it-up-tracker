// Package web serves the tracker's read-only web UI.
package web

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/spatterIight/pump-it-up-tracker/internal/art"
	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

// Options configure a Server.
type Options struct {
	Tracker *tracker.Tracker
	Art     *art.Resolver
	// BasePath is the path prefix the UI is served under, such as "/piu".
	// Empty or "/" serves it at the root.
	BasePath string
	Version  string
	Logger   *slog.Logger
	// Now returns the current time; it defaults to time.Now.
	Now func() time.Time
}

// Server renders the UI.
type Server struct {
	opts         Options
	base         string
	pages        map[string]*template.Template
	static       fs.FS
	assetVersion string
}

// NormalizeBasePath turns "/", "" and "piu/" into "" and "/piu".
func NormalizeBasePath(p string) string {
	p = strings.Trim(strings.TrimSpace(p), "/")
	if p == "" {
		return ""
	}
	return "/" + p
}

// New prepares a server.
func New(opts Options) (*Server, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	s := &Server{opts: opts, base: NormalizeBasePath(opts.BasePath), pages: map[string]*template.Template{}}

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	s.static = static
	h := sha256.New()
	fs.WalkDir(static, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := fs.ReadFile(static, p)
			h.Write([]byte(p))
			h.Write(b)
		}
		return nil
	})
	s.assetVersion = hex.EncodeToString(h.Sum(nil))[:10]

	funcs := s.funcs()
	for _, name := range []string{"home", "song", "activity", "notfound"} {
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/partials.html", "templates/"+name+".html")
		if err != nil {
			return nil, err
		}
		s.pages[name] = t
	}
	return s, nil
}

// Handler returns the HTTP handler for the whole UI.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /song/{slug}", s.song)
	mux.HandleFunc("GET /activity", s.activity)
	mux.HandleFunc("GET /art/{slug}", s.art)
	mux.HandleFunc("GET /api/data.json", s.data)
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.staticHandler()))
	mux.HandleFunc("/", s.notFound)

	root := http.NewServeMux()
	if s.base == "" {
		root.Handle("/", mux)
	} else {
		root.Handle(s.base+"/", http.StripPrefix(s.base, mux))
		root.Handle("GET "+s.base+"", http.RedirectHandler(s.base+"/", http.StatusMovedPermanently))
		// The container healthcheck probes the unprefixed path.
		root.HandleFunc("GET /healthz", s.healthz)
		root.HandleFunc("/", s.notFound)
	}
	return securityHeaders(root)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; font-src 'self'; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'self'")
		next.ServeHTTP(w, r)
	})
}

// page is what every template receives.
type page struct {
	Base         string
	Title        string
	Nav          string
	Player       string
	AssetVersion string
	AppVersion   string
	Now          time.Time

	// Current is the newest version played, which the headline stats cover.
	Current *tracker.Version
	// Played lists the versions with plays, newest first.
	Played []*tracker.Version
	// Filter is the version the songs list or the activity is narrowed to,
	// nil for every version.
	Filter *tracker.Version

	Stats   tracker.Stats
	Cards   []songCard
	Latest  *tracker.Day
	Song    *tracker.Song
	Days    []*tracker.Day
	Summary summary
}

// songCard is a song as the songs list shows it: in one version, the newest
// it was played in, or the version the list is filtered to.
type songCard struct {
	Song    *tracker.Song
	Version *tracker.Version
	// Charts played in the version.
	Charts    []*tracker.ChartHistory
	Cleared   bool
	BestGrade tracker.Grade
	Top       *tracker.ChartHistory
	// Plays counts the song's plays in every version, or in the version
	// the list is filtered to.
	Plays int
	// Attempts counts the plays in the card's version.
	Attempts int
	Last     time.Time
	// Rank orders cards by best grade: the newest version's first, then by
	// grade within a version. It is 0 for a song not cleared.
	Rank int
}

// summary adds up a list of days.
type summary struct {
	Plays, Fails int
	// Kcal is the sum over plays that recorded it, -1 when none did.
	Kcal float64
}

func (s *Server) newPage(title, nav string) *page {
	t := s.opts.Tracker
	return &page{
		Base:         s.base,
		Title:        title,
		Nav:          nav,
		Player:       t.Player,
		AssetVersion: s.assetVersion,
		AppVersion:   s.opts.Version,
		Now:          s.opts.Now(),
		Current:      t.Current,
		Played:       t.PlayedVersions(),
	}
}

// filter reads the version a list is narrowed to from the query string. An
// unknown version, or one without plays, shows every version.
func (s *Server) filter(r *http.Request) *tracker.Version {
	id := r.URL.Query().Get("version")
	if id == "" {
		return nil
	}
	v, ok := s.opts.Tracker.Version(id)
	if !ok {
		return nil
	}
	for _, played := range s.opts.Tracker.PlayedVersions() {
		if played == v {
			return v
		}
	}
	return nil
}

func (s *Server) cards(filter *tracker.Version) []songCard {
	t := s.opts.Tracker
	var cards []songCard
	for _, song := range t.Songs {
		c := songCard{Song: song, Version: song.Version, Plays: len(song.Plays), Last: song.LastPlayed()}
		if filter != nil {
			plays := song.PlaysIn(filter)
			if len(plays) == 0 {
				continue
			}
			c.Version, c.Plays, c.Last = filter, len(plays), plays[len(plays)-1].Date
		}
		c.Charts = song.ChartsIn(c.Version)
		c.Attempts = len(song.PlaysIn(c.Version))
		c.Cleared = song.ClearedIn(c.Version)
		c.BestGrade = song.BestGradeIn(c.Version)
		c.Top = song.TopChartIn(c.Version)
		if c.Cleared {
			c.Rank = tracker.GradeRank(c.Version.Scoring, c.BestGrade)
			for _, v := range t.Versions {
				if v.Before(c.Version) {
					c.Rank += 100
				}
			}
		}
		cards = append(cards, c)
	}
	return cards
}

func (s *Server) render(w http.ResponseWriter, status int, name string, p *page) {
	var buf bytes.Buffer
	if err := s.pages[name].Execute(&buf, p); err != nil {
		s.opts.Logger.Error("rendering page failed", "page", name, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	p := s.newPage("", "songs")
	t := s.opts.Tracker
	p.Stats = t.Stats(p.Now)
	p.Filter = s.filter(r)
	p.Cards = s.cards(p.Filter)
	if len(t.Days) > 0 {
		p.Latest = t.Days[0]
	}
	s.render(w, http.StatusOK, "home", p)
}

func (s *Server) song(w http.ResponseWriter, r *http.Request) {
	song, ok := s.opts.Tracker.Song(r.PathValue("slug"))
	if !ok {
		s.notFound(w, r)
		return
	}
	p := s.newPage(song.Title, "songs")
	p.Song = song
	s.render(w, http.StatusOK, "song", p)
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	p := s.newPage("Activity", "activity")
	p.Filter = s.filter(r)
	p.Days = s.opts.Tracker.DaysIn(p.Filter)
	p.Summary.Kcal = -1
	for _, d := range p.Days {
		p.Summary.Plays += len(d.Plays)
		for _, pl := range d.Plays {
			if pl.Broken {
				p.Summary.Fails++
			}
		}
		if d.Kcal >= 0 {
			p.Summary.Kcal = max(p.Summary.Kcal, 0) + d.Kcal
		}
	}
	s.render(w, http.StatusOK, "activity", p)
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusNotFound, "notfound", s.newPage("Not found", ""))
}

func (s *Server) art(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	song, ok := s.opts.Tracker.Song(slug)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if p, mod, ok := s.opts.Art.Lookup(slug); ok {
		f, err := os.Open(p)
		if err == nil {
			defer f.Close()
			w.Header().Set("Cache-Control", "public, max-age=604800")
			http.ServeContent(w, r, path.Base(p), mod, f)
			return
		}
		s.opts.Logger.Warn("art file vanished; serving a placeholder", "song", song.Title, "err", err)
	}
	w.Header().Set("Content-Type", "image/svg+xml")
	// Not cached, so that real art replaces it once it has been downloaded.
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(art.Placeholder(song.Title))
}

func (s *Server) staticHandler() http.Handler {
	files := http.FileServerFS(s.static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only files are served, not directory listings.
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("v") == s.assetVersion {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"status": "ok",
		"songs":  len(s.opts.Tracker.Songs),
		"plays":  len(s.opts.Tracker.Plays),
	})
}

// apiBest is a chart's personal best. It can be a play of a linked chart
// of another version with the same scoring system, which Version and Chart
// name.
type apiBest struct {
	Score   int     `json:"score"`
	Grade   *string `json:"grade"`
	Plate   string  `json:"plate,omitempty"`
	Date    string  `json:"date"`
	Version string  `json:"version"`
	Chart   string  `json:"chart"`
}

// apiPlay is one play. Score and Grade are null for a fail with no result;
// Grade is also null when the play's version's grade is unknown.
type apiPlay struct {
	Date    string  `json:"date"`
	Version string  `json:"version"`
	Score   *int    `json:"score"`
	Grade   *string `json:"grade"`
	Plate   string  `json:"plate,omitempty"`
	Broken  bool    `json:"broken"`
	PB      bool    `json:"pb"`
}

// apiLineageChart is one chart of a lineage.
type apiLineageChart struct {
	Version string `json:"version"`
	Chart   string `json:"chart"`
}

// apiChart is one chart of one version.
type apiChart struct {
	Chart   string `json:"chart"`
	Version string `json:"version"`
	// Lineage lists the charts linked to this one, itself included, oldest
	// version first.
	Lineage []apiLineageChart `json:"lineage"`
	Plays   int               `json:"plays"`
	Clears  int               `json:"clears"`
	Fails   int               `json:"fails"`
	// Cleared and Best carry across links between versions with the same
	// scoring system. Best is null when no play has a score.
	Cleared bool      `json:"cleared"`
	Best    *apiBest  `json:"best"`
	History []apiPlay `json:"history"`
}

type apiVersion struct {
	Version string `json:"version"`
	Name    string `json:"name"`
	Plays   int    `json:"plays"`
	Fails   int    `json:"fails"`
}

type apiSong struct {
	Title  string     `json:"title"`
	Slug   string     `json:"slug"`
	Artist string     `json:"artist,omitempty"`
	Art    string     `json:"art"`
	Charts []apiChart `json:"charts"`
}

// data is a machine-readable summary, handy for checking what was loaded.
func (s *Server) data(w http.ResponseWriter, r *http.Request) {
	t := s.opts.Tracker
	stats := t.Stats(s.opts.Now())
	songs := make([]apiSong, 0, len(t.Songs))
	for _, song := range t.Songs {
		as := apiSong{Title: song.Title, Slug: song.Slug, Artist: song.Artist, Art: "placeholder"}
		if _, _, ok := s.opts.Art.Lookup(song.Slug); ok {
			as.Art = "image"
		}
		for _, h := range song.Charts {
			ac := apiChart{
				Chart:   h.Chart.String(),
				Version: h.Version.ID,
				Plays:   len(h.Plays),
				Clears:  h.Clears,
				Fails:   h.Fails,
				Cleared: h.Record.Cleared,
				History: make([]apiPlay, 0, len(h.Plays)),
			}
			for _, lh := range h.Lineage.Charts {
				ac.Lineage = append(ac.Lineage, apiLineageChart{Version: lh.Version.ID, Chart: lh.Chart.String()})
			}
			if b := h.Record.Best; b != nil {
				ac.Best = &apiBest{
					Score:   b.Score,
					Grade:   apiGrade(b.Grade),
					Plate:   string(b.Plate),
					Date:    b.Date.Format("2006-01-02"),
					Version: b.Version.ID,
					Chart:   b.Chart.String(),
				}
			}
			for _, p := range h.Plays {
				ap := apiPlay{Date: p.Date.Format("2006-01-02"), Version: p.Version.ID, Plate: string(p.Plate), Broken: p.Broken, PB: p.IsPB}
				if p.HasTime {
					ap.Date = p.Date.Format("2006-01-02T15:04")
				}
				if p.HasScore() {
					score := p.Score
					ap.Score, ap.Grade = &score, apiGrade(p.Grade)
				}
				ac.History = append(ac.History, ap)
			}
			as.Charts = append(as.Charts, ac)
		}
		songs = append(songs, as)
	}
	versions := make([]apiVersion, 0, len(stats.Versions))
	for _, c := range stats.Versions {
		versions = append(versions, apiVersion{Version: c.Version.ID, Name: c.Version.Name, Plays: c.Plays, Fails: c.Fails})
	}
	writeJSON(w, map[string]any{
		"player":  t.Player,
		"version": s.opts.Version,
		// The headline stats cover the newest version played.
		"stats": map[string]any{
			"version":        stats.Version.ID,
			"plays":          stats.Plays,
			"fails":          stats.Fails,
			"songs":          stats.Songs,
			"charts":         stats.Charts,
			"highest_single": stats.HighestSingle,
			"highest_double": stats.HighestDouble,
		},
		"versions": versions,
		"art":      s.opts.Art.Status(),
		"songs":    songs,
	})
}

// apiGrade is a grade for the API: null when unknown.
func apiGrade(g tracker.Grade) *string {
	if g == "" {
		return nil
	}
	s := string(g)
	return &s
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}
