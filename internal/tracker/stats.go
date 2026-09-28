package tracker

import "time"

// Stats summarises the whole score log.
type Stats struct {
	Plays int
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
	// BestGrades counts, per grade, the charts whose best play earned it,
	// best grade first. Grades no chart has are left out.
	BestGrades []GradeCount
}

// GradeCount is how many charts have a grade as their best.
type GradeCount struct {
	Grade Grade
	Count int
}

// Stats computes the summary as of now.
func (t *Tracker) Stats(now time.Time) Stats {
	s := Stats{Plays: len(t.Plays), Songs: len(t.Songs), Kcal: -1}
	if len(t.Plays) > 0 {
		s.FirstPlay = t.Plays[0].Date
		s.LastPlay = t.Plays[len(t.Plays)-1].Date
	}
	// Play dates are on the player's clock, so compare calendar days rather
	// than instants.
	y, m, d := now.Date()
	recent := time.Date(y, m, d-29, 0, 0, 0, 0, time.UTC)
	for _, p := range t.Plays {
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
		for _, h := range song.Charts {
			s.Charts++
			if h.Best != nil && h.Cleared {
				counts[h.Best.Grade]++
			}
			if !h.Cleared {
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
	for _, g := range gradeThresholds {
		if n := counts[g.Grade]; n > 0 {
			s.BestGrades = append(s.BestGrades, GradeCount{g.Grade, n})
		}
	}
	return s
}
