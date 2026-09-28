// Package art finds jacket art for songs and caches it on disk.
//
// For each song, art is looked up in this order:
//
//  1. the custom art directory: the file named by the song's image setting,
//     or a file named after the song's slug (big-daddy.png)
//  2. the song's image setting, when it is a URL
//  3. each configured Source (PIU Scores, then the PIU Fandom wiki)
//
// Found images are downloaded once into the cache directory. Songs for which
// nothing was found are retried after a while. Until a song has art, a
// generated placeholder is served in its place.
package art

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Song is what the resolver needs to know about a song.
type Song struct {
	Slug  string
	Title string
	// Image is a URL, or a file name inside the custom art directory.
	Image string
}

// Source proposes image URLs for a song.
type Source interface {
	Name() string
	// URLs returns candidate image URLs, most likely first.
	URLs(ctx context.Context, s Song) ([]string, error)
}

// Options configure a Resolver.
type Options struct {
	// CacheDir holds downloaded images. It must be writable when FetchEnabled.
	CacheDir string
	// CustomDir optionally holds images supplied by the user.
	CustomDir string
	// FetchEnabled allows downloading art from the internet.
	FetchEnabled bool
	Sources      []Source
	Client       *http.Client
	UserAgent    string
	// RetryMissAfter is how long to wait before looking again for a song
	// nothing was found for.
	RetryMissAfter time.Duration
	// RequestDelay spaces out requests to the art sources.
	RequestDelay time.Duration
	Logger       *slog.Logger
}

// Resolver resolves and serves jacket art.
type Resolver struct {
	opts Options

	mu      sync.RWMutex
	images  map[string]image
	pending int
	missing int
	done    chan struct{}
}

type image struct {
	path    string
	modTime time.Time
}

// meta is stored next to each cached image (or in place of one, when nothing
// was found) as <slug>.json.
type meta struct {
	// Wanted is the image URL configured for the song when this was fetched,
	// so that changing it triggers a new download.
	Wanted    string    `json:"wanted,omitempty"`
	Source    string    `json:"source,omitempty"`
	URL       string    `json:"url,omitempty"`
	File      string    `json:"file,omitempty"`
	Missing   bool      `json:"missing,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

const maxImageBytes = 10 << 20

// New creates a resolver. Call Start to resolve art for a set of songs.
func New(opts Options) *Resolver {
	if opts.Client == nil {
		opts.Client = &http.Client{Timeout: 20 * time.Second}
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if opts.RetryMissAfter == 0 {
		opts.RetryMissAfter = 7 * 24 * time.Hour
	}
	done := make(chan struct{})
	close(done)
	return &Resolver{opts: opts, images: map[string]image{}, done: done}
}

// Start resolves art from disk right away and, when fetching is enabled,
// downloads missing art in the background. It returns immediately.
func (r *Resolver) Start(ctx context.Context, songs []Song) {
	var queue []Song
	for _, s := range songs {
		if img, ok := r.fromCustomDir(s); ok {
			r.set(s.Slug, img)
			continue
		}
		m, img, ok := r.fromCache(s)
		if ok {
			r.set(s.Slug, img)
			continue
		}
		if m != nil && m.Missing && m.Wanted == remoteImage(s) && time.Since(m.CheckedAt) < r.opts.RetryMissAfter {
			r.mu.Lock()
			r.missing++
			r.mu.Unlock()
			continue
		}
		queue = append(queue, s)
	}
	if !r.opts.FetchEnabled || len(queue) == 0 {
		r.mu.Lock()
		r.missing += len(queue)
		r.mu.Unlock()
		return
	}
	r.mu.Lock()
	r.pending = len(queue)
	r.done = make(chan struct{})
	done := r.done
	r.mu.Unlock()
	go func() {
		defer close(done)
		r.fetchAll(ctx, queue)
	}()
}

// Wait blocks until background fetching started by Start has finished.
func (r *Resolver) Wait() {
	r.mu.RLock()
	done := r.done
	r.mu.RUnlock()
	<-done
}

// Status reports how many songs have art, are still being looked up, or have none.
type Status struct {
	Resolved int `json:"resolved"`
	Pending  int `json:"pending"`
	Missing  int `json:"missing"`
}

// Status returns the current resolution status.
func (r *Resolver) Status() Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return Status{Resolved: len(r.images), Pending: r.pending, Missing: r.missing}
}

// Lookup returns the image file for a song, if it has one.
func (r *Resolver) Lookup(slug string) (path string, modTime time.Time, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	img, ok := r.images[slug]
	return img.path, img.modTime, ok
}

// Version identifies the art currently served for a song, for cache busting.
func (r *Resolver) Version(slug string) string {
	if _, mod, ok := r.Lookup(slug); ok {
		return strconv.FormatInt(mod.Unix(), 36)
	}
	return "p"
}

func (r *Resolver) set(slug string, img image) {
	r.mu.Lock()
	r.images[slug] = img
	r.mu.Unlock()
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// remoteImage is the song's image setting when it is a URL.
func remoteImage(s Song) string {
	if isURL(s.Image) {
		return s.Image
	}
	return ""
}

var imageExtensions = []string{".png", ".jpg", ".jpeg", ".webp", ".gif", ".avif"}

func (r *Resolver) fromCustomDir(s Song) (image, bool) {
	if r.opts.CustomDir == "" {
		return image{}, false
	}
	var candidates []string
	if s.Image != "" && !isURL(s.Image) {
		// Clean against the root so that "../x" cannot leave the directory.
		candidates = append(candidates, filepath.Join(r.opts.CustomDir, filepath.Clean("/"+s.Image)))
	} else {
		for _, ext := range imageExtensions {
			candidates = append(candidates, filepath.Join(r.opts.CustomDir, s.Slug+ext))
		}
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.Mode().IsRegular() {
			return image{path: c, modTime: fi.ModTime()}, true
		}
	}
	if s.Image != "" && !isURL(s.Image) {
		r.opts.Logger.Warn("configured art file not found in the custom art directory; looking elsewhere", "song", s.Title, "file", s.Image)
	}
	return image{}, false
}

func (r *Resolver) metaPath(slug string) string {
	return filepath.Join(r.opts.CacheDir, slug+".json")
}

func (r *Resolver) fromCache(s Song) (*meta, image, bool) {
	if r.opts.CacheDir == "" {
		return nil, image{}, false
	}
	b, err := os.ReadFile(r.metaPath(s.Slug))
	if err != nil {
		return nil, image{}, false
	}
	var m meta
	if json.Unmarshal(b, &m) != nil {
		return nil, image{}, false
	}
	if m.Missing || m.File == "" || m.Wanted != remoteImage(s) {
		return &m, image{}, false
	}
	path := filepath.Join(r.opts.CacheDir, filepath.Base(m.File))
	fi, err := os.Stat(path)
	if err != nil {
		return &m, image{}, false
	}
	return &m, image{path: path, modTime: fi.ModTime()}, true
}

func (r *Resolver) fetchAll(ctx context.Context, songs []Song) {
	if err := os.MkdirAll(r.opts.CacheDir, 0o755); err != nil {
		r.opts.Logger.Error("cannot create the art cache directory; art will not be downloaded", "dir", r.opts.CacheDir, "err", err)
		r.mu.Lock()
		r.missing += r.pending
		r.pending = 0
		r.mu.Unlock()
		return
	}
	for _, s := range songs {
		if ctx.Err() != nil {
			return
		}
		found, err := r.fetch(ctx, s)
		r.mu.Lock()
		r.pending--
		if !found {
			r.missing++
		}
		r.mu.Unlock()
		if err != nil {
			r.opts.Logger.Warn("could not look up art", "song", s.Title, "err", err)
		}
	}
	st := r.Status()
	r.opts.Logger.Info("art lookup finished", "resolved", st.Resolved, "missing", st.Missing)
}

// fetch looks for art for one song. It records a miss only when every source
// answered that it has nothing, so that a network outage is retried on the
// next start rather than remembered for a week.
func (r *Resolver) fetch(ctx context.Context, s Song) (bool, error) {
	type candidate struct{ source, url string }
	var candidates []candidate
	var errs []error
	if u := remoteImage(s); u != "" {
		candidates = append(candidates, candidate{"image", u})
	}
	sourcesFailed := false
	for _, src := range r.opts.Sources {
		urls, err := src.URLs(ctx, s)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", src.Name(), err))
			sourcesFailed = true
		}
		for _, u := range urls {
			candidates = append(candidates, candidate{src.Name(), u})
		}
	}
	for _, c := range candidates {
		body, ext, notFound, err := r.download(ctx, c.url)
		if err != nil {
			if !notFound {
				sourcesFailed = true
				errs = append(errs, fmt.Errorf("%s: %w", c.source, err))
			}
			continue
		}
		img, err := r.store(s, c.source, c.url, body, ext)
		if err != nil {
			return false, err
		}
		r.set(s.Slug, img)
		r.opts.Logger.Info("found art", "song", s.Title, "source", c.source)
		return true, nil
	}
	if !sourcesFailed {
		r.writeMeta(s.Slug, meta{Wanted: remoteImage(s), Missing: true, CheckedAt: time.Now().UTC()})
		r.opts.Logger.Info("no art found; using a placeholder", "song", s.Title)
	}
	return false, errors.Join(errs...)
}

func (r *Resolver) download(ctx context.Context, url string) (body []byte, ext string, notFound bool, err error) {
	if r.opts.RequestDelay > 0 {
		select {
		case <-ctx.Done():
			return nil, "", false, ctx.Err()
		case <-time.After(r.opts.RequestDelay):
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", true, err
	}
	req.Header.Set("User-Agent", r.opts.UserAgent)
	req.Header.Set("Accept", "image/*")
	resp, err := r.opts.Client.Do(req)
	if err != nil {
		return nil, "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return nil, "", true, fmt.Errorf("%s: %s", url, resp.Status)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", false, fmt.Errorf("%s: %s", url, resp.Status)
	}
	body, err = io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", false, err
	}
	if len(body) > maxImageBytes {
		return nil, "", true, fmt.Errorf("%s: larger than %d bytes", url, maxImageBytes)
	}
	// Some hosts answer a missing file with an HTML page and status 200, so
	// trust the bytes rather than the headers.
	ext = sniff(body)
	if ext == "" {
		return nil, "", true, fmt.Errorf("%s: not an image", url)
	}
	return body, ext, false, nil
}

func sniff(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return ".png"
	case bytes.HasPrefix(b, []byte("\xff\xd8\xff")):
		return ".jpg"
	case bytes.HasPrefix(b, []byte("GIF87a")), bytes.HasPrefix(b, []byte("GIF89a")):
		return ".gif"
	case len(b) > 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return ".webp"
	case len(b) > 12 && bytes.Equal(b[4:8], []byte("ftyp")) && (bytes.Equal(b[8:12], []byte("avif")) || bytes.Equal(b[8:12], []byte("avis"))):
		return ".avif"
	}
	return ""
}

func (r *Resolver) store(s Song, source, url string, body []byte, ext string) (image, error) {
	final := filepath.Join(r.opts.CacheDir, s.Slug+ext)
	tmp, err := os.CreateTemp(r.opts.CacheDir, "."+s.Slug+"-*")
	if err != nil {
		return image{}, err
	}
	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return image{}, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return image{}, err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return image{}, err
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		os.Remove(tmp.Name())
		return image{}, err
	}
	for _, other := range imageExtensions {
		if other != ext {
			os.Remove(filepath.Join(r.opts.CacheDir, s.Slug+other))
		}
	}
	r.writeMeta(s.Slug, meta{Wanted: remoteImage(s), Source: source, URL: url, File: filepath.Base(final), CheckedAt: time.Now().UTC()})
	fi, err := os.Stat(final)
	if err != nil {
		return image{}, err
	}
	return image{path: final, modTime: fi.ModTime()}, nil
}

func (r *Resolver) writeMeta(slug string, m meta) {
	b, _ := json.MarshalIndent(m, "", "  ")
	path := r.metaPath(slug)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		r.opts.Logger.Warn("could not write art cache metadata", "path", path, "err", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		r.opts.Logger.Warn("could not write art cache metadata", "path", path, "err", err)
	}
}
