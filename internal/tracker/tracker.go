// Package tracker loads a player's Pump It Up score log and derives the
// per-song and per-chart history the web UI shows.
package tracker

import (
	"time"
)

// Tracker is a validated score log with everything derived from it.
type Tracker struct {
	Player string
	// Versions are the game versions the log was read with, in release order.
	Versions []*Version
	// Current is the newest version with at least one play, or the newest
	// version when there are no plays. The headline stats cover it.
	Current *Version
	// Songs sorted by title.
	Songs []*Song
	// Plays in chronological order.
	Plays []*Play
	// Days with at least one play, newest first.
	Days []*Day

	songsBySlug map[string]*Song
}

// Song is one song and every chart of it that has been played.
type Song struct {
	Title  string
	Slug   string
	Artist string
	BPM    string
	// Image is an image URL or a file name inside the custom art directory,
	// as configured for the song. Empty when not configured. Art is shared
	// by every version of the song.
	Image string

	// Version is the newest version the song has plays in.
	Version *Version
	// Charts played, one per chart and version: newest version first, then
	// by mode and level.
	Charts []*ChartHistory
	// Lineages holds each chart lineage once, in the order of their newest
	// charts.
	Lineages []*Lineage
	// Plays in chronological order.
	Plays []*Play
}

// ChartHistory is every play of one chart of a song in one game version.
// Charts of different versions are different charts, even at the same level,
// unless a play links them with `continues`.
type ChartHistory struct {
	Song    *Song
	Version *Version
	Chart   Chart
	// Plays in chronological order.
	Plays []*Play
	// Continues is the chart of an earlier version this one continues, nil
	// if it is not linked to one.
	Continues *ChartHistory
	// Lineage is the chart lineage the chart is part of.
	Lineage *Lineage
	// Record holds the personal bests the chart shares with the charts it is
	// linked to in versions with the same scoring system.
	Record *Record
	// Clears counts the chart's plays that were not stage-broken.
	Clears int
	// Fails counts the chart's stage-broken plays, with or without a score.
	Fails int
}

// Lineage is one step chart of a song followed through game versions: charts
// linked with `continues`, possibly at different levels. A chart that is not
// linked to another is a lineage of its own.
type Lineage struct {
	Song *Song
	// Charts, one per version, oldest version first.
	Charts []*ChartHistory
	// Records split the lineage where the scoring system changes, oldest
	// first. Scores are only compared within a record.
	Records []*Record
	// Plays in chronological order.
	Plays []*Play
	// Clears counts the plays that were not stage-broken.
	Clears int
	// Fails counts the stage-broken plays, with or without a score.
	Fails int
	// FewestMisses is the lowest Bad+Miss count among plays with judgments,
	// or -1 when no play recorded judgments. Misses do not depend on the
	// scoring system, so it covers the whole lineage.
	FewestMisses int
	// BestPerfectRate is the highest perfect percentage among plays with
	// judgments, or -1 when no play recorded judgments.
	BestPerfectRate float64
}

// Record is the personal bests of a chart lineage under one scoring system:
// the charts of consecutive versions in it that score plays the same way.
type Record struct {
	Lineage *Lineage
	Scoring ScoringSystem
	// Charts, oldest version first.
	Charts []*ChartHistory
	// Plays in chronological order.
	Plays []*Play
	// Best is the highest-scoring cleared play, or the highest-scoring broken
	// play when the record has no clear. It is nil when no play has a score,
	// i.e. every attempt failed with no result.
	Best *Play
	// Cleared reports whether any play was not stage-broken.
	Cleared bool
	// Clears counts the plays that were not stage-broken.
	Clears int
	// Fails counts the stage-broken plays, with or without a score.
	Fails int
	// BestPlate is the best plate across cleared plays, if any was recorded.
	BestPlate Plate
}

// Play is one recorded result screen.
type Play struct {
	Song    *Song
	Version *Version
	Chart   Chart
	// History is the chart history this play belongs to.
	History *ChartHistory

	// Date is the date and time of the play as logged, on the player's clock.
	// It is stored in UTC whatever the time zone, so that it reads back as
	// written.
	Date    time.Time
	HasTime bool

	// Score is -1 for a fail with no result ("-" on the result screen); such
	// a play is always Broken and has no grade, plate or judgments. Scores
	// are as the cabinet showed them, in the scale of the play's version.
	Score int
	// Grade is from the play's version's grade set. It is empty when the
	// play has no score, or when its version's grades cannot be worked out
	// and none was logged.
	Grade     Grade
	Plate     Plate
	Broken    bool
	Judgments *Judgments
	// MaxCombo is -1 when not recorded.
	MaxCombo int
	// Kcal is -1 when not recorded.
	Kcal float64
	Note string

	// IsPB reports whether this play set a new personal best on its record
	// (the first clear counts).
	IsPB bool
	// FirstClear reports whether this is the first cleared play of its record.
	FirstClear bool
	// PrevBest is the best cleared score on the record before this play, 0 if
	// none.
	PrevBest int

	index int
}

// HasScore reports whether the play has a result. Only a stage break can
// lack one.
func (p *Play) HasScore() bool { return p.Score >= 0 }

// Delta is the score difference to the previous personal best. It is only
// meaningful for cleared plays that are not a record's first clear.
func (p *Play) Delta() int { return p.Score - p.PrevBest }

// Day groups the plays of one calendar day.
type Day struct {
	Date  time.Time
	Plays []*Play
	PBs   int
	// Kcal is the sum over plays that recorded it, -1 when none did.
	Kcal float64
}

// Song returns the song with the given slug.
func (t *Tracker) Song(slug string) (*Song, bool) {
	s, ok := t.songsBySlug[slug]
	return s, ok
}

// Version returns the version with the given ID.
func (t *Tracker) Version(id string) (*Version, bool) {
	return versionSet(t.Versions).lookup(id)
}

// PlayedVersions lists the versions with at least one play, newest first.
func (t *Tracker) PlayedVersions() []*Version {
	played := map[*Version]bool{}
	for _, p := range t.Plays {
		played[p.Version] = true
	}
	var out []*Version
	for i := len(t.Versions) - 1; i >= 0; i-- {
		if played[t.Versions[i]] {
			out = append(out, t.Versions[i])
		}
	}
	return out
}

// DaysIn groups the plays of one version by day, newest first. A nil
// version groups every play, as Days does.
func (t *Tracker) DaysIn(v *Version) []*Day {
	if v == nil {
		return t.Days
	}
	var plays []*Play
	for _, p := range t.Plays {
		if p.Version == v {
			plays = append(plays, p)
		}
	}
	return groupDays(plays)
}

// LastPlayed returns the date of the most recent play of the song.
func (s *Song) LastPlayed() time.Time {
	if len(s.Plays) == 0 {
		return time.Time{}
	}
	return s.Plays[len(s.Plays)-1].Date
}

// ChartsIn returns the song's charts in version v, by mode and level.
func (s *Song) ChartsIn(v *Version) []*ChartHistory {
	var out []*ChartHistory
	for _, h := range s.Charts {
		if h.Version == v {
			out = append(out, h)
		}
	}
	return out
}

// PlaysIn returns the song's plays in version v, in chronological order.
func (s *Song) PlaysIn(v *Version) []*Play {
	var out []*Play
	for _, p := range s.Plays {
		if p.Version == v {
			out = append(out, p)
		}
	}
	return out
}

// Cleared reports whether any chart of the song in its newest version has
// been cleared.
func (s *Song) Cleared() bool { return s.ClearedIn(s.Version) }

// ClearedIn reports whether any chart of the song in version v has been
// cleared, counting clears carried over a chart link.
func (s *Song) ClearedIn(v *Version) bool {
	for _, h := range s.ChartsIn(v) {
		if h.Record.Cleared {
			return true
		}
	}
	return false
}

// BestGrade returns the best grade of the song in its newest version.
func (s *Song) BestGrade() Grade { return s.BestGradeIn(s.Version) }

// BestGradeIn returns the best grade among the personal bests of the song's
// charts in version v, from cleared charts, or from broken plays if none of
// them has been cleared. It is empty when no play has a score or grade.
func (s *Song) BestGradeIn(v *Version) Grade {
	var best Grade
	cleared := s.ClearedIn(v)
	for _, h := range s.ChartsIn(v) {
		r := h.Record
		if r.Best != nil && r.Cleared == cleared && GradeRank(v.Scoring, r.Best.Grade) > GradeRank(v.Scoring, best) {
			best = r.Best.Grade
		}
	}
	return best
}

// TopChart returns the hardest chart played in the song's newest version.
func (s *Song) TopChart() *ChartHistory { return s.TopChartIn(s.Version) }

// TopChartIn returns the hardest chart played in version v, preferring
// doubles over singles at the same level. A co-op chart's number is its
// player count rather than a level, so co-op charts are left out: it is nil
// when only they were played.
func (s *Song) TopChartIn(v *Version) *ChartHistory {
	var top *ChartHistory
	for _, c := range s.ChartsIn(v) {
		if c.Chart.Mode == ModeCoOp {
			continue
		}
		if top == nil || c.Chart.Level > top.Chart.Level || (c.Chart.Level == top.Chart.Level && c.Chart.Mode > top.Chart.Mode) {
			top = c
		}
	}
	return top
}

// LineageGroup is the lineages of a song whose newest chart is in one
// version.
type LineageGroup struct {
	Version  *Version
	Lineages []*Lineage
}

// LineageGroups groups the song's lineages by the version of their newest
// chart, newest version first. A linked lineage appears once, under its
// newest version.
func (s *Song) LineageGroups() []LineageGroup {
	var groups []LineageGroup
	for _, l := range s.Lineages {
		v := l.Newest().Version
		if len(groups) == 0 || groups[len(groups)-1].Version != v {
			groups = append(groups, LineageGroup{Version: v})
		}
		g := &groups[len(groups)-1]
		g.Lineages = append(g.Lineages, l)
	}
	return groups
}

// LastPlayed returns the date of the most recent play of the chart.
func (h *ChartHistory) LastPlayed() time.Time { return h.Plays[len(h.Plays)-1].Date }

// Key is a stable, URL-fragment friendly identifier of the chart within its
// song: the chart's key ("s11") in the default version, prefixed with the
// version otherwise ("prime2-s7").
func (h *ChartHistory) Key() string {
	if h.Version.IsDefault() {
		return h.Chart.Key()
	}
	return h.Version.ID + "-" + h.Chart.Key()
}

// Newest returns the lineage's chart in its newest version.
func (l *Lineage) Newest() *ChartHistory { return l.Charts[len(l.Charts)-1] }

// Record returns the record of the lineage's newest chart: its current
// personal bests.
func (l *Lineage) Record() *Record { return l.Records[len(l.Records)-1] }

// Key identifies the lineage within its song by its newest chart.
func (l *Lineage) Key() string { return l.Newest().Key() }

// Linked reports whether the lineage spans more than one version.
func (l *Lineage) Linked() bool { return len(l.Charts) > 1 }

// LastPlayed returns the date of the most recent play of the lineage.
func (l *Lineage) LastPlayed() time.Time { return l.Plays[len(l.Plays)-1].Date }

// LastPlayed returns the date of the most recent play of the record.
func (r *Record) LastPlayed() time.Time { return r.Plays[len(r.Plays)-1].Date }

// Versions lists the versions of the record's charts, oldest first.
func (r *Record) Versions() []*Version {
	out := make([]*Version, len(r.Charts))
	for i, h := range r.Charts {
		out[i] = h.Version
	}
	return out
}
