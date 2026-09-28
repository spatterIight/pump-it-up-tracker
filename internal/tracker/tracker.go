// Package tracker loads a player's Pump It Up score log and derives the
// per-song and per-chart history the web UI shows.
package tracker

import (
	"time"
)

// Tracker is a validated score log with everything derived from it.
type Tracker struct {
	Player string
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
	// as configured for the song. Empty when not configured.
	Image string

	// Charts sorted by mode, then level.
	Charts []*ChartHistory
	// Plays in chronological order.
	Plays []*Play
}

// ChartHistory is every play of one chart of a song.
type ChartHistory struct {
	Song  *Song
	Chart Chart
	// Plays in chronological order.
	Plays []*Play
	// Best is the highest-scoring cleared play, or the highest-scoring broken
	// play when the chart has never been cleared.
	Best *Play
	// Cleared reports whether any play was not stage-broken.
	Cleared bool
	// BestPlate is the best plate across cleared plays, if any was recorded.
	BestPlate Plate
	// FewestMisses is the lowest Bad+Miss count among plays with judgments,
	// or -1 when no play recorded judgments.
	FewestMisses int
	// BestPerfectRate is the highest perfect percentage among plays with
	// judgments, or -1 when no play recorded judgments.
	BestPerfectRate float64
}

// Play is one recorded result screen.
type Play struct {
	Song  *Song
	Chart Chart
	// History is the chart history this play belongs to.
	History *ChartHistory

	Date    time.Time
	HasTime bool

	Score     int
	Grade     Grade
	Plate     Plate
	Broken    bool
	Judgments *Judgments
	// MaxCombo is -1 when not recorded.
	MaxCombo int
	// Kcal is -1 when not recorded.
	Kcal float64
	Note string

	// IsPB reports whether this play set a new personal best on its chart
	// (the first clear of a chart counts).
	IsPB bool
	// FirstClear reports whether this is the first cleared play of its chart.
	FirstClear bool
	// PrevBest is the best cleared score on the chart before this play, 0 if none.
	PrevBest int

	index int
}

// Delta is the score difference to the previous personal best. It is only
// meaningful for cleared plays that are not a chart's first clear.
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

// LastPlayed returns the date of the most recent play of the song.
func (s *Song) LastPlayed() time.Time {
	if len(s.Plays) == 0 {
		return time.Time{}
	}
	return s.Plays[len(s.Plays)-1].Date
}

// BestGrade returns the best grade among the song's cleared plays, or its
// best broken grade if it has never been cleared.
func (s *Song) BestGrade() Grade {
	var best Grade
	for _, c := range s.Charts {
		if c.Best != nil && c.Best.Grade.Rank() > best.Rank() {
			best = c.Best.Grade
		}
	}
	return best
}

// TopChart returns the hardest chart played, preferring doubles over singles
// at the same level.
func (s *Song) TopChart() *ChartHistory {
	var top *ChartHistory
	for _, c := range s.Charts {
		if top == nil || c.Chart.Level > top.Chart.Level || (c.Chart.Level == top.Chart.Level && c.Chart.Mode > top.Chart.Mode) {
			top = c
		}
	}
	return top
}
