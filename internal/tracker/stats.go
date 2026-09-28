package tracker

import "time"

// Stats summarises the score log. The headline figures cover one version,
// the newest one played (Tracker.Current), since scores, grades and levels
// of different versions do not compare.
type Stats struct {
	// Version is the version the headline figures cover.
	Version *Version
	Plays   int
	// Fails counts stage-broken plays, with or without a score.
	Fails  int
	Songs  int
	Charts int
	// PBsRecent counts personal bests set in the last 30 days, today
	// included, going by the calendar in now's time zone.
	PBsRecent int
	// HighestSingle and HighestDouble are the highest levels cleared, 0 if none.
	HighestSingle int
	HighestDouble int
	// Kcal is the sum over plays that recorded it, -1 when none did.
	Kcal      float64
	FirstPlay time.Time
	LastPlay  time.Time
	// BestGrades counts, per grade, the cleared charts whose best play
	// earned it, best grade first. Grades no chart has are left out.
	BestGrades []GradeCount
	// Ungraded counts the cleared charts whose best play has no grade: one
	// of a version whose grades are not worked out, logged without one.
	Ungraded int

	// Versions counts the plays of every version played, newest first.
	Versions []VersionCount
}

// ClearedCharts counts the cleared charts in the headline version.
func (s Stats) ClearedCharts() int {
	n := s.Ungraded
	for _, g := range s.BestGrades {
		n += g.Count
	}
	return n
}

// GradeCount is how many charts have a grade as their best.
type GradeCount struct {
	Grade Grade
	Count int
}

// VersionCount is how many plays were logged from a version.
type VersionCount struct {
	Version *Version
	Plays   int
	// Fails counts stage-broken plays, with or without a score.
	Fails int
}

// Stats computes the summary as of now.
func (t *Tracker) Stats(now time.Time) Stats {
	v := t.Current
	s := Stats{Version: v, Kcal: -1}
	// Play dates are on the player's clock, so compare calendar days rather
	// than instants.
	y, m, d := now.Date()
	recent := time.Date(y, m, d-29, 0, 0, 0, 0, time.UTC)
	for _, p := range t.Plays {
		if p.Version != v {
			continue
		}
		if s.Plays == 0 {
			s.FirstPlay = p.Date
		}
		s.Plays++
		s.LastPlay = p.Date
		if p.IsPB && !p.Date.Before(recent) {
			s.PBsRecent++
		}
		if p.Kcal >= 0 {
			s.Kcal = max(s.Kcal, 0) + p.Kcal
		}
		if p.Broken {
			s.Fails++
		}
	}
	counts := map[Grade]int{}
	for _, song := range t.Songs {
		charts := song.ChartsIn(v)
		if len(charts) > 0 {
			s.Songs++
		}
		for _, h := range charts {
			s.Charts++
			if r := h.Record; r.Best != nil && r.Cleared {
				if r.Best.Grade == "" {
					s.Ungraded++
				} else {
					counts[r.Best.Grade]++
				}
			}
			if h.Clears == 0 {
				continue
			}
			switch h.Chart.Mode {
			case ModeSingle, ModeSinglePerformance:
				s.HighestSingle = max(s.HighestSingle, h.Chart.Level)
			case ModeDouble, ModeDoublePerformance:
				s.HighestDouble = max(s.HighestDouble, h.Chart.Level)
			}
		}
	}
	for _, g := range v.Scoring.Grades() {
		if n := counts[g]; n > 0 {
			s.BestGrades = append(s.BestGrades, GradeCount{g, n})
		}
	}
	s.Versions = t.VersionCounts()
	return s
}

// VersionCounts counts the plays of every version played, newest first.
func (t *Tracker) VersionCounts() []VersionCount {
	var out []VersionCount
	for _, v := range t.PlayedVersions() {
		c := VersionCount{Version: v}
		for _, p := range t.Plays {
			if p.Version == v {
				c.Plays++
				if p.Broken {
					c.Fails++
				}
			}
		}
		out = append(out, c)
	}
	return out
}
