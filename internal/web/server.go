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

	Stats  tracker.Stats
	Songs  []*tracker.Song
	Latest *tracker.Day
	Song   *tracker.Song
	Days   []*tracker.Day
}

func (s *Server) newPage(title, nav string) *page {
	return &page{
		Base:         s.base,
		Title:        title,
		Nav:          nav,
		Player:       s.opts.Tracker.Player,
		AssetVersion: s.assetVersion,
		AppVersion:   s.opts.Version,
		Now:          s.opts.Now(),
	}
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
	p.Songs = t.Songs
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
	p.Stats = s.opts.Tracker.Stats(p.Now)
	p.Days = s.opts.Tracker.Days
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

type apiBest struct {
	Score int    `json:"score"`
	Grade string `json:"grade"`
	Plate string `json:"plate,omitempty"`
	Date  string `json:"date"`
}

// apiPlay is one play. Score and Grade are null for a fail with no result.
type apiPlay struct {
	Date   string  `json:"date"`
	Score  *int    `json:"score"`
	Grade  *string `json:"grade"`
	Plate  string  `json:"plate,omitempty"`
	Broken bool    `json:"broken"`
	PB     bool    `json:"pb"`
}

type apiChart struct {
	Chart   string `json:"chart"`
	Plays   int    `json:"plays"`
	Clears  int    `json:"clears"`
	Fails   int    `json:"fails"`
	Cleared bool   `json:"cleared"`
	// Best is null when no play has a score.
	Best    *apiBest  `json:"best"`
	History []apiPlay `json:"history"`
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
				Plays:   len(h.Plays),
				Clears:  h.Clears,
				Fails:   h.Fails,
				Cleared: h.Cleared,
				History: make([]apiPlay, 0, len(h.Plays)),
			}
			if h.Best != nil {
				ac.Best = &apiBest{
					Score: h.Best.Score,
					Grade: string(h.Best.Grade),
					Plate: string(h.Best.Plate),
					Date:  h.Best.Date.Format("2006-01-02"),
				}
			}
			for _, p := range h.Plays {
				ap := apiPlay{Date: p.Date.Format("2006-01-02"), Plate: string(p.Plate), Broken: p.Broken, PB: p.IsPB}
				if p.HasTime {
					ap.Date = p.Date.Format("2006-01-02T15:04")
				}
				if p.HasScore() {
					score, grade := p.Score, string(p.Grade)
					ap.Score, ap.Grade = &score, &grade
				}
				ac.History = append(ac.History, ap)
			}
			as.Charts = append(as.Charts, ac)
		}
		songs = append(songs, as)
	}
	writeJSON(w, map[string]any{
		"player":  t.Player,
		"version": s.opts.Version,
		"stats": map[string]any{
			"plays":          stats.Plays,
			"fails":          stats.Fails,
			"songs":          stats.Songs,
			"charts":         stats.Charts,
			"highest_single": stats.HighestSingle,
			"highest_double": stats.HighestDouble,
		},
		"art":   s.opts.Art.Status(),
		"songs": songs,
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}
