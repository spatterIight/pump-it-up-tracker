package web

import (
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spatterIight/pump-it-up-tracker/internal/catalog"
	"github.com/spatterIight/pump-it-up-tracker/internal/tracker"
)

// The progress pages measure the player against the game: PUMBILITY, and
// how much of each level folder they have cleared, from the chart list
// built into the app (see the catalog package).

// progressView is what the progress page shows for one version.
type progressView struct {
	Version *tracker.Version
	// Versions are the versions the page can show, newest first.
	Versions []*tracker.Version
	// Rating is the version's PUMBILITY, nil when it has none.
	Rating *tracker.Pumbility
	// Recent is how much it grew in the last 30 days.
	Recent float64
	// Groups are the level folders, singles then doubles, empty when there
	// is no chart list for the version.
	Groups []folderGroup
	// Next suggests charts to clear next.
	Next []nextGroup
}

// folderGroup is the level folders of one mode.
type folderGroup struct {
	Mode    tracker.Mode
	Folders []folderTile
}

// folderTile is one level folder, as the progress page shows it.
type folderTile struct {
	Ref     ballRef
	Key     string
	Cleared int
	Total   int
}

// Percent is the share of the folder cleared, from 0 to 100.
func (f folderTile) Percent() float64 {
	if f.Total == 0 {
		return 0
	}
	return float64(f.Cleared) * 100 / float64(f.Total)
}

// nextGroup is a few charts of one folder to try clearing next.
type nextGroup struct {
	Ref    ballRef
	Key    string
	Label  string
	Charts []folderRow
}

// folderView is what a level folder's page shows.
type folderView struct {
	Version  *tracker.Version
	Versions []*tracker.Version
	Ref      ballRef
	Key      string
	// Rows are the folder's charts, easiest to clear first, then the charts
	// of the level played that the list does not have.
	Rows    []folderRow
	Cleared int
	// Total counts the charts in the list.
	Total  int
	Rating *tracker.Pumbility
	// Prev and Next are the keys of the folders a level down and up, "" when
	// there is none.
	Prev, Next string
}

// folderRow is one chart of a level folder.
type folderRow struct {
	// Entry is the chart in the list, nil for a chart played that the list
	// does not have.
	Entry  *catalog.Chart
	Title  string
	Artist string
	// History is the chart's plays in the version, nil when it was never
	// played.
	History *tracker.ChartHistory
	Cleared bool
	// Gain is what clearing the chart at AA would add to PUMBILITY.
	Gain float64
}

// Attempts counts the chart's plays in the version.
func (r folderRow) Attempts() int {
	if r.History == nil {
		return 0
	}
	return len(r.History.Plays)
}

// ratingView is what the PUMBILITY page shows.
type ratingView struct {
	Version  *tracker.Version
	Versions []*tracker.Version
	Rating   *tracker.Pumbility
	Recent   float64
}

// progressVersions lists the versions played that the progress pages have
// something to show for, newest first: those with a PUMBILITY or a chart
// list.
func (s *Server) progressVersions() []*tracker.Version {
	var out []*tracker.Version
	for _, v := range s.opts.Tracker.PlayedVersions() {
		if v.Pumbility != nil || s.opts.Catalog.Mix(v) != nil {
			out = append(out, v)
		}
	}
	return out
}

// progressVersion is the version a progress page shows: the one the query
// string asks for, or else the newest played that there is something to
// show for; nil when there is none.
func (s *Server) progressVersion(r *http.Request) (*tracker.Version, []*tracker.Version) {
	vs := s.progressVersions()
	if len(vs) == 0 {
		return nil, nil
	}
	if v, ok := s.opts.Tracker.Version(r.URL.Query().Get("version")); ok && slices.Contains(vs, v) {
		return v, vs
	}
	return vs[0], vs
}

// recentGrowth is how much PUMBILITY grew in the 30 days up to now, today
// included, going by the calendar as the headline stats do.
func recentGrowth(p *tracker.Pumbility, now time.Time) float64 {
	y, m, d := now.Date()
	return p.Total - p.TotalOn(time.Date(y, m, d-30, 0, 0, 0, 0, time.UTC))
}

func (s *Server) progress(w http.ResponseWriter, r *http.Request) {
	p := s.newPage("Progress", "progress")
	v, vs := s.progressVersion(r)
	if v == nil {
		s.render(w, http.StatusOK, "progress", p)
		return
	}
	pv := &progressView{Version: v, Versions: vs, Rating: s.opts.Tracker.Pumbility(v)}
	if pv.Rating != nil {
		pv.Recent = recentGrowth(pv.Rating, p.Now)
	}
	if mix := s.opts.Catalog.Mix(v); mix != nil {
		folders := s.folders(v, mix, pv.Rating)
		for _, mode := range []tracker.Mode{tracker.ModeSingle, tracker.ModeDouble} {
			g := folderGroup{Mode: mode}
			top := 0
			for _, level := range mix.Levels(mode) {
				key := tracker.Chart{Mode: mode, Level: level}
				rows := folders[key]
				tile := folderTile{Ref: ballRef{v, key}, Key: key.Key()}
				for _, row := range rows {
					if row.Entry != nil {
						tile.Total++
						if row.Cleared {
							tile.Cleared++
						}
					}
					if row.Cleared {
						top = level
					}
				}
				g.Folders = append(g.Folders, tile)
			}
			pv.Groups = append(pv.Groups, g)
			if top > 0 {
				pv.Next = append(pv.Next, nextCharts(v, folders, tracker.Chart{Mode: mode, Level: top}, "Your hardest level cleared"))
				pv.Next = append(pv.Next, nextCharts(v, folders, tracker.Chart{Mode: mode, Level: top + 1}, "One level up"))
			}
		}
	}
	p.Title = v.Name + " progress"
	p.Progress = pv
	s.render(w, http.StatusOK, "progress", p)
}

// nextCharts are the easiest charts of a folder not cleared yet.
func nextCharts(v *tracker.Version, folders map[tracker.Chart][]folderRow, folder tracker.Chart, label string) nextGroup {
	g := nextGroup{Ref: ballRef{v, folder}, Key: folder.Key(), Label: label}
	for _, row := range folders[folder] {
		if !row.Cleared && row.Entry != nil && len(g.Charts) < 4 {
			g.Charts = append(g.Charts, row)
		}
	}
	return g
}

// folders sorts every chart of the list, and every chart played in version
// v that the list does not have, into level folders: easiest to clear first,
// then the charts the list does not have.
func (s *Server) folders(v *tracker.Version, mix *catalog.Mix, rating *tracker.Pumbility) map[tracker.Chart][]folderRow {
	played := map[*catalog.Chart]*tracker.ChartHistory{}
	var unlisted []*tracker.ChartHistory
	for _, song := range s.opts.Tracker.Songs {
		for _, h := range song.ChartsIn(v) {
			if e := mix.Lookup(song.Title, h.Chart); e != nil {
				played[e] = h
			} else if h.Chart.Mode == tracker.ModeSingle || h.Chart.Mode == tracker.ModeDouble {
				unlisted = append(unlisted, h)
			}
		}
	}
	out := map[tracker.Chart][]folderRow{}
	for _, e := range mix.Charts {
		row := folderRow{Entry: e, Title: e.Song, Artist: e.Artist, History: played[e]}
		if row.History != nil {
			row.Title = row.History.Song.Title
			row.Cleared = row.History.Record.Cleared
		}
		if !row.Cleared && rating != nil {
			row.Gain = rating.Gain(clearValue(v, e.Chart))
		}
		out[e.Chart] = append(out[e.Chart], row)
	}
	for key, rows := range out {
		slices.SortStableFunc(rows, func(a, b folderRow) int {
			if c := catalog.Ease(a.Entry, b.Entry); c != 0 {
				return c
			}
			return strings.Compare(strings.ToLower(a.Title), strings.ToLower(b.Title))
		})
		out[key] = rows
	}
	for _, h := range unlisted {
		row := folderRow{Title: h.Song.Title, Artist: s.songArtist(h.Song), History: h, Cleared: h.Record.Cleared}
		out[h.Chart] = append(out[h.Chart], row)
	}
	return out
}

// clearValue is what a chart cleared at AA with no plate is worth towards
// PUMBILITY in version v.
func clearValue(v *tracker.Version, c tracker.Chart) float64 {
	if v.Pumbility == nil {
		return 0
	}
	for _, t := range v.Scoring.GradeThresholds() {
		if t.Grade == "AA" {
			return v.Pumbility.Value(c, t.Min, t.Grade, "")
		}
	}
	return 0
}

func (s *Server) folder(w http.ResponseWriter, r *http.Request) {
	key, err := tracker.ParseChart(r.PathValue("folder"))
	if err != nil || (key.Mode != tracker.ModeSingle && key.Mode != tracker.ModeDouble) {
		s.notFound(w, r)
		return
	}
	v, vs := s.progressVersion(r)
	mix := s.opts.Catalog.Mix(v)
	levels := mix.Levels(key.Mode)
	i := slices.Index(levels, key.Level)
	if i < 0 {
		s.notFound(w, r)
		return
	}
	fv := &folderView{Version: v, Versions: vs, Ref: ballRef{v, key}, Key: key.Key(), Rating: s.opts.Tracker.Pumbility(v)}
	fv.Rows = s.folders(v, mix, fv.Rating)[key]
	for _, row := range fv.Rows {
		if row.Entry != nil {
			fv.Total++
			if row.Cleared {
				fv.Cleared++
			}
		}
	}
	if i > 0 {
		fv.Prev = tracker.Chart{Mode: key.Mode, Level: levels[i-1]}.Key()
	}
	if i < len(levels)-1 {
		fv.Next = tracker.Chart{Mode: key.Mode, Level: levels[i+1]}.Key()
	}
	p := s.newPage(key.Mode.Name()+" "+strconv.Itoa(key.Level)+" · "+v.Name, "progress")
	p.Folder = fv
	s.render(w, http.StatusOK, "folder", p)
}

func (s *Server) pumbility(w http.ResponseWriter, r *http.Request) {
	v, vs := s.progressVersion(r)
	var withRating []*tracker.Version
	for _, x := range vs {
		if x.Pumbility != nil {
			withRating = append(withRating, x)
		}
	}
	if v == nil || v.Pumbility == nil {
		if len(withRating) == 0 {
			s.notFound(w, r)
			return
		}
		v = withRating[0]
	}
	p := s.newPage("PUMBILITY · "+v.Name, "progress")
	rv := &ratingView{Version: v, Versions: withRating, Rating: s.opts.Tracker.Pumbility(v)}
	rv.Recent = recentGrowth(rv.Rating, p.Now)
	p.Rating = rv
	s.render(w, http.StatusOK, "pumbility", p)
}

// jacket serves a built-in jacket of a song that is not in the score log.
func (s *Server) jacket(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	f, err := s.opts.Art.OpenBundled(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, name, time.Time{}, rs)
}
